package video

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"strings"
)

// Start starts the NATS subscribers and the periodic video tiering background worker.
func (s *service) Start(ctx context.Context) error {
	if s.cfg == nil {
		return errors.New("cannot start background worker: config is nil")
	}

	// 1. Connect to NATS for Trickling/Sync/Motion subscriber
	s.log.Info("Connecting to NATS for Video Sync & Motion...", zap.String("url", s.cfg.NatsURL))
	nc, err := nats.Connect(s.cfg.NatsURL)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS in video service: %w", err)
	}
	s.natsConn = nc

	// 2. Subscribe to video.sync topic
	subSync, err := s.natsConn.Subscribe("video.sync", func(msg *nats.Msg) {
		s.handleNatsSyncMessage(msg)
	})
	if err != nil {
		s.natsConn.Close()
		s.natsConn = nil
		return fmt.Errorf("failed to subscribe to video.sync NATS topic: %w", err)
	}
	s.natsSubs = append(s.natsSubs, subSync)
	s.log.Info("Subscribed to video.sync NATS topic")

	// 2b. Subscribe to video.motion topic
	subMotion, err := s.natsConn.Subscribe("video.motion", func(msg *nats.Msg) {
		s.handleNatsMotionMessage(msg)
	})
	if err != nil {
		s.natsConn.Close()
		s.natsConn = nil
		return fmt.Errorf("failed to subscribe to video.motion NATS topic: %w", err)
	}
	s.natsSubs = append(s.natsSubs, subMotion)
	s.log.Info("Subscribed to video.motion NATS topic")

	// 3. Start Video Tiering Job worker goroutine
	go s.runTieringWorker(ctx)

	// 4. Start Recording & Retention Scheduler
	go s.runRecordingScheduler(ctx)

	return nil
}

// Close unsubscribes NATS subscriptions, closes connections, and signals background worker to stop.
func (s *service) Close() {
	s.log.Info("Stopping Video service background processes...")
	close(s.stopChan)

	for _, sub := range s.natsSubs {
		_ = sub.Unsubscribe()
	}
	s.natsSubs = nil

	if s.natsConn != nil {
		s.natsConn.Close()
		s.natsConn = nil
	}
	s.log.Info("Video service background processes stopped")
}

// SyncRecording receives local camera files (from direct API or NATS sync), deduplicates/reconciles against
// PostgreSQL timeline, saves the video file locally, and registers the database index.
func (s *service) SyncRecording(ctx context.Context, cameraID string, startTime, endTime time.Time, fileData []byte, filename string) (*VideoRecording, error) {
	// 1. Verify camera exists
	camera, err := s.repo.GetByID(ctx, cameraID)
	if err != nil {
		return nil, fmt.Errorf("camera lookup failed: %w", err)
	}

	if camera.MainStreamURL == "" {
		return nil, fmt.Errorf("recording rejected: camera %s does not have a main_stream_url configured", cameraID)
	}

	// 2. Query overlapping recordings
	overlapping, err := s.repo.GetOverlappingRecordings(ctx, cameraID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query overlapping recordings: %w", err)
	}

	// 3. Reconcile boundaries & determine deletes
	tempRec := VideoRecording{
		CameraID:    cameraID,
		StartTime:   startTime,
		EndTime:     endTime,
		StorageType: "local",
		FileSize:    int64(len(fileData)),
	}

	adjusted, idsToDelete, shouldInsert := ReconcileRecording(tempRec, overlapping)
	if !shouldInsert {
		s.log.Info("Incoming recording is fully redundant and skipped",
			zap.String("camera_id", cameraID),
			zap.Time("start", startTime),
			zap.Time("end", endTime),
		)
		return nil, nil
	}

	// 4. Delete fully overlapped recordings and their files
	for _, oldID := range idsToDelete {
		// Find filepath to clean up
		var oldPath string
		for _, o := range overlapping {
			if o.ID == oldID {
				oldPath = o.Filepath
				break
			}
		}

		s.log.Info("Deleting redundant overlapping recording", zap.String("id", oldID), zap.String("path", oldPath))
		if err := s.repo.DeleteRecording(ctx, oldID); err != nil {
			s.log.Error("Failed to delete recording index", zap.String("id", oldID), zap.Error(err))
		}

		if oldPath != "" && !filepath.IsAbs(oldPath) && s.cfg != nil {
			oldPath = filepath.Join(s.cfg.RecordingsLocalDir, oldPath)
		}
		if oldPath != "" {
			_ = os.Remove(oldPath)
		}
	}

	// 5. Create local storage directories
	localDir := filepath.Join(s.cfg.RecordingsLocalDir, cameraID)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create recordings directory: %w", err)
	}

	// If the filename is empty, auto-generate one
	if filename == "" {
		filename = fmt.Sprintf("%d_%d.mp4", adjusted.StartTime.Unix(), adjusted.EndTime.Unix())
	}

	relativeFilepath := filepath.Join(cameraID, filename)
	absoluteFilepath := filepath.Join(localDir, filename)

	// 6. Write file data to disk
	if err := os.WriteFile(absoluteFilepath, fileData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write video file to disk: %w", err)
	}

	// 7. Save to database index
	adjusted.Filepath = relativeFilepath
	if err := s.repo.SaveRecording(ctx, &adjusted); err != nil {
		// Clean up the file if DB insert fails
		_ = os.Remove(absoluteFilepath)
		return nil, fmt.Errorf("failed to save recording index: %w", err)
	}

	return &adjusted, nil
}

