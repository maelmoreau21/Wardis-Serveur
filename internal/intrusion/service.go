package intrusion

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

type Service interface {
	ListZones(ctx context.Context) ([]Zone, error)
	ArmZone(ctx context.Context, zoneID string) error
	DisarmZone(ctx context.Context, zoneID string) error
	ListSensors(ctx context.Context) ([]Capteur, error)
	TriggerSensor(ctx context.Context, sensorID string) (*Alarme, error)
	ListActiveAlarms(ctx context.Context) ([]Alarme, error)
	AcknowledgeAlarm(ctx context.Context, id string, userID string, username string, reason string) error
	TransferAlarm(ctx context.Context, id string, userID string, username string, recipient string, reason string) error
	SnoozeAlarm(ctx context.Context, id string, userID string, username string, durationMinutes int, reason string) error
}

type service struct {
	repo      Repository
	publisher EventPublisher
	log       *zap.Logger
}

func NewService(repo Repository, publisher EventPublisher, log *zap.Logger) Service {
	return &service{
		repo:      repo,
		publisher: publisher,
		log:       log,
	}
}

func (s *service) ListZones(ctx context.Context) ([]Zone, error) {
	return s.repo.ListZones(ctx)
}

func (s *service) ArmZone(ctx context.Context, zoneID string) error {
	zone, err := s.repo.GetZoneByID(ctx, zoneID)
	if err != nil {
		return err
	}

	if zone.Statut == "arme" {
		return nil // Already armed
	}

	err = s.repo.UpdateZoneStatus(ctx, zoneID, "arme")
	if err != nil {
		return fmt.Errorf("failed to arm zone: %w", err)
	}

	// Create log in history
	details := fmt.Sprintf("Zone %s armée", zone.Nom)
	_, logErr := s.repo.CreateHistoryLog(ctx, &HistoriqueAlarme{
		ZoneID:    &zoneID,
		Evenement: "zone_armee",
		Details:   &details,
	})
	if logErr != nil {
		s.log.Error("failed to create history log for arming zone", zap.Error(logErr))
	}

	return nil
}

func (s *service) DisarmZone(ctx context.Context, zoneID string) error {
	zone, err := s.repo.GetZoneByID(ctx, zoneID)
	if err != nil {
		return err
	}

	if zone.Statut == "desarme" {
		return nil // Already disarmed
	}

	err = s.repo.UpdateZoneStatus(ctx, zoneID, "desarme")
	if err != nil {
		return fmt.Errorf("failed to disarm zone: %w", err)
	}

	// Reset sensor statuses to "ok" in the zone as a clean disarming side effect
	sensors, err := s.repo.ListSensors(ctx)
	if err == nil {
		for _, sensor := range sensors {
			if sensor.ZoneID == zoneID && sensor.Statut != "ok" {
				_ = s.repo.UpdateSensorStatus(ctx, sensor.ID, "ok")
			}
		}
	}

	// Create log in history
	details := fmt.Sprintf("Zone %s désarmée", zone.Nom)
	_, logErr := s.repo.CreateHistoryLog(ctx, &HistoriqueAlarme{
		ZoneID:    &zoneID,
		Evenement: "zone_desarmee",
		Details:   &details,
	})
	if logErr != nil {
		s.log.Error("failed to create history log for disarming zone", zap.Error(logErr))
	}

	return nil
}

func (s *service) ListSensors(ctx context.Context) ([]Capteur, error) {
	return s.repo.ListSensors(ctx)
}

func (s *service) TriggerSensor(ctx context.Context, sensorID string) (*Alarme, error) {
	// 1. Fetch sensor
	sensor, err := s.repo.GetSensorByID(ctx, sensorID)
	if err != nil {
		return nil, err
	}

	// 2. Update sensor status to "declenche"
	err = s.repo.UpdateSensorStatus(ctx, sensorID, "declenche")
	if err != nil {
		return nil, fmt.Errorf("failed to update sensor status to declenche: %w", err)
	}

	// 3. Create sensor trigger history entry
	details := fmt.Sprintf("Capteur %s déclenché (type: %s)", sensor.Nom, sensor.Type)
	_, logErr := s.repo.CreateHistoryLog(ctx, &HistoriqueAlarme{
		ZoneID:    &sensor.ZoneID,
		CapteurID: &sensorID,
		Evenement: "capteur_declenche",
		Details:   &details,
	})
	if logErr != nil {
		s.log.Error("failed to create history log for sensor trigger", zap.Error(logErr))
	}

	// 4. Fetch zone of the sensor
	zone, err := s.repo.GetZoneByID(ctx, sensor.ZoneID)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone for triggered sensor: %w", err)
	}

	// 5. If zone is armed, trigger an alarm
	if zone.Statut == "arme" {
		alarmRecord, err := s.repo.CreateAlarm(ctx, &Alarme{
			ZoneID:    sensor.ZoneID,
			CapteurID: sensorID,
			Statut:    "active",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create alarm: %w", err)
		}

		// Log alarm triggered in history
		alarmDetails := fmt.Sprintf("Alarme déclenchée par le capteur %s dans la zone %s", sensor.Nom, zone.Nom)
		_, logErr = s.repo.CreateHistoryLog(ctx, &HistoriqueAlarme{
			AlarmeID:  &alarmRecord.ID,
			ZoneID:    &sensor.ZoneID,
			CapteurID: &sensorID,
			Evenement: "alarme_declenchee",
			Details:   &alarmDetails,
		})
		if logErr != nil {
			s.log.Error("failed to create history log for alarm triggered", zap.Error(logErr))
		}

		// Publish NATS event
		s.publishNatsAlarmTriggered(ctx, alarmRecord)

		return alarmRecord, nil
	}

	return nil, nil
}

