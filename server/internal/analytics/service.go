package analytics

import (
	"context"
)

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

func (s *Service) Overview(ctx context.Context, tenantID string) (*Overview, error) {
	return s.repo.Overview(tenantID)
}

func (s *Service) DocStats(ctx context.Context, tenantID string) (*DocStats, error) {
	return s.repo.DocStats(tenantID)
}

func (s *Service) KBUsage(ctx context.Context, tenantID string) ([]KBUsage, error) {
	return s.repo.KBUsage(tenantID)
}

func (s *Service) RecentActivity(ctx context.Context, tenantID string, limit int) ([]ActivityItem, error) {
	return s.repo.RecentActivity(tenantID, limit)
}
