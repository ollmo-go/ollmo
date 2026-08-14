package audit

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(log *AuditLog) error {
	if log.ID == "" {
		log.ID = uuid.NewString()
	}
	return r.db.Create(log).Error
}

// List returns audit logs for a tenant, optionally filtered by user_id and
// action prefix. Results are newest-first.
func (r *Repo) List(tenantID, userID, action string, page, size int) ([]*AuditLog, int64, error) {
	var items []*AuditLog
	var total int64
	q := r.db.Model(&AuditLog{}).Where("tenant_id = ?", tenantID)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if action != "" {
		q = q.Where("action LIKE ?", action+"%")
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error
	return items, total, err
}
