package agent

import (
	"errors"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// FindByKB returns the active agent definition for a KB.
func (r *Repo) FindByKB(tenantID, kbID string) (*Agent, error) {
	var a Agent
	err := r.db.Where("tenant_id = ? AND kb_id = ? AND active = ?", tenantID, kbID, true).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &a, err
}

// FindByKBID returns the active agent definition for a KB by kb_id only,
// ignoring tenant. Used as a fallback when Create fails on the kb_id unique
// index (e.g. the row was created by a different tenant in a shared KB).
func (r *Repo) FindByKBID(kbID string) (*Agent, error) {
	var a Agent
	err := r.db.Where("kb_id = ? AND active = ?", kbID, true).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &a, err
}

// Create inserts a new agent definition.
func (r *Repo) Create(a *Agent) error {
	return r.db.Create(a).Error
}

// Update replaces the definition and bumps the version.
func (r *Repo) Update(a *Agent) error {
	return r.db.Save(a).Error
}
