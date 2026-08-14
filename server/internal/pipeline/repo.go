package pipeline

import (
	"errors"

	"ollmo/ollmo/pkg/errs"
	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Upsert inserts or replaces the pipeline for a KB. We always update the same
// row (unique on kb_id) so the version counter advances in place.
func (r *Repo) Upsert(p *Pipeline) error {
	// Find existing to bump version; if absent, create fresh.
	var existing Pipeline
	err := r.db.Where("tenant_id = ? AND kb_id = ?", p.TenantID, p.KbID).First(&existing).Error
	if err == nil {
		existing.Version += 1
		existing.Definition = p.Definition
		existing.Active = true
		return r.db.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.Create(p).Error
}

func (r *Repo) FindByKB(tenantID, kbID string) (*Pipeline, error) {
	var p Pipeline
	err := r.db.Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("pipeline not found")
		}
		return nil, err
	}
	return &p, nil
}

func (r *Repo) DeleteByKB(tenantID, kbID string) error {
	return r.db.Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).Delete(&Pipeline{}).Error
}
