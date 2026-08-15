package chat

import "time"

// Message roles map to the OpenAI chat message roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Conversation is a chat session scoped to a knowledge base.
type Conversation struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID  string    `gorm:"size:36;index:idx_conv_tenant_owner,priority:1;not null" json:"tenant_id"`
	KbID      string    `gorm:"size:36;index;not null" json:"kb_id"`
	Title     string    `gorm:"size:255;not null" json:"title"`
	OwnerID   string    `gorm:"size:36;index:idx_conv_tenant_owner,priority:2;not null" json:"owner_id"`
	Pinned    bool      `gorm:"not null;default:false" json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message is one turn in a conversation. Citations is a JSON-encoded
// []Citation stored as a string; it is empty for non-assistant messages.
type Message struct {
	ID             string `gorm:"primaryKey;size:36" json:"id"`
	TenantID       string `gorm:"size:36;index:idx_msg_tenant_conv_created,priority:1;index:idx_msg_tenant_role_vote,priority:1;not null" json:"tenant_id"`
	ConversationID string `gorm:"size:36;index:idx_msg_tenant_conv_created,priority:2;not null" json:"conversation_id"`
	Role           string `gorm:"size:32;not null;index:idx_msg_tenant_role_vote,priority:2" json:"role"`
	Content        string `gorm:"type:text" json:"content"`
	Reasoning      string `gorm:"type:text" json:"reasoning,omitempty"`
	Citations      string `gorm:"type:text" json:"citations,omitempty"`
	// Annotation marks assistant messages that came from a curated annotation
	// reply match rather than LLM generation. Persisted so the "标注回复"
	// label survives page reloads and history browsing.
	Annotation       bool `gorm:"not null;default:false" json:"annotation,omitempty"`
	RetrieveMs       int  `gorm:"default:0" json:"retrieve_ms,omitempty"`
	GenerateMs       int  `gorm:"default:0" json:"generate_ms,omitempty"`
	TotalMs          int  `gorm:"default:0" json:"total_ms,omitempty"`
	PromptTokens     int  `gorm:"default:0" json:"prompt_tokens,omitempty"`
	CompletionTokens int  `gorm:"default:0" json:"completion_tokens,omitempty"`
	TotalTokens      int  `gorm:"default:0" json:"total_tokens,omitempty"`
	// Vote is the user's feedback on an assistant message: "", "up", or
	// "down". Downvotes surface in analytics for bad-case review. Indexed
	// with (tenant_id, role) because the feedback list filters on
	// role='assistant' AND vote<>'' across the whole tenant.
	Vote string `gorm:"size:8;not null;default:'';index:idx_msg_tenant_role_vote,priority:3" json:"vote,omitempty"`
	// FollowUps stores LLM-generated follow-up suggestions for this
	// assistant message as a JSON string array, rendered as clickable chips.
	FollowUps string    `gorm:"type:text" json:"follow_ups,omitempty"`
	CreatedAt time.Time `gorm:"index:idx_msg_tenant_conv_created,priority:3" json:"created_at"`
}

// Citation maps a retrieved chunk back to its source document. The chat
// service serializes a slice of these into Message.Citations as JSON.
type Citation struct {
	ChunkID     string  `json:"chunk_id"`
	DocID       string  `json:"doc_id"`
	DocName     string  `json:"doc_name"`
	Score       float64 `json:"score,omitempty"`        // retrieval score (RRF or rerank)
	Content     string  `json:"content,omitempty"`      // chunk text, for citation preview
	PageNumbers string  `json:"page_numbers,omitempty"` // source page numbers, if available
}
