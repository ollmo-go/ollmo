package bill

import "time"

// Record sources identify which pipeline step produced the LLM call.
const (
	SourceChat         = "chat"         // main streaming reply (flat or graph terminal)
	SourceClassifier   = "classifier"   // intent-classification completion
	SourceIntermediate = "intermediate" // non-terminal llm node (rewrite/summarize)
	SourceFollowUps    = "followups"    // follow-up question generation
)

// Record is one LLM invocation charge. Rows are append-only: each call to
// the model writes a row, so aggregations can answer "how many tokens /
// how much money per user / per model / per tenant" by summing.
type Record struct {
	ID               string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID         string    `gorm:"size:36;index:idx_bill_tenant_created,priority:1;not null" json:"tenant_id"`
	UserID           string    `gorm:"size:36;index;not null" json:"user_id"`
	KbID             string    `gorm:"size:36;index" json:"kb_id,omitempty"`
	ConversationID   string    `gorm:"size:36;index" json:"conversation_id,omitempty"`
	Source           string    `gorm:"size:16;not null" json:"source"`
	Provider         string    `gorm:"size:32;not null;default:openai" json:"provider"`
	ModelID          string    `gorm:"size:36;index" json:"model_id,omitempty"`
	ModelName        string    `gorm:"size:128;not null" json:"model_name"`
	PromptTokens     int       `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens int       `gorm:"not null;default:0" json:"completion_tokens"`
	TotalTokens      int       `gorm:"not null;default:0" json:"total_tokens"`
	// Amount is the estimated cost in yuan (per-1M-token price × tokens).
	// Prices default to 0, so rows with no configured price cost nothing.
	Amount    float64   `gorm:"not null;default:0" json:"amount"`
	CreatedAt time.Time `gorm:"index:idx_bill_tenant_created,priority:2" json:"created_at"`
}

// TableName maps Record to the bills table.
func (Record) TableName() string { return "bills" }
