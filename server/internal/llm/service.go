package llm

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/crypto"
	"ollmo/ollmo/pkg/errs"
)

type Service struct {
	repo *Repo
	llm  *clients.LLMClient
}

func NewService(repo *Repo, llm *clients.LLMClient) *Service {
	return &Service{repo: repo, llm: llm}
}

type CreateInput struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	Endpoint    string  `json:"endpoint"`
	APIKey      string  `json:"api_key"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	ContextLen  int     `json:"context_length"`
	TopP        float64 `json:"top_p"`
	IsDefault   bool    `json:"is_default"`
}

type UpdateInput struct {
	Name        *string  `json:"name"`
	Provider    *string  `json:"provider"`
	Endpoint    *string  `json:"endpoint"`
	APIKey      *string  `json:"api_key"`
	Model       *string  `json:"model"`
	Temperature *float64 `json:"temperature"`
	MaxTokens   *int     `json:"max_tokens"`
	ContextLen  *int     `json:"context_length"`
	TopP        *float64 `json:"top_p"`
	Status      *string  `json:"status"`
	IsDefault   *bool    `json:"is_default"`
}

var allowedProviders = map[string]bool{
	ProviderOpenAI: true, ProviderAnthropic: true, ProviderDeepSeek: true,
	ProviderZhipu: true, ProviderOllama: true, ProviderCustom: true,
}

func (s *Service) Create(ctx context.Context, tenantID, ownerID string, in CreateInput) (*LLMModel, error) {
	if in.Name == "" {
		return nil, errs.BadRequest("name is required")
	}
	if in.Model == "" {
		return nil, errs.BadRequest("model is required")
	}
	if in.Provider == "" {
		in.Provider = ProviderOpenAI
	}
	if !allowedProviders[in.Provider] {
		return nil, errs.BadRequest("unknown provider: " + in.Provider)
	}
	if in.Endpoint == "" {
		in.Endpoint = defaultEndpoint(in.Provider)
	}
	if err := clients.ValidateEndpoint(in.Endpoint); err != nil {
		return nil, errs.BadRequest(err.Error())
	}
	if in.Temperature == 0 {
		in.Temperature = 0.7
	}
	if in.MaxTokens == 0 {
		in.MaxTokens = 2048
	}
	if in.TopP == 0 {
		in.TopP = 1
	}

	p := &LLMModel{
		ID:            uuid.NewString(),
		TenantID:      tenantID,
		Name:          in.Name,
		Provider:      in.Provider,
		Endpoint:      in.Endpoint,
		APIKey:        in.APIKey,
		Model:         in.Model,
		Temperature:   in.Temperature,
		MaxTokens:     in.MaxTokens,
		ContextLength: in.ContextLen,
		TopP:          in.TopP,
		IsDefault:     in.IsDefault,
		OwnerID:       ownerID,
		Status:        StatusActive,
	}
	if err := s.repo.Create(p); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create llm provider", err)
	}
	if in.IsDefault {
		if err := s.repo.ClearDefault(tenantID, p.ID); err != nil {
			// Non-fatal: there may be multiple defaults, but Create succeeded.
			log.Printf("[llm] clear default failed tenant=%s provider=%s: %v", tenantID, p.ID, err)
		}
	}
	p.APIKey = crypto.MaskSecret(p.APIKey)
	return p, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (*LLMModel, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	p.APIKey = crypto.MaskSecret(p.APIKey)
	return p, nil
}

// GetDefault returns the tenant's default provider. The chat domain calls this
// when the client does not specify one.
func (s *Service) GetDefault(ctx context.Context, tenantID string) (*LLMModel, error) {
	return s.repo.FindDefault(tenantID)
}

func (s *Service) List(ctx context.Context, tenantID string, page, size int) ([]*LLMModel, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	items, total, err := s.repo.List(tenantID, page, size)
	if err != nil {
		return nil, 0, err
	}
	for _, p := range items {
		p.APIKey = crypto.MaskSecret(p.APIKey)
	}
	return items, total, nil
}

func (s *Service) Update(ctx context.Context, tenantID, id string, in UpdateInput) (*LLMModel, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.Provider != nil {
		if !allowedProviders[*in.Provider] {
			return nil, errs.BadRequest("unknown provider: " + *in.Provider)
		}
		p.Provider = *in.Provider
	}
	if in.Endpoint != nil {
		if err := clients.ValidateEndpoint(*in.Endpoint); err != nil {
			return nil, errs.BadRequest(err.Error())
		}
		p.Endpoint = *in.Endpoint
	}
	// Empty or the masked form of the current key means "unchanged", so a
	// client echoing back the masked value never corrupts the stored secret.
	if in.APIKey != nil && *in.APIKey != "" && *in.APIKey != crypto.MaskSecret(p.APIKey) {
		p.APIKey = *in.APIKey
	}
	if in.Model != nil {
		p.Model = *in.Model
	}
	if in.Temperature != nil {
		p.Temperature = *in.Temperature
	}
	if in.MaxTokens != nil {
		p.MaxTokens = *in.MaxTokens
	}
	if in.ContextLen != nil {
		p.ContextLength = *in.ContextLen
	}
	if in.TopP != nil {
		p.TopP = *in.TopP
	}
	if in.Status != nil {
		if *in.Status != StatusActive && *in.Status != StatusDisabled {
			return nil, errs.BadRequest("invalid status: " + *in.Status)
		}
		p.Status = *in.Status
	}
	if in.IsDefault != nil {
		p.IsDefault = *in.IsDefault
	}
	if err := s.repo.Update(p); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "update llm provider", err)
	}
	if in.IsDefault != nil && *in.IsDefault {
		if err := s.repo.ClearDefault(tenantID, p.ID); err != nil {
			log.Printf("[llm] clear default failed tenant=%s provider=%s: %v", tenantID, p.ID, err)
		}
	}
	p.APIKey = crypto.MaskSecret(p.APIKey)
	return p, nil
}

func (s *Service) Delete(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(tenantID, id)
}

// Test sends a minimal completion request to verify the provider's endpoint
// and credentials. Returns the assistant's reply so the UI can show it.
// The test outcome (success/failed + timestamp) is persisted to the provider
// row so the UI can display the last test status without re-running.
func (s *Service) Test(ctx context.Context, tenantID, id string) (string, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return "", err
	}
	if p.Status != StatusActive {
		return "", errs.BadRequest("provider is not active")
	}
	out, err := s.llm.Chat(ctx, p.Endpoint, p.APIKey, clients.ChatRequest{
		Model: p.Model,
		Messages: []clients.ChatMessage{
			{Role: "user", Content: "Reply with the single word: ok"},
		},
		Temperature: 0,
		MaxTokens:   512,
	})
	now := time.Now()
	if err != nil {
		_ = s.repo.UpdateTestResult(tenantID, id, "failed", now)
		return "", errs.Wrap(errs.CodeInternal, "llm test", err)
	}
	_ = s.repo.UpdateTestResult(tenantID, id, "success", now)
	return out, nil
}

func defaultEndpoint(provider string) string {
	switch strings.ToLower(provider) {
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	case ProviderAnthropic:
		// Anthropic's OpenAI-compatible endpoint; native Messages API is Phase 2.
		return "https://api.anthropic.com/v1"
	case ProviderDeepSeek:
		return "https://api.deepseek.com/v1"
	case ProviderZhipu:
		return "https://open.bigmodel.cn/api/paas/v4"
	case ProviderOllama:
		return "http://localhost:11434/v1"
	default:
		return ""
	}
}
