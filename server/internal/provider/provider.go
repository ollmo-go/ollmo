package provider

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ollmo/ollmo/pkg/crypto"
	"ollmo/ollmo/pkg/errs"
)

// Provider is one settings card: an endpoint plus one credential, grouping
// model rows of any kind (chat/embedding/rerank). Endpoint and APIKey are
// copied onto each bound model row, so this table only drives the settings
// UI; the call path never reads it.
type Provider struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	TenantID  string `gorm:"size:36;index:idx_prov_tenant;not null" json:"tenant_id"`
	CatalogID string `gorm:"size:32;not null;default:custom" json:"catalog_id"`
	Name      string `gorm:"size:128;not null" json:"name"`
	Endpoint  string `gorm:"size:512;not null" json:"endpoint"`
	// APIKey is stored encrypted; API responses expose only HasKey.
	APIKey    string    `gorm:"size:512" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName maps Provider to the model_providers table.
func (Provider) TableName() string { return "model_providers" }

type Repo struct {
	db  *gorm.DB
	key crypto.EncryptKey
}

func NewRepo(db *gorm.DB, key crypto.EncryptKey) *Repo {
	return &Repo{db: db, key: key}
}

func (r *Repo) Create(p *Provider) error {
	plain := p.APIKey
	p.APIKey = crypto.Encrypt(r.key, plain)
	if err := r.db.Create(p).Error; err != nil {
		p.APIKey = plain
		return err
	}
	p.APIKey = plain
	return nil
}

func (r *Repo) FindByID(tenantID, id string) (*Provider, error) {
	var p Provider
	err := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&p).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errs.NotFound("provider not found")
		}
		return nil, err
	}
	p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	return &p, nil
}

func (r *Repo) List(tenantID string) ([]*Provider, error) {
	var items []*Provider
	if err := r.db.Where("tenant_id = ?", tenantID).
		Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	for _, p := range items {
		p.APIKey = crypto.Decrypt(r.key, p.APIKey)
	}
	return items, nil
}

// FindByEndpoint returns the first provider using the same endpoint, or nil
// when none exists. Used to reject duplicate provider cards.
func (r *Repo) FindByEndpoint(tenantID, endpoint string) (*Provider, error) {
	var p Provider
	err := r.db.Where("tenant_id = ? AND endpoint = ?", tenantID, endpoint).First(&p).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Save persists name/endpoint/APIKey (APIKey must be the plaintext value).
func (r *Repo) Save(p *Provider) error {
	plain := p.APIKey
	p.APIKey = crypto.Encrypt(r.key, plain)
	if err := r.db.Save(p).Error; err != nil {
		p.APIKey = plain
		return err
	}
	p.APIKey = plain
	return nil
}

func (r *Repo) Delete(tenantID, id string) error {
	res := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&Provider{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("provider not found")
	}
	return nil
}

// NewID exposes ID generation to the service layer.
func NewID() string { return uuid.NewString() }

// CountKBsByEmbedding counts knowledge bases whose pinned embedding model is
// modelID. Used to reject deleting an embedding model still in use.
func (r *Repo) CountKBsByEmbedding(tenantID, modelID string) (int64, error) {
	if modelID == "" {
		return 0, nil
	}
	var n int64
	err := r.db.Table("knowledge_bases").
		Where("tenant_id = ? AND embedding_model_id = ?", tenantID, modelID).
		Count(&n).Error
	return n, err
}

// CountAgentUsages counts active agents whose graph references modelID as a
// node's llm_model_id or rerank_model_id. Used to reject deleting a chat or
// rerank model still wired into an agent graph.
func (r *Repo) CountAgentUsages(tenantID, modelID string) (int64, error) {
	if modelID == "" {
		return 0, nil
	}
	var defs []string
	if err := r.db.Table("agents").
		Where("tenant_id = ? AND active = ?", tenantID, true).
		Pluck("definition", &defs).Error; err != nil {
		return 0, err
	}
	var n int64
	for _, d := range defs {
		if d == "" {
			continue
		}
		var def struct {
			Nodes []struct {
				Data map[string]interface{} `json:"data"`
			} `json:"nodes"`
		}
		if err := json.Unmarshal([]byte(d), &def); err != nil {
			continue
		}
		for _, nd := range def.Nodes {
			if v, ok := nd.Data["llm_model_id"].(string); ok && v == modelID {
				n++
				break
			}
			if v, ok := nd.Data["rerank_model_id"].(string); ok && v == modelID {
				n++
				break
			}
		}
	}
	return n, nil
}
