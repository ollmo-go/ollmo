package memory

import "time"

// Memory type and status constants. Type distinguishes episodic memories
// (conversation summaries, the current implementation) from semantic memories
// (user facts/preferences, planned). Status mirrors the RAGFlow model:
// active memories are injected into prompts, disabled ones are kept but
// ignored, forgotten ones are soft-deleted.
const (
	TypeEpisodic = "episodic"
	TypeSemantic = "semantic"

	StatusActive    = "active"
	StatusDisabled  = "disabled"
	StatusForgotten = "forgotten"

	SourceManual = "manual"
	SourceAuto   = "auto"
)

// Memory stores a conversation summary so future chats in the same KB can
// reference past context without re-loading the full message history. One
// row per summarized conversation; KB-level context is built by concatenating
// all memory rows for that KB, ordered by recency.
type Memory struct {
	ID             string `gorm:"primaryKey;size:36" json:"id"`
	TenantID       string `gorm:"size:36;index:idx_mem_tenant_kb_created,priority:1;index:idx_mem_tenant_conv,priority:1;not null" json:"tenant_id"`
	KbID           string `gorm:"size:36;index:idx_mem_tenant_kb_created,priority:2;not null" json:"kb_id"`
	ConversationID string `gorm:"size:36;index:idx_mem_tenant_conv,priority:2" json:"conversation_id"`
	UserID         string `gorm:"size:36;index" json:"user_id"`
	Type           string `gorm:"size:32;default:episodic" json:"type"`
	Status         string `gorm:"size:32;default:active" json:"status"`
	Source         string `gorm:"size:32;default:manual" json:"source"`
	// MessageCount records how many messages existed when the summary was
	// generated. Auto-summarization re-triggers only when the conversation
	// grows by another threshold-sized batch beyond this count.
	MessageCount int       `gorm:"default:0" json:"message_count"`
	Title        string    `gorm:"size:255" json:"title"`
	Summary      string    `gorm:"type:text" json:"summary"`
	KeyPoints    string    `gorm:"type:text" json:"key_points,omitempty"` // JSON array of strings
	CreatedAt    time.Time `gorm:"index:idx_mem_tenant_kb_created,priority:3" json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Memory) TableName() string { return "memories" }
