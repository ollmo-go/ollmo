package audit

import (
	"context"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

// Record writes an audit log entry. It never returns an error so callers can
// fire-and-forget; a failed audit write must not break the request.
func (s *Service) Record(ctx context.Context, tenantID, userID, userName, action, resource, detail, ip string) {
	_ = s.repo.Create(&AuditLog{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		UserID:   userID,
		UserName: userName,
		Action:   action,
		Resource: resource,
		Detail:   detail,
		IP:       ip,
	})
}

func (s *Service) List(ctx context.Context, tenantID, userID, action string, page, size int) ([]*AuditLog, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 50
	}
	return s.repo.List(tenantID, userID, action, page, size)
}
