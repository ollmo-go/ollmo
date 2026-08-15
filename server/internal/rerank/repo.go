package rerank

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

func (r *Repo) Create(p *RerankModel) error {
	plain := p.APIKey
	p.APIKey = crypto.Encrypt(r.key, plain)
	if err := r.db.Create(p).Error; err != nil {
		p.APIKey = plain
		return err
	}
	p.APIKey = plain
	return nil
}

func (r *Repo) FindByID(tenantID, id string) (*RerankModel, error) {
	var p RerankModel
	err := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("rerank provider not found")
		}
		return nil, err
	}
	p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	return &p, nil
}

func (r *Repo) FindDefault(tenantID string) (*RerankModel, error) {
	var p RerankModel
	err := r.db.Where("tenant_id = ? AND is_default = ? AND status = ?",
		tenantID, true, StatusActive).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("no default rerank provider configured")
		}
		return nil, err
	}
	p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	return &p, nil
}

func (r *Repo) List(tenantID string, page, size int) ([]*RerankModel, int64, error) {
	var items []*RerankModel
	var total int64
	q := r.db.Model(&RerankModel{}).Where("tenant_id = ?", tenantID)
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

func (r *Repo) Update(p *RerankModel) error {
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
	return r.db.Model(&RerankModel{}).
		Where("tenant_id = ? AND id <> ?", tenantID, exceptID).
		Update("is_default", false).Error
}

// UpdateTestResult records the last test outcome without touching other
// fields (avoids re-encrypting the API key).
func (r *Repo) UpdateTestResult(tenantID, id, status string, at time.Time) error {
	return r.db.Model(&RerankModel{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(map[string]interface{}{
			"last_tested_at":   at,
			"last_test_status": status,
		}).Error
}

func (r *Repo) Delete(tenantID, id string) error {
	res := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&RerankModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("rerank provider not found")
	}
	return nil
}

// ListByProvider returns the rows grouped under one provider card, oldest
// first so the card shows a stable order.
func (r *Repo) ListByProvider(tenantID, providerID string) ([]*RerankModel, error) {
	var items []*RerankModel
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
		Delete(&RerankModel{}).Error
}

// UpdateProviderCreds re-points rows bound to a provider card at the card's
// current endpoint/key, keeping rows self-contained for the call path.
func (r *Repo) UpdateProviderCreds(tenantID, providerID, endpoint, apiKey string) error {
	return r.db.Model(&RerankModel{}).
		Where("tenant_id = ? AND provider_id = ?", tenantID, providerID).
		Updates(map[string]interface{}{
			"endpoint": endpoint,
			"api_key":  crypto.Encrypt(r.key, apiKey),
		}).Error
}
