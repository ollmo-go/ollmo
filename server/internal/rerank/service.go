package rerank

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/crypto"
	"ollmo/ollmo/pkg/errs"
)

// Reranker abstracts the rerank call so search can use either the DB-resolved
// provider or the static config-based fallback. tenantID is included so the
// resolver can look up per-tenant credentials and model. providerID, when
// non-empty, selects a specific provider (KB-level override); empty falls back
// to the tenant default.
type Reranker interface {
	Rerank(ctx context.Context, tenantID, providerID, query string, docs []clients.RerankInput, topN int) ([]clients.RerankOutput, error)
	Available(ctx context.Context, tenantID, providerID string) bool
}

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

type CreateInput struct {
	Name      string `json:"name"`
	Endpoint  string `json:"endpoint"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	TopN      int    `json:"top_n"`
	IsDefault bool   `json:"is_default"`
}

type UpdateInput struct {
	Name      *string `json:"name"`
	Endpoint  *string `json:"endpoint"`
	APIKey    *string `json:"api_key"`
	Model     *string `json:"model"`
	TopN      *int    `json:"top_n"`
	Status    *string `json:"status"`
	IsDefault *bool   `json:"is_default"`
}

func (s *Service) Create(ctx context.Context, tenantID, ownerID string, in CreateInput) (*RerankModel, error) {
	if in.Name == "" {
		return nil, errs.BadRequest("name is required")
	}
	if in.Model == "" {
		return nil, errs.BadRequest("model is required")
	}
	if in.Endpoint == "" {
		return nil, errs.BadRequest("endpoint is required")
	}
	if err := clients.ValidateEndpoint(in.Endpoint); err != nil {
		return nil, errs.BadRequest(err.Error())
	}
	p := &RerankModel{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Name:      in.Name,
		Endpoint:  in.Endpoint,
		APIKey:    in.APIKey,
		Model:     in.Model,
		TopN:      in.TopN,
		IsDefault: in.IsDefault,
		OwnerID:   ownerID,
		Status:    StatusActive,
	}
	if err := s.repo.Create(p); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create rerank provider", err)
	}
	if in.IsDefault {
		if err := s.repo.ClearDefault(tenantID, p.ID); err != nil {
			log.Printf("[rerank] clear default failed tenant=%s provider=%s: %v", tenantID, p.ID, err)
		}
	}
	p.APIKey = crypto.MaskSecret(p.APIKey)
	return p, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (*RerankModel, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if p != nil {
		p.APIKey = crypto.MaskSecret(p.APIKey)
	}
	return p, err
}

func (s *Service) List(ctx context.Context, tenantID string, page, size int) ([]*RerankModel, int64, error) {
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

func (s *Service) Update(ctx context.Context, tenantID, id string, in UpdateInput) (*RerankModel, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		p.Name = *in.Name
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
	if in.TopN != nil {
		p.TopN = *in.TopN
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
		return nil, errs.Wrap(errs.CodeInternal, "update rerank provider", err)
	}
	if in.IsDefault != nil && *in.IsDefault {
		if err := s.repo.ClearDefault(tenantID, p.ID); err != nil {
			log.Printf("[rerank] clear default failed tenant=%s provider=%s: %v", tenantID, p.ID, err)
		}
	}
	p.APIKey = crypto.MaskSecret(p.APIKey)
	return p, nil
}

func (s *Service) Delete(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(tenantID, id)
}

// Test sends a minimal rerank request to verify the provider's endpoint and
// credentials. The test outcome is persisted so the UI can show the last
// test status without re-running.
func (s *Service) Test(ctx context.Context, tenantID, id string) (string, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return "", err
	}
	if p.Status != StatusActive {
		return "", errs.BadRequest("provider is not active")
	}
	client := clients.NewRerank(p.Endpoint, p.APIKey)
	out, err := client.Rerank(ctx, p.Model, "test", []clients.RerankInput{
		{DocID: "1", Content: "test document"},
	}, 1)
	now := time.Now()
	if err != nil {
		_ = s.repo.UpdateTestResult(tenantID, id, "failed", now)
		return "", errs.Wrap(errs.CodeInternal, "rerank test", err)
	}
	_ = s.repo.UpdateTestResult(tenantID, id, "success", now)
	if len(out) > 0 {
		return "ok", nil
	}
	return "empty", nil
}

// Resolver implements Reranker by looking up the tenant's rerank provider in
// the DB. Falls back to the static client (from config.yaml) when no DB
// provider matches. When neither is available, Rerank returns nil and the
// search service skips the rerank leg.
type Resolver struct {
	repo          *Repo
	fallback      *clients.RerankClient
	fallbackModel string
	mu            sync.Mutex
	cache         map[string]*clients.RerankClient
}

func NewResolver(repo *Repo, fallback *clients.RerankClient, fallbackModel string) *Resolver {
	return &Resolver{repo: repo, fallback: fallback, fallbackModel: fallbackModel, cache: make(map[string]*clients.RerankClient)}
}

// Available returns true if rerank will work for this tenant: either the
// config fallback is set, or the tenant has at least one active DB provider.
// When providerID is non-empty, checks that specific provider instead of the
// tenant default.
func (r *Resolver) Available(ctx context.Context, tenantID, providerID string) bool {
	if r.fallback != nil && r.fallbackModel != "" && providerID == "" {
		return true
	}
	if r.repo != nil {
		p, err := r.resolve(tenantID, providerID)
		return err == nil && p != nil
	}
	return false
}

func (r *Resolver) Rerank(ctx context.Context, tenantID, providerID, query string, docs []clients.RerankInput, topN int) ([]clients.RerankOutput, error) {
	p, err := r.resolve(tenantID, providerID)
	if err != nil || p == nil {
		return nil, nil
	}
	var client *clients.RerankClient
	if p.Endpoint != "" {
		client = r.getOrCreate(p.Endpoint, p.APIKey)
	} else {
		client = r.fallback
	}
	if client == nil {
		return nil, nil
	}
	if topN <= 0 {
		topN = p.TopN
	}
	if topN <= 0 {
		topN = 10
	}
	return client.Rerank(ctx, p.Model, query, docs, topN)
}

// resolve finds the rerank provider: KB-level providerID first, then tenant
// default, then static config fallback. Returns nil when none is available.
func (r *Resolver) resolve(tenantID, providerID string) (*RerankModel, error) {
	if r.repo != nil {
		if providerID != "" {
			p, err := r.repo.FindByID(tenantID, providerID)
			if err == nil && p.Status == StatusActive {
				return p, nil
			}
			// KB-specified provider missing or inactive; fall through to default.
		}
		p, err := r.repo.FindDefault(tenantID)
		if err == nil {
			return p, nil
		}
	}
	if r.fallback != nil && r.fallbackModel != "" && providerID == "" {
		return &RerankModel{Model: r.fallbackModel}, nil
	}
	return nil, nil
}

func (r *Resolver) getOrCreate(endpoint, apiKey string) *clients.RerankClient {
	key := endpoint + "|" + apiKey
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.cache[key]; ok {
		return c
	}
	c := clients.NewRerank(endpoint, apiKey)
	r.cache[key] = c
	return c
}