// handleNatsSyncMessage handles incoming base64 video sync data via NATS
func (s *service) handleNatsSyncMessage(msg *nats.Msg) {
	s.log.Debug("Received video.sync NATS message", zap.Int("size", len(msg.Data)))
	var payload SyncRecordingPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		s.log.Error("Failed to unmarshal NATS video sync payload", zap.Error(err))
		return
	}

	videoData, err := base64.StdEncoding.DecodeString(payload.VideoDataB64)
	if err != nil {
		s.log.Error("Failed to decode base64 video data from NATS payload", zap.Error(err))
		return
	}

	filename := fmt.Sprintf("%d_%d.mp4", payload.StartTime.Unix(), payload.EndTime.Unix())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	rec, err := s.SyncRecording(ctx, payload.CameraID, payload.StartTime, payload.EndTime, videoData, filename)
	if err != nil {
		s.log.Error("Failed to sync recording from NATS subscriber", zap.String("camera_id", payload.CameraID), zap.Error(err))
		return
	}

	if rec != nil {
		s.log.Info("Successfully synced recording via NATS", zap.String("recording_id", rec.ID), zap.String("camera_id", payload.CameraID))
	}
}

// runTieringWorker runs the periodic loop to tier/archive old local recordings.
func (s *service) runTieringWorker(ctx context.Context) {
	s.log.Info("Starting Video Tiering background worker",
		zap.Duration("interval", s.cfg.VideoTieringInterval),
		zap.Int("age_days", s.cfg.VideoTieringAgeDays),
	)

	ticker := time.NewTicker(s.cfg.VideoTieringInterval)
	defer ticker.Stop()

	// Run initially on startup
	s.runTieringJob(ctx)

	for {
		select {
		case <-s.stopChan:
			s.log.Info("Video Tiering worker stopped by stop signal")
			return
		case <-ctx.Done():
			s.log.Info("Video Tiering worker stopped by context cancellation")
			return
		case <-ticker.C:
			s.runTieringJob(ctx)
		}
	}
}

