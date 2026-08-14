package memory

import "gorm.io/gorm"

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Create inserts a new memory row.
func (r *Repo) Create(m *Memory) error {
	return r.db.Create(m).Error
}

// FindByConversation returns the memory for a conversation, if one exists.
func (r *Repo) FindByConversation(tenantID, convID string) (*Memory, error) {
	var m Memory
	err := r.db.Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListByKB returns a user's memory summaries for a KB, most recent first.
// Memories are scoped per user: on shared KBs, one member's summaries must
// not surface to another.
func (r *Repo) ListByKB(tenantID, userID, kbID string, page, size int) ([]*Memory, int64, error) {
	var items []*Memory
	var total int64
	q := r.db.Model(&Memory{}).Where("tenant_id = ? AND user_id = ? AND kb_id = ?", tenantID, userID, kbID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").
		Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

// ListActiveByKB returns a user's active memories for a KB, most recent
// first. Used to build prompt context; disabled/forgotten memories are
// excluded, and summaries from other members of a shared KB never leak
// into the prompt.
func (r *Repo) ListActiveByKB(tenantID, userID, kbID string, limit int) ([]*Memory, error) {
	var items []*Memory
	err := r.db.Where("tenant_id = ? AND user_id = ? AND kb_id = ? AND status = ?", tenantID, userID, kbID, StatusActive).
		Order("created_at DESC").Limit(limit).Find(&items).Error
	return items, err
}

// UpdateStatus sets the status of a memory row (active/disabled/forgotten).
func (r *Repo) UpdateStatus(tenantID, kbID, id, status string) error {
	return r.db.Model(&Memory{}).
		Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, id).
		Update("status", status).Error
}

// Delete removes a memory row. Scoped by kb_id so a caller with access to
// one KB cannot delete memories belonging to another.
func (r *Repo) Delete(tenantID, kbID, id string) error {
	return r.db.Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, id).Delete(&Memory{}).Error
}

// DeleteByConversation removes all memories attached to a conversation. Used
// when the conversation itself is deleted so no orphaned summaries keep being
// injected into future prompts.
func (r *Repo) DeleteByConversation(tenantID, convID string) error {
	return r.db.Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).Delete(&Memory{}).Error
}
