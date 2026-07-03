package intrusion_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"wardis-server/internal/intrusion"
)

type mockPublisher struct {
	triggeredCalls []interface{}
}

func (m *mockPublisher) PublishAlarmTriggered(ctx context.Context, payload interface{}) error {
	m.triggeredCalls = append(m.triggeredCalls, payload)
	return nil
}

func (m *mockPublisher) Close() {}

type mockRepository struct {
	zones   map[string]intrusion.Zone
	sensors map[string]intrusion.Capteur
	alarms  map[string]intrusion.Alarme
	history []intrusion.HistoriqueAlarme
}

func (m *mockRepository) ListZones(ctx context.Context) ([]intrusion.Zone, error) {
	var zones []intrusion.Zone
	for _, z := range m.zones {
		zones = append(zones, z)
	}
	return zones, nil
}

func (m *mockRepository) GetZoneByID(ctx context.Context, id string) (*intrusion.Zone, error) {
	z, ok := m.zones[id]
	if !ok {
		return nil, intrusion.ErrZoneNotFound
	}
	return &z, nil
}

func (m *mockRepository) UpdateZoneStatus(ctx context.Context, id string, statut string) error {
	z, ok := m.zones[id]
	if !ok {
		return intrusion.ErrZoneNotFound
	}
	z.Statut = statut
	m.zones[id] = z
	return nil
}

func (m *mockRepository) ListSensors(ctx context.Context) ([]intrusion.Capteur, error) {
	var sensors []intrusion.Capteur
	for _, s := range m.sensors {
		sensors = append(sensors, s)
	}
	return sensors, nil
}

func (m *mockRepository) GetSensorByID(ctx context.Context, id string) (*intrusion.Capteur, error) {
	s, ok := m.sensors[id]
	if !ok {
		return nil, intrusion.ErrSensorNotFound
	}
	return &s, nil
}

func (m *mockRepository) UpdateSensorStatus(ctx context.Context, id string, statut string) error {
	s, ok := m.sensors[id]
	if !ok {
		return intrusion.ErrSensorNotFound
	}
	s.Statut = statut
	m.sensors[id] = s
	return nil
}

func (m *mockRepository) CreateAlarm(ctx context.Context, alarm *intrusion.Alarme) (*intrusion.Alarme, error) {
	alarm.ID = "alarm-mock-uuid"
	alarm.DeclencheeA = time.Now()
	m.alarms[alarm.ID] = *alarm
	return alarm, nil
}

func (m *mockRepository) ListActiveAlarms(ctx context.Context) ([]intrusion.Alarme, error) {
	var alarms []intrusion.Alarme
	for _, a := range m.alarms {
		if a.Statut == "active" {
			alarms = append(alarms, a)
		}
	}
	return alarms, nil
}

func (m *mockRepository) CreateHistoryLog(ctx context.Context, log *intrusion.HistoriqueAlarme) (*intrusion.HistoriqueAlarme, error) {
	log.ID = "history-mock-uuid"
	log.CreeLe = time.Now()
	m.history = append(m.history, *log)
	return log, nil
}

