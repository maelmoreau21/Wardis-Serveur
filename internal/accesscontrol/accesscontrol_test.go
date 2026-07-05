package accesscontrol_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"wardis-server/internal/accesscontrol"
)

// mockPublisher is a mock of the EventPublisher interface
type mockPublisher struct {
	grantedCalls []interface{}
	deniedCalls  []interface{}
}

func (m *mockPublisher) PublishAccessGranted(ctx context.Context, payload interface{}) error {
	m.grantedCalls = append(m.grantedCalls, payload)
	return nil
}

func (m *mockPublisher) PublishAccessDenied(ctx context.Context, payload interface{}) error {
	m.deniedCalls = append(m.deniedCalls, payload)
	return nil
}

func (m *mockPublisher) Close() {}

// mockRepository is a mock of the Repository interface
type mockRepository struct {
	doors       map[string]accesscontrol.Door
	badges      map[string]accesscontrol.Badge
	permissions map[string]bool // key: badgeID + "_" + doorID
	logs        []accesscontrol.AccessLog
}

func (m *mockRepository) ListDoors(ctx context.Context) ([]accesscontrol.Door, error) {
	var doors []accesscontrol.Door
	for _, d := range m.doors {
		doors = append(doors, d)
	}
	return doors, nil
}

func (m *mockRepository) GetDoorByID(ctx context.Context, id string) (*accesscontrol.Door, error) {
	d, ok := m.doors[id]
	if !ok {
		return nil, accesscontrol.ErrDoorNotFound
	}
	return &d, nil
}

func (m *mockRepository) UpdateDoorStatus(ctx context.Context, id string, status string) error {
	d, ok := m.doors[id]
	if !ok {
		return accesscontrol.ErrDoorNotFound
	}
	d.Status = status
	m.doors[id] = d
	return nil
}

func (m *mockRepository) GetBadgeByNumber(ctx context.Context, number string) (*accesscontrol.Badge, error) {
	for _, b := range m.badges {
		if b.Number == number {
			return &b, nil
		}
	}
	return nil, accesscontrol.ErrBadgeNotFound
}

func (m *mockRepository) AssignBadge(ctx context.Context, number string, userID string) (*accesscontrol.Badge, error) {
	b := accesscontrol.Badge{
		ID:        "badge-uuid-" + number,
		Number:    number,
		UserID:    &userID,
		Status:    "active",
		CreatedAt: time.Now(),
	}
	m.badges[b.ID] = b
	return &b, nil
}

func (m *mockRepository) CheckAccessPermission(ctx context.Context, badgeID string, doorID string) (bool, error) {
	allowed, ok := m.permissions[badgeID+"_"+doorID]
	if !ok {
		return false, nil
	}
	return allowed, nil
}

func (m *mockRepository) CreateAccessLog(ctx context.Context, log *accesscontrol.AccessLog) (*accesscontrol.AccessLog, error) {
	log.ID = "log-uuid-mocked"
	log.CreatedAt = time.Now()
	m.logs = append(m.logs, *log)
	return log, nil
}

func (m *mockRepository) ListAccessLogs(ctx context.Context) ([]accesscontrol.AccessLog, error) {
	return m.logs, nil
}

func (m *mockRepository) ListCardholders(ctx context.Context) ([]accesscontrol.Cardholder, error) {
	return nil, nil
}

func (m *mockRepository) GetCardholderByID(ctx context.Context, id string) (*accesscontrol.Cardholder, error) {
	return &accesscontrol.Cardholder{
		ID:        id,
		FirstName: "Mocked",
		LastName:  "Cardholder",
		Photo:     "mocked_photo",
	}, nil
}

func (m *mockRepository) CreateCardholder(ctx context.Context, c *accesscontrol.Cardholder) (*accesscontrol.Cardholder, error) {
	c.ID = "cardholder-uuid-mocked"
	return c, nil
}

func (m *mockRepository) UpdateCardholder(ctx context.Context, id string, c *accesscontrol.Cardholder) (*accesscontrol.Cardholder, error) {
	c.ID = id
	return c, nil
}

func (m *mockRepository) DeleteCardholder(ctx context.Context, id string) error {
	return nil
}