func (s *service) ListActiveAlarms(ctx context.Context) ([]Alarme, error) {
	return s.repo.ListActiveAlarms(ctx)
}

func (s *service) publishNatsAlarmTriggered(ctx context.Context, alarm *Alarme) {
	payload := map[string]interface{}{
		"alarme_id":    alarm.ID,
		"zone_id":      alarm.ZoneID,
		"capteur_id":   alarm.CapteurID,
		"statut":       alarm.Statut,
		"timestamp":    alarm.DeclencheeA,
	}

	err := s.publisher.PublishAlarmTriggered(ctx, payload)
	if err != nil {
		s.log.Error("failed to publish NATS alarm.triggered event",
			zap.String("alarme_id", alarm.ID),
			zap.Error(err),
		)
	} else {
		s.log.Info("successfully published NATS alarm.triggered event",
			zap.String("alarme_id", alarm.ID),
		)
	}
}

func (s *service) AcknowledgeAlarm(ctx context.Context, id string, userID string, username string, reason string) error {
	alarm, err := s.repo.GetAlarmByID(ctx, id)
	if err != nil {
		return err
	}

	err = s.repo.UpdateAlarmStatus(ctx, id, "acquittee", &userID)
	if err != nil {
		return err
	}

	details := fmt.Sprintf("%s a acquitté l'alarme: %s", username, reason)
	_, logErr := s.repo.CreateHistoryLog(ctx, &HistoriqueAlarme{
		AlarmeID:  &id,
		ZoneID:    &alarm.ZoneID,
		CapteurID: &alarm.CapteurID,
		Evenement: "alarme_acquittee",
		Details:   &details,
	})
	if logErr != nil {
		s.log.Error("failed to create history log for alarm acknowledgement", zap.Error(logErr))
	}

	// Publish NATS event so all clients update active alarms in real-time
	eventPayload := map[string]interface{}{
		"alarme_id":     id,
		"statut":        "acquittee",
		"acquittee_par": userID,
		"timestamp":     time.Now(),
		"reason":        reason,
		"username":      username,
	}
	err = s.publisher.PublishEvent(ctx, "alarm.acknowledged", eventPayload)
	if err != nil {
		s.log.Error("failed to publish NATS alarm.acknowledged event", zap.Error(err))
	}
	return nil
}

func (s *service) TransferAlarm(ctx context.Context, id string, userID string, username string, recipient string, reason string) error {
	alarm, err := s.repo.GetAlarmByID(ctx, id)
	if err != nil {
		return err
	}

	details := fmt.Sprintf("%s a transféré l'alarme à %s: %s", username, recipient, reason)
	_, logErr := s.repo.CreateHistoryLog(ctx, &HistoriqueAlarme{
		AlarmeID:  &id,
		ZoneID:    &alarm.ZoneID,
		CapteurID: &alarm.CapteurID,
		Evenement: "alarme_transferee",
		Details:   &details,
	})
	if logErr != nil {
		s.log.Error("failed to create history log for alarm transfer", zap.Error(logErr))
	}

	// Publish NATS event
	eventPayload := map[string]interface{}{
		"alarme_id": id,
		"statut":    "active",
		"details":   details,
		"timestamp": time.Now(),
		"recipient": recipient,
		"reason":    reason,
		"username":  username,
	}
	err = s.publisher.PublishEvent(ctx, "alarm.transferred", eventPayload)
	if err != nil {
		s.log.Error("failed to publish NATS alarm.transferred event", zap.Error(err))
	}
	return nil
}

func (s *service) SnoozeAlarm(ctx context.Context, id string, userID string, username string, durationMinutes int, reason string) error {
	alarm, err := s.repo.GetAlarmByID(ctx, id)
	if err != nil {
		return err
	}

	details := fmt.Sprintf("%s a reporté l'alarme de %d minutes: %s", username, durationMinutes, reason)
	_, logErr := s.repo.CreateHistoryLog(ctx, &HistoriqueAlarme{
		AlarmeID:  &id,
		ZoneID:    &alarm.ZoneID,
		CapteurID: &alarm.CapteurID,
		Evenement: "alarme_reportee",
		Details:   &details,
	})
	if logErr != nil {
		s.log.Error("failed to create history log for alarm snooze", zap.Error(logErr))
	}

	// Publish NATS event
	eventPayload := map[string]interface{}{
		"alarme_id":        id,
		"statut":           "active",
		"details":          details,
		"timestamp":        time.Now(),
		"duration_minutes": durationMinutes,
		"reason":           reason,
		"username":         username,
	}
	err = s.publisher.PublishEvent(ctx, "alarm.snoozed", eventPayload)
	if err != nil {
		s.log.Error("failed to publish NATS alarm.snoozed event", zap.Error(err))
	}
	return nil
}