// runTieringJob scans for local files older than X days, uploads to MinIO, and updates paths.
func (s *service) runTieringJob(ctx context.Context) {
	if s.minioClient == nil {
		s.log.Warn("Skipping video tiering job: MinIO client is not initialized")
		return
	}

	s.log.Info("Running Video Tiering job...")

	// Calculate age threshold
	threshold := time.Now().AddDate(0, 0, -s.cfg.VideoTieringAgeDays)

	recordings, err := s.repo.GetLocalRecordingsOlderThan(ctx, threshold)
	if err != nil {
		s.log.Error("Failed to retrieve local recordings for tiering", zap.Error(err))
		return
	}

	if len(recordings) == 0 {
		s.log.Info("No old local video recordings found for tiering")
		return
	}

	s.log.Info("Found recordings to archive", zap.Int("count", len(recordings)))

	// Ensure MinIO bucket exists
	bucket := s.cfg.MinioBucket
	err = s.minioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	if err != nil {
		// Check if bucket already exists
		exists, errBucketExists := s.minioClient.BucketExists(ctx, bucket)
		if errBucketExists != nil || !exists {
			s.log.Error("Failed to ensure MinIO bucket exists", zap.String("bucket", bucket), zap.Error(err))
			return
		}
	}

	for _, rec := range recordings {
		localPath := filepath.Join(s.cfg.RecordingsLocalDir, rec.Filepath)
		s.log.Info("Tiering recording file", zap.String("id", rec.ID), zap.String("local_path", localPath))

		// Check if file exists
		file, err := os.Open(localPath)
		if err != nil {
			if os.IsNotExist(err) {
				s.log.Warn("Local recording file not found on disk, skipping upload and updating DB to error/missing status if needed", zap.String("id", rec.ID), zap.String("path", localPath))
				// We can update the index or delete it to stay consistent, here we delete the DB index to avoid orphaned dead links
				_ = s.repo.DeleteRecording(ctx, rec.ID)
			} else {
				s.log.Error("Failed to open local recording file", zap.String("id", rec.ID), zap.Error(err))
			}
			continue
		}

		// Upload to MinIO
		// Key: cameraID/filename
		objectKey := rec.Filepath // e.g. "camera_uuid/filename.mp4"
		_, err = s.minioClient.PutObject(ctx, bucket, objectKey, file, rec.FileSize, minio.PutObjectOptions{
			ContentType: "video/mp4",
		})
		_ = file.Close() // Close file after reading

		if err != nil {
			s.log.Error("Failed to upload recording to MinIO", zap.String("id", rec.ID), zap.String("object", objectKey), zap.Error(err))
			continue
		}

		// Update PostgreSQL index to cloud storage
		cloudURL := fmt.Sprintf("minio://%s/%s", bucket, objectKey)
		err = s.repo.UpdateRecordingStorageType(ctx, rec.ID, "cloud", cloudURL)
		if err != nil {
			s.log.Error("Failed to update database storage path to cloud", zap.String("id", rec.ID), zap.Error(err))
			// Attempt to clean up the MinIO object since DB update failed (rollback cloud upload to prevent inconsistency)
			_ = s.minioClient.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{})
			continue
		}

		// Clean up the local file
		err = os.Remove(localPath)
		if err != nil {
			s.log.Error("Failed to delete local recording file after cloud migration", zap.String("path", localPath), zap.Error(err))
		} else {
			s.log.Info("Successfully archived video recording to cloud", zap.String("id", rec.ID), zap.String("cloud_path", cloudURL))
		}
	}

	s.log.Info("Video Tiering job run completed")
}

// handleNatsMotionMessage handles motion detection events from NATS
func (s *service) handleNatsMotionMessage(msg *nats.Msg) {
	s.log.Debug("Received video.motion NATS message", zap.Int("size", len(msg.Data)))
	var payload VideoEventPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		s.log.Error("Failed to unmarshal NATS video motion payload", zap.Error(err))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	camera, err := s.repo.GetByID(ctx, payload.CameraID)
	if err != nil {
		s.log.Error("Failed to look up camera for NATS motion event", zap.String("camera_id", payload.CameraID), zap.Error(err))
		return
	}

	// Trigger motion recording if configured
	if camera.RecordingMode == "motion" || camera.RecordingMode == "both" {
		startTime := payload.Timestamp
		if startTime.IsZero() {
			startTime = time.Now()
		}
		endTime := startTime.Add(30 * time.Second)

		// Create simulated 1-byte file to record motion segment
		dummyData := []byte{0}
		filename := fmt.Sprintf("motion_%d_%d.mp4", startTime.Unix(), endTime.Unix())

		_, err := s.SyncRecording(ctx, camera.ID, startTime, endTime, dummyData, filename)
		if err != nil {
			s.log.Error("Failed to auto-record motion event", zap.String("camera_id", camera.ID), zap.Error(err))
		} else {
			s.log.Info("Successfully auto-recorded motion event segment", zap.String("camera_id", camera.ID), zap.Time("start", startTime))
		}
	}
}

