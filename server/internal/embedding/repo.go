package embedding

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

func NewRepo(db *gorm.DB, key crypto.EncryptKey) *Repo {
	return &Repo{db: db, key: key}
}

func (r *Repo) Create(p *EmbeddingModel) error {
	plain := p.APIKey
	p.APIKey = crypto.Encrypt(r.key, plain)
	if err := r.db.Create(p).Error; err != nil {
		p.APIKey = plain
		return err
	}
	p.APIKey = plain
	return nil
}

func (r *Repo) FindByID(tenantID, id string) (*EmbeddingModel, error) {
	var p EmbeddingModel
	err := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("embedding provider not found")
		}
		return nil, err
	}
	p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	return &p, nil
}

func (r *Repo) FindDefault(tenantID string) (*EmbeddingModel, error) {
	var p EmbeddingModel
	err := r.db.Where("tenant_id = ? AND is_default = ? AND status = ?",
		tenantID, true, StatusActive).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("no default embedding provider configured")
		}
		return nil, err
	}
	p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	return &p, nil
}

// FindByModel returns the tenant's provider matching the given model name.
// Used by the resolver when a KB pins a specific embedding model.
func (r *Repo) FindByModel(tenantID, model string) (*EmbeddingModel, error) {
	var p EmbeddingModel
	err := r.db.Where("tenant_id = ? AND model = ? AND status = ?",
		tenantID, model, StatusActive).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("embedding provider not found for model: " + model)
		}
		return nil, err
	}
	p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	return &p, nil
}

func (r *Repo) List(tenantID string, page, size int) ([]*EmbeddingModel, int64, error) {
	var items []*EmbeddingModel
	var total int64
	q := r.db.Model(&EmbeddingModel{}).Where("tenant_id = ?", tenantID)
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

func (r *Repo) Update(p *EmbeddingModel) error {
	plain := p.APIKey
	p.APIKey = crypto.Encrypt(r.key, plain)
	if err := r.db.Save(p).Error; err != nil {
		p.APIKey = plain
		return err
	}
	p.APIKey = plain
	return nil
}

func (r *Repo) ClearDefault(tenantID, exceptID string) error {
	return r.db.Model(&EmbeddingModel{}).
		Where("tenant_id = ? AND id <> ?", tenantID, exceptID).
		Update("is_default", false).Error
}

// UpdateTestResult records the last test outcome without touching other
// fields (avoids re-encrypting the API key).
func (r *Repo) UpdateTestResult(tenantID, id, status string, at time.Time) error {
	return r.db.Model(&EmbeddingModel{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(map[string]interface{}{
			"last_tested_at":   at,
			"last_test_status": status,
		}).Error
}

func (r *Repo) Delete(tenantID, id string) error {
	res := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&EmbeddingModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("embedding model not found")
	}
	return nil
}

// ListByProvider returns the rows grouped under one provider card, oldest
// first so the card shows a stable order.
func (r *Repo) ListByProvider(tenantID, providerID string) ([]*EmbeddingModel, error) {
	var items []*EmbeddingModel
	if err := r.db.Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).
		Order("created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	for _, p := range items {
		p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	}
	return items, nil
}

// DeleteByProvider removes every row bound to a provider card (card delete).
func (r *Repo) DeleteByProvider(tenantID, providerID string) error {
	return r.db.Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).
		Delete(&EmbeddingModel{}).Error
}

// UpdateProviderCreds re-points rows bound to a provider card at the card's
// current endpoint/key, keeping rows self-contained for the call path.
func (r *Repo) UpdateProviderCreds(tenantID, providerID, endpoint, apiKey string) error {
	return r.db.Model(&EmbeddingModel{}).
		Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).
		Updates(map[string]interface{}{
			"endpoint": endpoint,
			"api_key":  crypto.Encrypt(r.key, apiKey),
		}).Error
}
