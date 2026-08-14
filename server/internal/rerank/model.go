package rerank

import "time"

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// RerankModel is a tenant-scoped rerank model configuration. Endpoint and
// APIKey are Cohere-style /v1/rerank compatible (SiliconFlow, Jina, TEI).
type RerankModel struct {
	ID             string     `gorm:"primaryKey;size:36" json:"id"`
	TenantID       string     `gorm:"size:36;index;not null" json:"tenant_id"`
	Name           string     `gorm:"size:128;not null" json:"name"`
	Endpoint       string     `gorm:"size:512;not null" json:"endpoint"`
	APIKey         string     `gorm:"size:512" json:"api_key,omitempty"`
	Model          string     `gorm:"size:128;not null" json:"model"`
	TopN           int        `gorm:"not null;default:10" json:"top_n"`
	IsDefault      bool       `gorm:"not null;default:false" json:"is_default"`
	OwnerID        string     `gorm:"size:36;index;not null" json:"owner_id"`
	Status         string     `gorm:"size:32;not null;default:active" json:"status"`
	LastTestedAt   *time.Time `json:"last_tested_at"`
	LastTestStatus string     `gorm:"size:16;default:''" json:"last_test_status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName maps RerankModel to the rerank_models table.
func (RerankModel) TableName() string { return "rerank_models" }
