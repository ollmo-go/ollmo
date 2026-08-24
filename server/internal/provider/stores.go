package provider

import (
	"context"
	"fmt"
	"strings"

	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/internal/rerank"
	"ollmo/ollmo/pkg/clients"
)

// ModelRef is the provider-agnostic view of one model row bound to a card.
// MaxTokens/prices are chat-specific; embedding/rerank rows report them 0.
type ModelRef struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Model          string  `json:"model"`
	ContextLength  int     `json:"context_length,omitempty"`
	MaxTokens      int     `json:"max_tokens,omitempty"`
	InputPrice     float64 `json:"input_price,omitempty"`
	OutputPrice    float64 `json:"output_price,omitempty"`
	IsDefault      bool    `json:"is_default"`
	LastTestStatus string  `json:"last_test_status"`
}

// KindStore abstracts the per-kind model table so one service serves
// chat/embedding/rerank. Implementations copy the provider's endpoint/key
// onto the row so the call path stays join-free.
type KindStore interface {
	CreateModel(tenantID, ownerID, providerID string, pv *Provider, in ModelInput) (ModelRef, error)
	ListModels(tenantID, providerID string) ([]ModelRef, error)
	DeleteModel(tenantID, modelID string) error
	UpdateModel(tenantID string, m ModelRef) error
	UpdateCreds(tenantID, providerID, endpoint, apiKey string) error
	DeleteAll(tenantID, providerID string) error
}

// LLMStore binds chat model rows.
type LLMStore struct{ repo *llm.Repo }

func NewLLMStore(repo *llm.Repo) *LLMStore { return &LLMStore{repo: repo} }

func (s *LLMStore) CreateModel(tenantID, ownerID, providerID string, pv *Provider, in ModelInput) (ModelRef, error) {
	// The tenant's first chat model becomes the default so chat works
	// immediately after setup; later models stay non-default.
	_, err := s.repo.FindDefault(tenantID)
	isDefault := err != nil
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = in.Model
	}
	maxTokens := in.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	p := &llm.LLMModel{
		ID:            NewID(),
		TenantID:      tenantID,
		ProviderID:    providerID,
		Name:          name,
		Provider:      pv.CatalogID,
		Endpoint:      pv.Endpoint,
		APIKey:        pv.APIKey,
		Model:         in.Model,
		MaxTokens:     maxTokens,
		ContextLength: in.ContextLength,
		InputPrice:    in.InputPrice,
		OutputPrice:   in.OutputPrice,
		OwnerID:       ownerID,
		Status:        llm.StatusActive,
		IsDefault:     isDefault,
	}
	if err := s.repo.Create(p); err != nil {
		return ModelRef{}, err
	}
	return ModelRef{ID: p.ID, Name: p.Name, Model: p.Model, ContextLength: p.ContextLength, MaxTokens: p.MaxTokens, InputPrice: p.InputPrice, OutputPrice: p.OutputPrice, IsDefault: isDefault}, nil
}

func (s *LLMStore) ListModels(tenantID, providerID string) ([]ModelRef, error) {
	items, err := s.repo.ListByProvider(tenantID, providerID)
	if err != nil {
		return nil, err
	}
	refs := make([]ModelRef, 0, len(items))
	for _, p := range items {
		refs = append(refs, ModelRef{ID: p.ID, Name: p.Name, Model: p.Model, ContextLength: p.ContextLength, MaxTokens: p.MaxTokens, InputPrice: p.InputPrice, OutputPrice: p.OutputPrice, IsDefault: p.IsDefault, LastTestStatus: p.LastTestStatus})
	}
	return refs, nil
}

func (s *LLMStore) DeleteModel(tenantID, modelID string) error {
	return s.repo.Delete(tenantID, modelID)
}
func (s *LLMStore) UpdateModel(tenantID string, m ModelRef) error {
	p, err := s.repo.FindByID(tenantID, m.ID)
	if err != nil {
		return err
	}
	p.Name = m.Name
	p.ContextLength = m.ContextLength
	if m.MaxTokens > 0 {
		p.MaxTokens = m.MaxTokens
	}
	p.InputPrice = m.InputPrice
	p.OutputPrice = m.OutputPrice
	return s.repo.Update(p)
}
func (s *LLMStore) UpdateCreds(tenantID, providerID, endpoint, apiKey string) error {
	return s.repo.UpdateProviderCreds(tenantID, providerID, endpoint, apiKey)
}
func (s *LLMStore) DeleteAll(tenantID, providerID string) error {
	return s.repo.DeleteByProvider(tenantID, providerID)
}

// EmbeddingStore binds embedding model rows. Dim is derived from the model
// name, matching the embedding service's own create path.
type EmbeddingStore struct{ repo *embedding.Repo }