func TestIntrusionService(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("Arm Zone Success", func(t *testing.T) {
		repo := &mockRepository{
			zones: map[string]intrusion.Zone{
				"zone-1": {
					ID:     "zone-1",
					Nom:    "Zone Entrée",
					Statut: "desarme",
				},
			},
		}
		pub := &mockPublisher{}
		svc := intrusion.NewService(repo, pub, logger)

		err := svc.ArmZone(ctx, "zone-1")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if repo.zones["zone-1"].Statut != "arme" {
			t.Errorf("expected zone status to be 'arme', got '%s'", repo.zones["zone-1"].Statut)
		}

		if len(repo.history) != 1 || repo.history[0].Evenement != "zone_armee" {
			t.Errorf("expected one history log with event 'zone_armee'")
		}
	})

	t.Run("Disarm Zone Success", func(t *testing.T) {
		repo := &mockRepository{
			zones: map[string]intrusion.Zone{
				"zone-1": {
					ID:     "zone-1",
					Nom:    "Zone Entrée",
					Statut: "arme",
				},
			},
			sensors: map[string]intrusion.Capteur{
				"sensor-1": {
					ID:     "sensor-1",
					ZoneID: "zone-1",
					Statut: "declenche",
				},
			},
		}
		pub := &mockPublisher{}
		svc := intrusion.NewService(repo, pub, logger)

		err := svc.DisarmZone(ctx, "zone-1")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if repo.zones["zone-1"].Statut != "desarme" {
			t.Errorf("expected zone status to be 'desarme', got '%s'", repo.zones["zone-1"].Statut)
		}

		if repo.sensors["sensor-1"].Statut != "ok" {
			t.Errorf("expected sensor status to reset to 'ok', got '%s'", repo.sensors["sensor-1"].Statut)
		}

		foundDisarmedLog := false
		for _, h := range repo.history {
			if h.Evenement == "zone_desarmee" {
				foundDisarmedLog = true
			}
		}
		if !foundDisarmedLog {
			t.Errorf("expected history log with event 'zone_desarmee'")
		}
	})

	t.Run("Trigger Sensor while Disarmed", func(t *testing.T) {
		repo := &mockRepository{
			zones: map[string]intrusion.Zone{
				"zone-1": {
					ID:     "zone-1",
					Nom:    "Zone Entrée",
					Statut: "desarme",
				},
			},
			sensors: map[string]intrusion.Capteur{
				"sensor-1": {
					ID:     "sensor-1",
					ZoneID: "zone-1",
					Nom:    "Mouvement Entrée",
					Type:   "mouvement",
					Statut: "ok",
				},
			},
			alarms: make(map[string]intrusion.Alarme),
		}
		pub := &mockPublisher{}
		svc := intrusion.NewService(repo, pub, logger)

		alarm, err := svc.TriggerSensor(ctx, "sensor-1")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if alarm != nil {
			t.Errorf("expected no alarm to be created when zone is disarmed")
		}

		if repo.sensors["sensor-1"].Statut != "declenche" {
			t.Errorf("expected sensor status to be 'declenche', got '%s'", repo.sensors["sensor-1"].Statut)
		}

		if len(pub.triggeredCalls) != 0 {
			t.Errorf("expected no NATS events to be published")
		}

		foundTriggeredLog := false
		for _, h := range repo.history {
			if h.Evenement == "capteur_declenche" {
				foundTriggeredLog = true
			}
		}
		if !foundTriggeredLog {
			t.Errorf("expected history log with event 'capteur_declenche'")
		}
	})

	t.Run("Trigger Sensor while Armed", func(t *testing.T) {
		repo := &mockRepository{
			zones: map[string]intrusion.Zone{
				"zone-1": {
					ID:     "zone-1",
					Nom:    "Zone Entrée",
					Statut: "arme",
				},
			},
			sensors: map[string]intrusion.Capteur{
				"sensor-1": {
					ID:     "sensor-1",
					ZoneID: "zone-1",
					Nom:    "Mouvement Entrée",
					Type:   "mouvement",
					Statut: "ok",
				},
			},
			alarms: make(map[string]intrusion.Alarme),
		}
		pub := &mockPublisher{}
		svc := intrusion.NewService(repo, pub, logger)

		alarm, err := svc.TriggerSensor(ctx, "sensor-1")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if alarm == nil {
			t.Fatalf("expected alarm to be created when zone is armed")
		}

		if alarm.Statut != "active" {
			t.Errorf("expected alarm status to be 'active', got '%s'", alarm.Statut)
		}

		if repo.sensors["sensor-1"].Statut != "declenche" {
			t.Errorf("expected sensor status to be 'declenche', got '%s'", repo.sensors["sensor-1"].Statut)
		}

		if len(pub.triggeredCalls) != 1 {
			t.Errorf("expected exactly 1 NATS alarm event to be published, got %d", len(pub.triggeredCalls))
		}

		foundAlarmLog := false
		for _, h := range repo.history {
			if h.Evenement == "alarme_declenchee" {
				foundAlarmLog = true
			}
		}
		if !foundAlarmLog {
			t.Errorf("expected history log with event 'alarme_declenchee'")
		}
	})
}
