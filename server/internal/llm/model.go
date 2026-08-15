package llm

import "time"

// Provider identifiers. Endpoint defaults are resolved in the service layer.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderDeepSeek  = "deepseek"
	ProviderZhipu     = "zhipu"
	ProviderOllama    = "ollama"
	ProviderCustom    = "custom"
)

// Provider lifecycle statuses.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// LLMModel is a tenant-scoped chat model configuration. Endpoint and APIKey
// are OpenAI-compatible; non-OpenAI providers use their compatible endpoints.
// ProviderID groups rows under a settings provider card; endpoint/api_key
// stay duplicated on the row so the call path needs no join.
type LLMModel struct {
	ID          string  `gorm:"primaryKey;size:36" json:"id"`
	TenantID    string  `gorm:"size:36;index;not null" json:"tenant_id"`
	ProviderID  string  `gorm:"size:36;index" json:"provider_id,omitempty"`
	Name        string  `gorm:"size:128;not null" json:"name"`
	Provider    string  `gorm:"size:32;not null;default:openai" json:"provider"`
	Endpoint    string  `gorm:"size:512;not null" json:"endpoint"`
	APIKey      string  `gorm:"size:512" json:"api_key,omitempty"`
	Model       string  `gorm:"size:128;not null" json:"model"`
	Temperature float64 `gorm:"not null;default:0.7" json:"temperature"`
	MaxTokens   int     `gorm:"not null;default:2048" json:"max_tokens"`
	// ContextLength is the model's input+output window in tokens. 0 means
	// unknown; the chat service then assumes DefaultContextLength.
	ContextLength int     `gorm:"not null;default:0" json:"context_length"`
	TopP          float64 `gorm:"not null;default:1" json:"top_p"`
	// InputPrice/OutputPrice are the model's cost in yuan per 1M tokens.
	// 0 means unconfigured; the bill module then charges 0 for this model.
	InputPrice     float64    `gorm:"not null;default:0" json:"input_price"`
	OutputPrice    float64    `gorm:"not null;default:0" json:"output_price"`
	IsDefault      bool       `gorm:"not null;default:false" json:"is_default"`
	OwnerID        string     `gorm:"size:36;index;not null" json:"owner_id"`
	Status         string     `gorm:"size:32;not null;default:active" json:"status"`
	LastTestedAt   *time.Time `json:"last_tested_at"`
	LastTestStatus string     `gorm:"size:16;default:''" json:"last_test_status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName maps LLMModel to the llm_models table.
func (LLMModel) TableName() string { return "llm_models" }
