package kb

import "time"

// KnowledgeBase is a tenant-scoped collection of documents. The embedding
// model is pinned here (by id reference to embedding_models) so the Milvus
// collection dimension is fixed at creation.
type KnowledgeBase struct {
	ID               string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID         string    `gorm:"size:36;index;not null" json:"tenant_id"`
	Name             string    `gorm:"size:128;not null" json:"name"`
	Description      string    `gorm:"size:512" json:"description"`
	EmbeddingModelID string    `gorm:"column:embedding_model_id;size:36;not null" json:"embedding_model_id"`
	ChunkStrategy    string    `gorm:"size:32;not null;default:parent_child" json:"chunk_strategy"`
	ChunkSize        int       `gorm:"not null;default:500" json:"chunk_size"`
	ChunkOverlap     int       `gorm:"not null;default:50" json:"chunk_overlap"`
	DocCount         int       `gorm:"not null;default:0" json:"doc_count"`
	OwnerID          string    `gorm:"size:36;index;not null" json:"owner_id"`
	Visibility       string    `gorm:"size:32;not null;default:private" json:"visibility"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (KnowledgeBase) TableName() string { return "knowledge_bases" }

const (
	VisibilityPrivate = "private"
	VisibilityTeam    = "team"
)
