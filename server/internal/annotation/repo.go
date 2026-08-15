package annotation

import (
	"errors"

	"gorm.io/gorm"

	"ollmo/ollmo/pkg/errs"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(a *Annotation) error { return r.db.Create(a).Error }

// Find locates an annotation scoped to tenant AND knowledge base; the kbID
// condition prevents cross-KB IDOR on annotation IDs.
func (r *Repo) Find(tenantID, kbID, id string) (*Annotation, error) {
	var a Annotation
	err := r.db.Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, id).First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("annotation not found")
		}
		return nil, err
	}
	return &a, nil
}

func (r *Repo) List(tenantID, kbID string, page, size int) ([]*Annotation, int64, error) {
	var items []*Annotation
	var total int64
	q := r.db.Model(&Annotation{}).Where("tenant_id = ? AND kb_id = ?", tenantID, kbID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error
	return items, total, err
}

// ListEnabled returns every enabled annotation for a KB; used when the KB's
// embedding model changes and all question vectors must be rebuilt.
func (r *Repo) ListEnabled(tenantID, kbID string) ([]*Annotation, error) {
	var items []*Annotation
	err := r.db.Where("tenant_id = ? AND kb_id = ? AND enabled = ?", tenantID, kbID, true).
		Find(&items).Error
	return items, err
}

// HasEnabled reports whether the KB has any enabled annotation. Match calls
// this before embedding the query so KBs without annotations skip the
// embedding API call and the doomed Milvus search entirely.
func (r *Repo) HasEnabled(tenantID, kbID string) (bool, error) {
	var n int64
	err := r.db.Model(&Annotation{}).
		Where("tenant_id = ? AND kb_id = ? AND enabled = ?", tenantID, kbID, true).
		Limit(1).Count(&n).Error
	return n > 0, err
}

func (r *Repo) Update(tenantID, kbID, id string, cols map[string]any) error {
	res := r.db.Model(&Annotation{}).
		Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, id).
		Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("annotation not found")
	}
	return nil
}

func (r *Repo) Delete(tenantID, kbID, id string) error {
	res := r.db.Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, id).
		Delete(&Annotation{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("annotation not found")
	}
	return nil
}

// CleanupByKB removes all annotation rows for a KB (KB deletion).
func (r *Repo) CleanupByKB(tenantID, kbID string) error {
	return r.db.Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Delete(&Annotation{}).Error
}
