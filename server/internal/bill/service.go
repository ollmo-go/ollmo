package bill

import (
	"context"

	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/pkg/clients"
)

// Overview aggregates a tenant's token usage and estimated cost.
type Overview struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Amount           float64 `json:"amount"`
	CallCount        int64   `json:"call_count"`
}

// UserTotal aggregates one user's token usage and estimated cost.
type UserTotal struct {
	UserID           string  `json:"user_id"`
	UserName         string  `json:"user_name"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Amount           float64 `json:"amount"`
	CallCount        int64   `json:"call_count"`
}

// ModelTotal aggregates one model's token usage and estimated cost.
type ModelTotal struct {
	ModelID          string  `json:"model_id"`
	ModelName        string  `json:"model_name"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Amount           float64 `json:"amount"`
	CallCount        int64   `json:"call_count"`
}

// Service owns bill persistence and aggregation. The chat service calls
// Record on every LLM invocation; analytics handlers read the aggregates.
type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

func (s *Service) Record(rec *Record) error {
	if s == nil || s.repo == nil || rec == nil {
		return nil
	}
	return s.repo.Create(rec)
}

// RecordUsage is a convenience for the chat service: it charges a model call
// whose token usage came back from the provider (usage may be nil for
// providers that omit it — the row is still written with 0 tokens).
func (s *Service) RecordUsage(tenantID, userID, kbID, convID, source string, model *llm.LLMModel, usage *clients.TokenUsage) {
	if s == nil || s.repo == nil {
		return
	}
	if model == nil {
		return
	}
	rec := &Record{
		ID:             newID(),
		TenantID:       tenantID,
		UserID:         userID,
		KbID:           kbID,
		ConversationID: convID,
		Source:         source,
		Provider:       model.Provider,
		ModelID:        model.ID,
		ModelName:      model.Model,
	}
	if usage != nil {
		rec.PromptTokens = usage.PromptTokens
		rec.CompletionTokens = usage.CompletionTokens
		rec.TotalTokens = usage.TotalTokens
	}
	rec.Amount = estimateAmount(model, rec.PromptTokens, rec.CompletionTokens)
	if err := s.repo.Create(rec); err != nil {
		// Billing failures are logged and swallowed: a flaky write must
		// never break a chat stream or a node walk.
		logWarn("bill record failed tenant=%s user=%s model=%s: %v", tenantID, userID, model.Model, err)
	}
}

func (s *Service) List(ctx context.Context, tenantID string, limit int) ([]*Record, error) {
	return s.repo.List(tenantID, limit)
}

func (s *Service) UserTotals(ctx context.Context, tenantID string) ([]UserTotal, error) {
	return s.repo.UserTotals(tenantID)
}

func (s *Service) ModelTotals(ctx context.Context, tenantID string) ([]ModelTotal, error) {
	return s.repo.ModelTotals(tenantID)
}

func (s *Service) Overview(ctx context.Context, tenantID string) (*Overview, error) {
	return s.repo.Overview(tenantID)
}

// MineOverview returns the calling user's own usage totals.
func (s *Service) MineOverview(ctx context.Context, tenantID, userID string) (*Overview, error) {
	return s.repo.OverviewByUser(tenantID, userID)
}

// MineRecords returns the calling user's recent bill rows.
func (s *Service) MineRecords(ctx context.Context, tenantID, userID string, limit int) ([]*Record, error) {
	return s.repo.ListByUser(tenantID, userID, limit)
}
