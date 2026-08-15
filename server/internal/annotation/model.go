package annotation

import "time"

// Annotation is a curated question/answer pair attached to a KB. When a user
// question closely matches an annotation question (cosine similarity over the
// annotation embedding collection), the chat stream short-circuits and
// returns the stored answer verbatim instead of running retrieval + LLM.
type Annotation struct {
	ID        string    `gorm:"size:36;primaryKey" json:"id"`
	TenantID  string    `gorm:"size:36;index:idx_annotation_tenant_kb;not null" json:"tenant_id"`
	KbID      string    `gorm:"size:36;index:idx_annotation_tenant_kb;not null" json:"kb_id"`
	Question  string    `gorm:"type:text;not null" json:"question"`
	Answer    string    `gorm:"type:text;not null" json:"answer"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
