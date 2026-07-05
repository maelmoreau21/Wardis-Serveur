package layout

import (
	"context"
	"go.uber.org/zap"
)

type Service interface {
	List(ctx context.Context, userID string) ([]SavedView, error)
	GetByID(ctx context.Context, id string) (*SavedView, error)
	Create(ctx context.Context, userID string, req SavedViewRequest) (*SavedView, error)
	Update(ctx context.Context, id string, req SavedViewRequest) (*SavedView, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo Repository
	log  *zap.Logger
}

func NewService(repo Repository, log *zap.Logger) Service {
	return &service{
		repo: repo,
		log:  log,
	}
}

func (s *service) List(ctx context.Context, userID string) ([]SavedView, error) {
	return s.repo.List(ctx, userID)
}

func (s *service) GetByID(ctx context.Context, id string) (*SavedView, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) Create(ctx context.Context, userID string, req SavedViewRequest) (*SavedView, error) {
	v := &SavedView{
		Name:       req.Name,
		GridLayout: req.GridLayout,
		Slots:      req.Slots,
	}
	if userID != "" {
		v.UserID = &userID
	}
	return s.repo.Create(ctx, v)
}

func (s *service) Update(ctx context.Context, id string, req SavedViewRequest) (*SavedView, error) {
	v := &SavedView{
		Name:       req.Name,
		GridLayout: req.GridLayout,
		Slots:      req.Slots,
	}
	return s.repo.Update(ctx, id, v)
}

func (s *service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