func NewEmbeddingStore(repo *embedding.Repo) *EmbeddingStore { return &EmbeddingStore{repo: repo} }

func (s *EmbeddingStore) CreateModel(tenantID, ownerID, providerID string, pv *Provider, in ModelInput) (ModelRef, error) {
	cli := clients.NewEmbedding(pv.Endpoint, pv.APIKey)
	dim, err := cli.DetectDim(context.Background(), in.Model)
	if err != nil {
		return ModelRef{}, fmt.Errorf("cannot detect embedding dimension for model %q: %w", in.Model, err)
	}
	_, fdErr := s.repo.FindDefault(tenantID)
	isDefault := fdErr != nil
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = in.Model
	}
	p := &embedding.EmbeddingModel{
		ID:         NewID(),
		TenantID:   tenantID,
		ProviderID: providerID,
		Name:       name,
		Endpoint:   pv.Endpoint,
		APIKey:     pv.APIKey,
		Model:      in.Model,
		Dim:        dim,
		OwnerID:    ownerID,
		Status:     embedding.StatusActive,
		IsDefault:  isDefault,
	}
	if err := s.repo.Create(p); err != nil {
		return ModelRef{}, err
	}
	return ModelRef{ID: p.ID, Name: p.Name, Model: p.Model, IsDefault: isDefault}, nil
}

func (s *EmbeddingStore) ListModels(tenantID, providerID string) ([]ModelRef, error) {
	items, err := s.repo.ListByProvider(tenantID, providerID)
	if err != nil {
		return nil, err
	}
	refs := make([]ModelRef, 0, len(items))
	for _, p := range items {
		refs = append(refs, ModelRef{ID: p.ID, Name: p.Name, Model: p.Model, IsDefault: p.IsDefault, LastTestStatus: p.LastTestStatus})
	}
	return refs, nil
}

func (s *EmbeddingStore) DeleteModel(tenantID, modelID string) error {
	return s.repo.Delete(tenantID, modelID)
}
func (s *EmbeddingStore) UpdateModel(tenantID string, m ModelRef) error {
	p, err := s.repo.FindByID(tenantID, m.ID)
	if err != nil {
		return err
	}
	p.Name = m.Name
	return s.repo.Update(p)
}
func (s *EmbeddingStore) UpdateCreds(tenantID, providerID, endpoint, apiKey string) error {
	return s.repo.UpdateProviderCreds(tenantID, providerID, endpoint, apiKey)
}
func (s *EmbeddingStore) DeleteAll(tenantID, providerID string) error {
	return s.repo.DeleteByProvider(tenantID, providerID)
}

// RerankStore binds rerank model rows.
type RerankStore struct{ repo *rerank.Repo }

func NewRerankStore(repo *rerank.Repo) *RerankStore { return &RerankStore{repo: repo} }

func (s *RerankStore) CreateModel(tenantID, ownerID, providerID string, pv *Provider, in ModelInput) (ModelRef, error) {
	_, err := s.repo.FindDefault(tenantID)
	isDefault := err != nil
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = in.Model
	}
	p := &rerank.RerankModel{
		ID:         NewID(),
		TenantID:   tenantID,
		ProviderID: providerID,
		Name:       name,
		Endpoint:   pv.Endpoint,
		APIKey:     pv.APIKey,
		Model:      in.Model,
		OwnerID:    ownerID,
		Status:     rerank.StatusActive,
		IsDefault:  isDefault,
	}
	if err := s.repo.Create(p); err != nil {
		return ModelRef{}, err
	}
	return ModelRef{ID: p.ID, Name: p.Name, Model: p.Model, IsDefault: isDefault}, nil
}

func (s *RerankStore) ListModels(tenantID, providerID string) ([]ModelRef, error) {
	items, err := s.repo.ListByProvider(tenantID, providerID)
	if err != nil {
		return nil, err
	}
	refs := make([]ModelRef, 0, len(items))
	for _, p := range items {
		refs = append(refs, ModelRef{ID: p.ID, Name: p.Name, Model: p.Model, IsDefault: p.IsDefault, LastTestStatus: p.LastTestStatus})
	}
	return refs, nil
}

func (s *RerankStore) DeleteModel(tenantID, modelID string) error {
	return s.repo.Delete(tenantID, modelID)
}
func (s *RerankStore) UpdateModel(tenantID string, m ModelRef) error {
	p, err := s.repo.FindByID(tenantID, m.ID)
	if err != nil {
		return err
	}
	p.Name = m.Name
	return s.repo.Update(p)
}
func (s *RerankStore) UpdateCreds(tenantID, providerID, endpoint, apiKey string) error {
	return s.repo.UpdateProviderCreds(tenantID, providerID, endpoint, apiKey)
}
func (s *RerankStore) DeleteAll(tenantID, providerID string) error {
	return s.repo.DeleteByProvider(tenantID, providerID)
}
