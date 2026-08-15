package kb

import (
	"errors"
	"time"

	"ollmo/ollmo/pkg/cache"
	"ollmo/ollmo/pkg/errs"

	"gorm.io/gorm"
)

// rowTTL keeps hot-path KB reads (chat turn: annotation match + each
// retrieval node + prompt assembly) on one DB hit per window. All writers
// below invalidate, so staleness is bounded by this TTL.
const rowTTL = 30 * time.Second

type Repo struct {
	db    *gorm.DB
	cache *cache.TTL[*KnowledgeBase]
}

func NewRepo(db *gorm.DB) *Repo {
	return &Repo{db: db, cache: cache.NewTTL[*KnowledgeBase](rowTTL)}
}

func (r *Repo) Create(k *KnowledgeBase) error { return r.db.Create(k).Error }

func (r *Repo) FindByID(tenantID, id string) (*KnowledgeBase, error) {
	key := tenantID + "|" + id
	if k, ok := r.cache.Get(key); ok {
		c := *k // copy so callers may mutate freely
		return &c, nil
	}
	var k KnowledgeBase
	err := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&k).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("knowledge base not found")
		}
		return nil, err
	}
	r.cache.Set(key, &k)
	c := k
	return &c, nil
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

func (r *Repo) Update(k *KnowledgeBase) error {
	if err := r.db.Save(k).Error; err != nil {
		return err
	}
	r.cache.Delete(k.TenantID + "|" + k.ID)
	return nil
}

func (r *Repo) Delete(tenantID, id string) error {
	res := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&KnowledgeBase{})
	if res.Error != nil {
		return res.Error
	}
	r.cache.Delete(tenantID + "|" + id)
	if res.RowsAffected == 0 {
		return errs.NotFound("knowledge base not found")
	}
	return nil
}

// IncDocCount adjusts doc_count atomically. delta may be negative.
func (r *Repo) IncDocCount(tenantID, id string, delta int) error {
	if err := r.db.Model(&KnowledgeBase{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		UpdateColumn("doc_count", gorm.Expr("doc_count + ?", delta)).Error; err != nil {
		return err
	}
	// doc_count gates the empty-KB retrieval shortcut, so drop the row now.
	r.cache.Delete(tenantID + "|" + id)
	return nil
}
