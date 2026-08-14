package kb

import (
	"errors"

	"ollmo/ollmo/pkg/errs"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(k *KnowledgeBase) error { return r.db.Create(k).Error }

func (r *Repo) FindByID(tenantID, id string) (*KnowledgeBase, error) {
	var k KnowledgeBase
	err := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&k).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("knowledge base not found")
		}
		return nil, err
	}
	return &k, nil
}

// List returns KBs the user owns, is a member of, or are team-visible.
func (r *Repo) List(tenantID, userID string, page, size int) ([]*KnowledgeBase, int64, error) {
	var items []*KnowledgeBase
	var total int64
	q := r.db.Model(&KnowledgeBase{}).Where(
		"tenant_id = ? AND (owner_id = ? OR visibility = ?)",
		tenantID, userID, VisibilityTeam,
	)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error
	return items, total, err
}

func (r *Repo) Update(k *KnowledgeBase) error { return r.db.Save(k).Error }

func (r *Repo) Delete(tenantID, id string) error {
	res := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&KnowledgeBase{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("knowledge base not found")
	}
	return nil
}

// IncDocCount adjusts doc_count atomically. delta may be negative.
func (r *Repo) IncDocCount(tenantID, id string, delta int) error {
	return r.db.Model(&KnowledgeBase{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		UpdateColumn("doc_count", gorm.Expr("doc_count + ?", delta)).Error
}
