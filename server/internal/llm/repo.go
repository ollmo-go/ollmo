package llm

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"ollmo/ollmo/pkg/crypto"
	"ollmo/ollmo/pkg/errs"
)

type Repo struct {
	db  *gorm.DB
	key crypto.EncryptKey
}

// NewRepo creates a repo. key is optional; when nil API keys are stored and
// returned as plaintext (useful for tests and local dev without a secret).
func NewRepo(db *gorm.DB, key crypto.EncryptKey) *Repo {
	return &Repo{db: db, key: key}
}

func (r *Repo) Create(p *LLMModel) error {
	plain := p.APIKey
	p.APIKey = crypto.Encrypt(r.key, plain)
	if err := r.db.Create(p).Error; err != nil {
		p.APIKey = plain
		return err
	}
	p.APIKey = plain
	return nil
}

func (r *Repo) FindByID(tenantID, id string) (*LLMModel, error) {
	var p LLMModel
	err := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("llm provider not found")
		}
		return nil, err
	}
	p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	return &p, nil
}

// FindDefault returns the tenant's default provider. Used by chat when the
// caller does not specify one.
func (r *Repo) FindDefault(tenantID string) (*LLMModel, error) {
	var p LLMModel
	err := r.db.Where("tenant_id = ? AND is_default = ? AND status = ?",
		tenantID, true, StatusActive).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("no default llm provider configured")
		}
		return nil, err
	}
	p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	return &p, nil
}

func (r *Repo) List(tenantID string, page, size int) ([]*LLMModel, int64, error) {
	var items []*LLMModel
	var total int64
	q := r.db.Model(&LLMModel{}).Where("tenant_id = ?", tenantID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	for _, p := range items {
		p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	}
	return items, total, nil
}

func (r *Repo) Update(p *LLMModel) error {
	plain := p.APIKey
	p.APIKey = crypto.Encrypt(r.key, plain)
	if err := r.db.Save(p).Error; err != nil {
		p.APIKey = plain
		return err
	}
	p.APIKey = plain
	return nil
}

// ClearDefault unsets is_default on every other provider in the tenant. Used
// when promoting a provider to default so only one is active at a time.
func (r *Repo) ClearDefault(tenantID, exceptID string) error {
	return r.db.Model(&LLMModel{}).
		Where("tenant_id = ? AND id <> ?", tenantID, exceptID).
		Update("is_default", false).Error
}

// UpdateTestResult records the last test outcome without touching other
// fields (avoids re-encrypting the API key).
func (r *Repo) UpdateTestResult(tenantID, id, status string, at time.Time) error {
	return r.db.Model(&LLMModel{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(map[string]interface{}{
			"last_tested_at":   at,
			"last_test_status": status,
		}).Error
}

func (r *Repo) Delete(tenantID, id string) error {
	res := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&LLMModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("llm provider not found")
	}
	return nil
}
