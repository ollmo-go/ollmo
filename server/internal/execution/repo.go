package execution

import (
	"ollmo/ollmo/pkg/errs"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Create inserts one execution record.
func (r *Repo) Create(e *Execution) error {
	return r.db.Create(e).Error
}

// List returns executions for a KB, newest first. source filters chat/test
// runs; empty returns both.
func (r *Repo) List(tenantID, kbID, source string, page, size int) ([]*Execution, int64, error) {
	q := r.db.Where("tenant_id = ? AND kb_id = ?", tenantID, kbID)
	if source != "" {
		q = q.Where("source = ?", source)
	}
	var total int64
	if err := q.Model(&Execution{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*Execution
	if err := q.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Find returns one execution by id, tenant-scoped.
func (r *Repo) Find(tenantID, id string) (*Execution, error) {
	var e Execution
	if err := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&e).Error; err != nil {
		return nil, errs.NotFound("execution not found")
	}
	return &e, nil
}

// FindByMessageID returns the execution that produced one assistant message.
// Used by the analytics feedback list to deep-link a bad case into its replay.
// Records without a persisted message (message_id = ”) never match since the
// caller always passes a non-empty uuid.
func (r *Repo) FindByMessageID(tenantID, messageID string) (*Execution, error) {
	var e Execution
	if err := r.db.Where("tenant_id = ? AND message_id = ?", tenantID, messageID).First(&e).Error; err != nil {
		return nil, errs.NotFound("execution not found")
	}
	return &e, nil
}
