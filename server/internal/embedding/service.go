package embedding

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/crypto"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/vector"
)

// Embedder abstracts the embedding call so doc/search can use either the
// DB-resolved provider or the static config-based fallback. tenantID is
// included so the resolver can look up per-tenant credentials.
type Embedder interface {
	Embed(ctx context.Context, tenantID, model string, inputs []string) ([][]float32, error)
	ResolveBatchSize(ctx context.Context, tenantID, model string) int
	// ResolveModel translates a KB's pinned embedding_model_id into the model
	// name, vector dimension, and the provider's batch size.
	// Used by doc/search to bridge the id reference to the runtime model name.
	ResolveModel(ctx context.Context, tenantID, id string) (model string, dim, batchSize int, err error)
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
	BatchSize int    `json:"batch_size"`
	IsDefault bool   `json:"is_default"`
}

type UpdateInput struct {
	Name      *string `json:"name"`
	Endpoint  *string `json:"endpoint"`
	APIKey    *string `json:"api_key"`
	Model     *string `json:"model"`
	BatchSize *int    `json:"batch_size"`
	Status    *string `json:"status"`
	IsDefault *bool   `json:"is_default"`
}

func (s *Service) Create(ctx context.Context, tenantID, ownerID string, in CreateInput) (*EmbeddingModel, error) {
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
	dim := vector.EmbeddingDim(in.Model)
	if dim == 0 {
		// Unknown model — probe the endpoint to detect the real dimension.
		cli := clients.NewEmbedding(in.Endpoint, in.APIKey)
		detected, err := cli.DetectDim(ctx, in.Model)
		if err != nil {
			return nil, errs.BadRequest(fmt.Sprintf("cannot detect embedding dimension for model %q: %v", in.Model, err))
		}
		dim = detected
	}
	p := &EmbeddingModel{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Name:      in.Name,
		Endpoint:  in.Endpoint,
		APIKey:    in.APIKey,
		Model:     in.Model,
		Dim:       dim,
		BatchSize: in.BatchSize,
		IsDefault: in.IsDefault,
		OwnerID:   ownerID,
		Status:    StatusActive,
	}
	if err := s.repo.Create(p); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create embedding provider", err)
	}
	if in.IsDefault {
		if err := s.repo.ClearDefault(tenantID, p.ID); err != nil {
			log.Printf("[embedding] clear default failed tenant=%s provider=%s: %v", tenantID, p.ID, err)
		}
	}
	p.APIKey = crypto.MaskSecret(p.APIKey)
	return p, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (*EmbeddingModel, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if p != nil {
		p.APIKey = crypto.MaskSecret(p.APIKey)
	}
	return p, err
}

func (s *Service) List(ctx context.Context, tenantID string, page, size int) ([]*EmbeddingModel, int64, error) {
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

func (s *Service) Update(ctx context.Context, tenantID, id string, in UpdateInput) (*EmbeddingModel, error) {
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
		dim := vector.EmbeddingDim(*in.Model)
		if dim == 0 {
			cli := clients.NewEmbedding(p.Endpoint, p.APIKey)
			detected, err := cli.DetectDim(ctx, *in.Model)
			if err != nil {
				return nil, errs.BadRequest(fmt.Sprintf("cannot detect embedding dimension for model %q: %v", *in.Model, err))
			}
			dim = detected
		}
		p.Model = *in.Model
		p.Dim = dim
	}
	if in.BatchSize != nil {
		p.BatchSize = *in.BatchSize
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
		return nil, errs.Wrap(errs.CodeInternal, "update embedding provider", err)
	}
	if in.IsDefault != nil && *in.IsDefault {
		if err := s.repo.ClearDefault(tenantID, p.ID); err != nil {
			log.Printf("[embedding] clear default failed tenant=%s provider=%s: %v", tenantID, p.ID, err)
		}
	}
	p.APIKey = crypto.MaskSecret(p.APIKey)
	return p, nil
}

func (s *Service) Delete(ctx context.Context, tenantID, id string) error {
	return s.repo.Delete(tenantID, id)
}

// Test sends a minimal embedding request to verify the provider's endpoint
// and credentials. The test outcome is persisted so the UI can show the
// last test status without re-running.
func (s *Service) Test(ctx context.Context, tenantID, id string) (string, error) {
	p, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return "", err
	}
	if p.Status != StatusActive {
		return "", errs.BadRequest("provider is not active")
	}
	client := clients.NewEmbedding(p.Endpoint, p.APIKey)
	vecs, err := client.Embed(ctx, p.Model, []string{"test"})
	now := time.Now()
	if err != nil {
		_ = s.repo.UpdateTestResult(tenantID, id, "failed", now)
		return "", errs.Wrap(errs.CodeInternal, "embedding test", err)
	}
	_ = s.repo.UpdateTestResult(tenantID, id, "success", now)
	if len(vecs) > 0 {
		return "ok", nil
	}
	return "empty", nil
}

// Resolver implements Embedder by looking up the tenant's embedding provider
// in the DB. When model is empty the tenant's default provider (and its model)
// is used. Returns an error when no provider is configured.
type Resolver struct {
	repo     *Repo
	fallback *clients.EmbeddingClient
	mu       sync.Mutex
	cache    map[string]*clients.EmbeddingClient
}

func NewResolver(repo *Repo, fallback *clients.EmbeddingClient) *Resolver {
	return &Resolver{repo: repo, fallback: fallback, cache: make(map[string]*clients.EmbeddingClient)}
}

func (r *Resolver) Embed(ctx context.Context, tenantID, model string, inputs []string) ([][]float32, error) {
	client, resolvedModel, _ := r.resolve(ctx, tenantID, model)
	if client == nil {
		return nil, errs.Internal("no embedding provider configured for tenant")
	}
	if model == "" {
		model = resolvedModel
	}
	return client.Embed(ctx, model, inputs)
}

// ResolveBatchSize returns the batch size from the resolved provider. Falls
// back to 32 when no provider is found.
func (r *Resolver) ResolveBatchSize(ctx context.Context, tenantID, model string) int {
	_, _, bs := r.resolve(ctx, tenantID, model)
	if bs > 0 {
		return bs
	}
	return 32
}

// ResolveModel looks up a KB's pinned embedding model by id and returns the
// model name, vector dimension, and the provider's batch size.
func (r *Resolver) ResolveModel(ctx context.Context, tenantID, id string) (string, int, int, error) {
	if r.repo == nil {
		return "", 0, 0, errs.Internal("embedding repo not configured")
	}
	p, err := r.repo.FindByID(tenantID, id)
	if err != nil {
		return "", 0, 0, err
	}
	return p.Model, p.Dim, p.BatchSize, nil
}

func (r *Resolver) resolve(ctx context.Context, tenantID, model string) (*clients.EmbeddingClient, string, int) {
	if r.repo != nil {
		if model != "" {
			p, err := r.repo.FindByModel(tenantID, model)
			if err == nil {
				return r.getOrCreate(p.Endpoint, p.APIKey), p.Model, p.BatchSize
			}
		}
		p, err := r.repo.FindDefault(tenantID)
		if err == nil {
			return r.getOrCreate(p.Endpoint, p.APIKey), p.Model, p.BatchSize
		}
	}
	if r.fallback != nil {
		return r.fallback, model, 0
	}
	return nil, "", 0
}

func (r *Resolver) getOrCreate(endpoint, apiKey string) *clients.EmbeddingClient {
	key := endpoint + "|" + apiKey
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.cache[key]; ok {
		return c
	}
	c := clients.NewEmbedding(endpoint, apiKey)
	r.cache[key] = c
	return c
}
