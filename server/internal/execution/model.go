package execution

import "time"

// Execution statuses.
const (
	StatusSuccess   = "success"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

// Execution sources. Chat runs come from real conversations; test runs from
// the agent config test drawer.
const (
	SourceChat = "chat"
	SourceTest = "test"
)

// Execution persists one agent-graph run (node-by-node trace plus the final
// outcome) so it can be replayed later on the canvas. Recorded for both real
// chats and test-drawer runs; the source field distinguishes them.
type Execution struct {
	ID             string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID       string    `gorm:"size:36;index:idx_exec_tenant_kb_created,priority:1;not null" json:"tenant_id"`
	KbID           string    `gorm:"size:36;index:idx_exec_tenant_kb_created,priority:2;not null" json:"kb_id"`
	ConversationID string    `gorm:"size:36;index" json:"conversation_id,omitempty"`
	MessageID      string    `gorm:"size:36" json:"message_id,omitempty"`
	UserID         string    `gorm:"size:36" json:"user_id,omitempty"`
	UserName       string    `gorm:"-" json:"user_name,omitempty"`
	Source         string    `gorm:"size:16;not null;default:'chat'" json:"source"`
	Status         string    `gorm:"size:16;not null;default:'success'" json:"status"`
	Query          string    `gorm:"type:text" json:"query"`
	Answer         string    `gorm:"type:text" json:"answer,omitempty"`
	TerminalType   string    `gorm:"size:32" json:"terminal_type,omitempty"`
	Trace          string    `gorm:"type:longtext" json:"trace"`
	TotalMs        int       `gorm:"default:0" json:"total_ms"`
	CreatedAt      time.Time `gorm:"index:idx_exec_tenant_kb_created,priority:3" json:"created_at"`
}

func (Execution) TableName() string { return "agent_executions" }