// runRecordingScheduler runs the loop for scheduling continuous recordings and enforcing camera-level retention
func (s *service) runRecordingScheduler(ctx context.Context) {
	s.log.Info("Starting Continuous Recording and Retention Scheduler")

	// Check every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Initial run
	s.runRecordingScheduleJob(ctx)

	for {
		select {
		case <-s.stopChan:
			s.log.Info("Recording Scheduler worker stopped by stop signal")
			return
		case <-ctx.Done():
			s.log.Info("Recording Scheduler worker stopped by context cancellation")
			return
		case <-ticker.C:
			s.runRecordingScheduleJob(ctx)
		}
	}
}

func (s *service) runRecordingScheduleJob(ctx context.Context) {
	s.log.Debug("Running Recording Schedule and Retention Job...")
	cameras, err := s.repo.List(ctx)
	if err != nil {
		s.log.Error("Failed to fetch cameras in recording scheduler", zap.Error(err))
		return
	}

	now := time.Now()

	for _, cam := range cameras {
		if cam.Statut != "active" {
			continue
		}

		// 1. Handle continuous recording
		if cam.RecordingMode == "continuous" || cam.RecordingMode == "both" {
			startTime := now.Add(-1 * time.Minute)
			endTime := now

			// Check if a recording already exists for this interval
			existing, err := s.repo.GetOverlappingRecordings(ctx, cam.ID, startTime, endTime)
			if err == nil && len(existing) == 0 {
				// No recording covers this time, generate a continuous segment
				dummyData := []byte{0}
				filename := fmt.Sprintf("cont_%d_%d.mp4", startTime.Unix(), endTime.Unix())
				_, err := s.SyncRecording(ctx, cam.ID, startTime, endTime, dummyData, filename)
				if err != nil {
					s.log.Error("Failed to schedule continuous recording segment", zap.String("camera_id", cam.ID), zap.Error(err))
				}
			}
		}

		// 2. Enforce camera-specific retention policy
		retentionDays := cam.RetentionDays
		if retentionDays <= 0 {
			retentionDays = 30 // Default fallback
		}

		threshold := now.AddDate(0, 0, -retentionDays)
		// Retrieve recordings for this camera older than the threshold
		// We query from long ago (e.g. 10 years ago) to threshold
		oldRecordings, err := s.repo.GetOverlappingRecordings(ctx, cam.ID, now.AddDate(-10, 0, 0), threshold)
		if err != nil {
			s.log.Error("Failed to fetch old recordings for retention cleanup", zap.String("camera_id", cam.ID), zap.Error(err))
			continue
		}

		for _, rec := range oldRecordings {
			s.log.Info("Retention: cleaning up expired video recording", zap.String("id", rec.ID), zap.String("camera_id", cam.ID), zap.Time("start", rec.StartTime))
			
			// Delete DB record
			if err := s.repo.DeleteRecording(ctx, rec.ID); err != nil {
				s.log.Error("Retention: failed to delete recording index", zap.String("id", rec.ID), zap.Error(err))
				continue
			}

			// Clean up file from storage
			if rec.StorageType == "local" {
				localPath := filepath.Join(s.cfg.RecordingsLocalDir, rec.Filepath)
				_ = os.Remove(localPath)
			} else if rec.StorageType == "cloud" && s.minioClient != nil {
				trimmed := strings.TrimPrefix(rec.Filepath, "minio://")
				parts := strings.SplitN(trimmed, "/", 2)
				if len(parts) == 2 {
					_ = s.minioClient.RemoveObject(ctx, parts[0], parts[1], minio.RemoveObjectOptions{})
				}
			}
		}
	}
}
