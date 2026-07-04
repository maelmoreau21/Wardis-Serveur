package video

import (
	"testing"
	"time"
)

func TestReconcileRecording(t *testing.T) {
	baseTime := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		incoming     VideoRecording
		existing     []VideoRecording
		wantStart    time.Time
		wantEnd      time.Time
		wantDelete   []string
		wantInsert   bool
	}{
		{
			name: "No overlap",
			incoming: VideoRecording{
				StartTime: baseTime,
				EndTime:   baseTime.Add(5 * time.Minute),
			},
			existing: []VideoRecording{
				{ID: "1", StartTime: baseTime.Add(10 * time.Minute), EndTime: baseTime.Add(15 * time.Minute)},
			},
			wantStart:  baseTime,
			wantEnd:    baseTime.Add(5 * time.Minute),
			wantDelete: nil,
			wantInsert: true,
		},
		{
			name: "Incoming completely covered by existing",
			incoming: VideoRecording{
				StartTime: baseTime.Add(2 * time.Minute),
				EndTime:   baseTime.Add(8 * time.Minute),
			},
			existing: []VideoRecording{
				{ID: "1", StartTime: baseTime, EndTime: baseTime.Add(10 * time.Minute)},
			},
			wantInsert: false,
		},
		{
			name: "Incoming completely covers existing",
			incoming: VideoRecording{
				StartTime: baseTime,
				EndTime:   baseTime.Add(10 * time.Minute),
			},
			existing: []VideoRecording{
				{ID: "1", StartTime: baseTime.Add(2 * time.Minute), EndTime: baseTime.Add(8 * time.Minute)},
			},
			wantStart:  baseTime,
			wantEnd:    baseTime.Add(10 * time.Minute),
			wantDelete: []string{"1"},
			wantInsert: true,
		},
		{
			name: "Incoming starts before and ends inside existing (left overlap)",
			incoming: VideoRecording{
				StartTime: baseTime,
				EndTime:   baseTime.Add(5 * time.Minute),
			},
			existing: []VideoRecording{
				{ID: "1", StartTime: baseTime.Add(3 * time.Minute), EndTime: baseTime.Add(8 * time.Minute)},
			},
			wantStart:  baseTime,
			wantEnd:    baseTime.Add(3 * time.Minute),
			wantDelete: nil,
			wantInsert: true,
		},
		{
			name: "Incoming starts inside and ends after existing (right overlap)",
			incoming: VideoRecording{
				StartTime: baseTime.Add(5 * time.Minute),
				EndTime:   baseTime.Add(10 * time.Minute),
			},
			existing: []VideoRecording{
				{ID: "1", StartTime: baseTime, EndTime: baseTime.Add(7 * time.Minute)},
			},
			wantStart:  baseTime.Add(7 * time.Minute),
			wantEnd:    baseTime.Add(10 * time.Minute),
			wantDelete: nil,
			wantInsert: true,
		},
		{
			name: "Incoming overlaps both sides causing double truncation to empty",
			incoming: VideoRecording{
				StartTime: baseTime.Add(2 * time.Minute),
				EndTime:   baseTime.Add(8 * time.Minute),
			},
			existing: []VideoRecording{
				{ID: "1", StartTime: baseTime, EndTime: baseTime.Add(5 * time.Minute)},
				{ID: "2", StartTime: baseTime.Add(5 * time.Minute), EndTime: baseTime.Add(10 * time.Minute)},
			},
			wantInsert: false,
		},
		{
			name: "Incoming overlaps both sides leaving a valid middle section",
			incoming: VideoRecording{
				StartTime: baseTime,
				EndTime:   baseTime.Add(15 * time.Minute),
			},
			existing: []VideoRecording{
				{ID: "1", StartTime: baseTime, EndTime: baseTime.Add(3 * time.Minute)},
				{ID: "2", StartTime: baseTime.Add(12 * time.Minute), EndTime: baseTime.Add(15 * time.Minute)},
			},
			wantStart:  baseTime.Add(3 * time.Minute),
			wantEnd:    baseTime.Add(12 * time.Minute),
			wantDelete: nil,
			wantInsert: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdj, gotDel, gotInsert := ReconcileRecording(tt.incoming, tt.existing)
			if gotInsert != tt.wantInsert {
				t.Fatalf("expected insert %v, got %v", tt.wantInsert, gotInsert)
			}
			if !gotInsert {
				return
			}
			if !gotAdj.StartTime.Equal(tt.wantStart) {
				t.Errorf("expected start %v, got %v", tt.wantStart, gotAdj.StartTime)
			}
			if !gotAdj.EndTime.Equal(tt.wantEnd) {
				t.Errorf("expected end %v, got %v", tt.wantEnd, gotAdj.EndTime)
			}
			if len(gotDel) != len(tt.wantDelete) {
				t.Fatalf("expected delete count %d, got %d", len(tt.wantDelete), len(gotDel))
			}
			for i, id := range gotDel {
				if id != tt.wantDelete[i] {
					t.Errorf("expected delete ID %s, got %s", tt.wantDelete[i], id)
				}
			}
		})
	}
}
