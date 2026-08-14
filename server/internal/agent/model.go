package agent

import "time"

// Agent is the per-KB Q&A flow definition stored as a JSON DAG. The frontend
// renders it on a React Flow canvas; the chat service walks the graph to
// customize retrieval and generation.
//
// One row per KB (unique on kb_id). Saving writes a new version; only the
// latest active row is read at execution time.
type Agent struct {
	ID         string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID   string    `gorm:"size:36;index;not null" json:"tenant_id"`
	KbID       string    `gorm:"size:36;uniqueIndex;not null" json:"kb_id"`
	Name       string    `gorm:"size:128;not null;default:'Default Agent'" json:"name"`
	Version    int       `gorm:"not null;default:1" json:"version"`
	Definition string    `gorm:"type:text" json:"definition"`
	Active     bool      `gorm:"not null;default:true" json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Agent) TableName() string { return "agents" }

// Definition is the typed view of the agent JSON. It mirrors the React Flow
// graph shape so the frontend round-trips the canvas state without translation.
// OpeningMessage is definition-level (not a node) so the start node is implicit.
type Definition struct {
	Nodes              []Node   `json:"nodes"`
	Edges              []Edge   `json:"edges"`
	OpeningMessage     string   `json:"opening_message,omitempty"`
	SuggestedQuestions []string `json:"suggested_questions,omitempty"`
}

type Node struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Position Position               `json:"position"`
	Data     map[string]interface{} `json:"data"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
}

// Node type identifiers for the agent canvas. "start"/"end" are implicit
// (not rendered on canvas) — the chat engine treats graph entry as start
// and terminal nodes (llm, message) as end points.
const (
	NodeInput      = "start" // implicit
	NodeRetrieval  = "retrieval"
	NodeLLM        = "llm"
	NodeOutput     = "end"        // implicit
	NodeMessage    = "message"    // direct reply without LLM
	NodeCondition  = "condition"  // branch on variable comparison
	NodeClassifier = "classifier" // LLM-based question routing
)

// ExecutionConfig is the flat config derived from the Definition. The chat
// service uses it to override its default retrieval and generation behavior.
type ExecutionConfig struct {
	UseRetrieval    bool
	TopK            int
	Rerank          bool
	UseGraph        bool
	SystemPrompt    string
	Temperature     float64
	MaxTokens       int
	TopP            float64
	OpeningMessage  string
	LLMModelID      string
	RerankModelID   string // empty falls back to the tenant default rerank model
	ReasoningEffort string // "" / "high" / "max" — enables thinking mode on supported models
}

// DefaultExecutionConfig returns sensible defaults that match the hardcoded
// chat flow. The agent definition overrides only the fields it customizes.
func DefaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		UseRetrieval: true,
		TopK:         10,
		Rerank:       true,
		UseGraph:     true,
		SystemPrompt: "",
		Temperature:  0.7,
		MaxTokens:    2048,
		TopP:         0.9,
	}
}