func (m *mockRepository) CreateDoor(ctx context.Context, d *accesscontrol.Door) (*accesscontrol.Door, error) {
	return d, nil
}
func (m *mockRepository) UpdateDoor(ctx context.Context, id string, d *accesscontrol.Door) (*accesscontrol.Door, error) {
	return d, nil
}
func (m *mockRepository) DeleteDoor(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListSites(ctx context.Context) ([]accesscontrol.Site, error) {
	return nil, nil
}
func (m *mockRepository) GetSiteByID(ctx context.Context, id string) (*accesscontrol.Site, error) {
	return nil, nil
}
func (m *mockRepository) CreateSite(ctx context.Context, s *accesscontrol.Site) (*accesscontrol.Site, error) {
	return s, nil
}
func (m *mockRepository) UpdateSite(ctx context.Context, id string, s *accesscontrol.Site) (*accesscontrol.Site, error) {
	return s, nil
}
func (m *mockRepository) DeleteSite(ctx context.Context, id string) error {
	return nil
}

func TestAccessControlService(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("Assign Badge Success", func(t *testing.T) {
		repo := &mockRepository{
			badges: make(map[string]accesscontrol.Badge),
		}
		pub := &mockPublisher{}
		svc := accesscontrol.NewService(repo, pub, logger)

		b, err := svc.AssignBadge(ctx, "CARD123", "user-uuid-1")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if b.Number != "CARD123" {
			t.Errorf("expected badge number CARD123, got %s", b.Number)
		}
		if *b.UserID != "user-uuid-1" {
			t.Errorf("expected user ID user-uuid-1, got %s", *b.UserID)
		}
		if b.Status != "active" {
			t.Errorf("expected badge status active, got %s", b.Status)
		}
	})

	t.Run("Open / Close Door Success", func(t *testing.T) {
		repo := &mockRepository{
			doors: map[string]accesscontrol.Door{
				"door-1": {
					ID:     "door-1",
					Status: "closed",
				},
			},
		}
		pub := &mockPublisher{}
		svc := accesscontrol.NewService(repo, pub, logger)

		err := svc.OpenDoor(ctx, "door-1")
		if err != nil {
			t.Fatalf("expected no error opening door, got: %v", err)
		}
		if repo.doors["door-1"].Status != "open" {
			t.Errorf("expected door status 'open', got '%s'", repo.doors["door-1"].Status)
		}

		err = svc.CloseDoor(ctx, "door-1")
		if err != nil {
			t.Fatalf("expected no error closing door, got: %v", err)
		}
		if repo.doors["door-1"].Status != "closed" {
			t.Errorf("expected door status 'closed', got '%s'", repo.doors["door-1"].Status)
		}
	})

	t.Run("Swipe Badge - Non-existent Badge", func(t *testing.T) {
		repo := &mockRepository{
			doors: map[string]accesscontrol.Door{
				"door-1": {
					ID:     "door-1",
					SiteID: "site-1",
				},
			},
			badges: make(map[string]accesscontrol.Badge),
		}
		pub := &mockPublisher{}
		svc := accesscontrol.NewService(repo, pub, logger)

		resp, err := svc.SwipeBadge(ctx, "door-1", "UNKNOWN_CARD")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if resp.AccessGranted {
			t.Error("expected access to be denied")
		}

		if len(repo.logs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(repo.logs))
		}
		if repo.logs[0].AccessType != "denied" {
			t.Errorf("expected access log to be denied, got %s", repo.logs[0].AccessType)
		}
		if *repo.logs[0].DeniedReason != "badge not found" {
			t.Errorf("expected reason 'badge not found', got '%s'", *repo.logs[0].DeniedReason)
		}

		if len(pub.deniedCalls) != 1 {
			t.Errorf("expected 1 NATS denied call, got %d", len(pub.deniedCalls))
		}
	})

	t.Run("Swipe Badge - Inactive Badge", func(t *testing.T) {
		repo := &mockRepository{
			doors: map[string]accesscontrol.Door{
				"door-1": {
					ID:     "door-1",
					SiteID: "site-1",
				},
			},
			badges: map[string]accesscontrol.Badge{
				"badge-1": {
					ID:     "badge-1",
					Number: "INACTIVE_CARD",
					Status: "revoked",
				},
			},
		}
		pub := &mockPublisher{}
		svc := accesscontrol.NewService(repo, pub, logger)

		resp, err := svc.SwipeBadge(ctx, "door-1", "INACTIVE_CARD")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if resp.AccessGranted {
			t.Error("expected access to be denied")
		}

		if len(repo.logs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(repo.logs))
		}
		if *repo.logs[0].DeniedReason != "badge is revoked" {
			t.Errorf("expected reason 'badge is revoked', got '%s'", *repo.logs[0].DeniedReason)
		}
	})

	t.Run("Swipe Badge - No Permission", func(t *testing.T) {
		repo := &mockRepository{
			doors: map[string]accesscontrol.Door{
				"door-1": {
					ID:     "door-1",
					SiteID: "site-1",
					Name:   "Front Door",
				},
			},
			badges: map[string]accesscontrol.Badge{
				"badge-1": {
					ID:     "badge-1",
					Number: "CARD_NO_PERM",
					Status: "active",
				},
			},
			permissions: make(map[string]bool),
		}
		pub := &mockPublisher{}
		svc := accesscontrol.NewService(repo, pub, logger)

		resp, err := svc.SwipeBadge(ctx, "door-1", "CARD_NO_PERM")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if resp.AccessGranted {
			t.Error("expected access to be denied")
		}

		if len(repo.logs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(repo.logs))
		}
		if repo.logs[0].AccessType != "denied" {
			t.Errorf("expected access log to be denied, got %s", repo.logs[0].AccessType)
		}
	})

	t.Run("Swipe Badge - Access Granted Success", func(t *testing.T) {
		repo := &mockRepository{
			doors: map[string]accesscontrol.Door{
				"door-1": {
					ID:     "door-1",
					SiteID: "site-1",
					Name:   "Front Door",
				},
			},
			badges: map[string]accesscontrol.Badge{
				"badge-1": {
					ID:     "badge-1",
					Number: "CARD_OK",
					Status: "active",
					UserID: func() *string { s := "user-1"; return &s }(),
				},
			},
			permissions: map[string]bool{
				"badge-1_door-1": true,
			},
		}
		pub := &mockPublisher{}
		svc := accesscontrol.NewService(repo, pub, logger)

		resp, err := svc.SwipeBadge(ctx, "door-1", "CARD_OK")
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !resp.AccessGranted {
			t.Error("expected access to be granted")
		}

		if len(repo.logs) != 1 {
			t.Fatalf("expected 1 access log, got %d", len(repo.logs))
		}
		if repo.logs[0].AccessType != "granted" {
			t.Errorf("expected access log to be granted, got %s", repo.logs[0].AccessType)
		}
		if repo.logs[0].DeniedReason != nil {
			t.Errorf("expected nil denied reason, got %s", *repo.logs[0].DeniedReason)
		}

		if len(pub.grantedCalls) != 1 {
			t.Errorf("expected 1 NATS granted call, got %d", len(pub.grantedCalls))
		}
	})
}
