package embedding

import "time"

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// EmbeddingModel is a tenant-scoped embedding model configuration. Endpoint
// and APIKey are OpenAI-compatible /v1/embeddings. Dim is auto-derived from
// the model name via vector.EmbeddingDim at creation time.
type EmbeddingModel struct {
	ID             string     `gorm:"primaryKey;size:36" json:"id"`
	TenantID       string     `gorm:"size:36;index;not null" json:"tenant_id"`
	Name           string     `gorm:"size:128;not null" json:"name"`
	Endpoint       string     `gorm:"size:512;not null" json:"endpoint"`
	APIKey         string     `gorm:"size:512" json:"api_key,omitempty"`
	Model          string     `gorm:"size:128;not null" json:"model"`
	Dim            int        `gorm:"not null" json:"dim"`
	BatchSize      int        `gorm:"not null;default:32" json:"batch_size"`
	IsDefault      bool       `gorm:"not null;default:false" json:"is_default"`
	OwnerID        string     `gorm:"size:36;index;not null" json:"owner_id"`
	Status         string     `gorm:"size:32;not null;default:active" json:"status"`
	LastTestedAt   *time.Time `json:"last_tested_at"`
	LastTestStatus string     `gorm:"size:16;default:''" json:"last_test_status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName maps EmbeddingModel to the embedding_models table.
func (EmbeddingModel) TableName() string { return "embedding_models" }
