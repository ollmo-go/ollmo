package kb

import (
	"context"
	"log"

	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/vector"

	"github.com/google/uuid"
)

// EmbeddingDefaultFinder returns the tenant's default embedding model id.
// When nil or erroring, the KB service leaves the id empty and Create fails
// with a validation error.
type EmbeddingDefaultFinder func(tenantID string) (string, error)

// EmbeddingResolver validates an embedding model id and returns its vector
// dimension. Implemented by the server layer (wrapping embedding.Repo) so kb
// does not import the embedding package (which would create a circular
// dependency since doc imports kb).
type EmbeddingResolver interface {
	Resolve(tenantID, id string) (dim int, err error)
}

type Service struct {
	repo          *Repo
	store         *vector.Store
	reembedder    Reembedder
	annSyncer     AnnSyncer
	embedDefault  EmbeddingDefaultFinder
	embedResolver EmbeddingResolver
}

// Reembedder re-enqueues embed tasks for all docs in a KB. Implemented by
// doc.Service; kept as an interface here so kb does not import doc (which
// would create a circular dependency since doc already imports kb).
type Reembedder interface {
	ReembedAll(ctx context.Context, tenantID, kbID string) error
}

// AnnSyncer keeps annotation vectors in sync with KB lifecycle events.
// Implemented by annotation.Service; kept as an interface here to avoid a
// circular dependency (annotation imports kb for the repo).
type AnnSyncer interface {
	SyncOnDelete(ctx context.Context, tenantID, kbID string) error
	SyncOnEmbeddingChange(ctx context.Context, tenantID, kbID string) error
}

func NewService(repo *Repo, store *vector.Store) *Service {
	return &Service{repo: repo, store: store}
}

func (s *Service) WithReembedder(r Reembedder) *Service {
	s.reembedder = r
	return s
}

// WithAnnSyncer wires the annotation cleanup/rebuild hooks.
func (s *Service) WithAnnSyncer(a AnnSyncer) *Service {
	s.annSyncer = a
	return s
}

// WithEmbeddingDefault wires the tenant default embedding model resolver.
// When set, Create/Update resolve an empty embedding_model_id to the tenant's
// default embedding model id.
func (s *Service) WithEmbeddingDefault(f EmbeddingDefaultFinder) *Service {
	s.embedDefault = f
	return s
}

// WithEmbeddingResolver wires the id validator. When set, Create/Update
// validate the embedding_model_id by looking it up and checking its dim so
// the Milvus collection can be created later without surprises.
func (s *Service) WithEmbeddingResolver(r EmbeddingResolver) *Service {
	s.embedResolver = r
	return s
}

type CreateInput struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	EmbeddingModelID string `json:"embedding_model_id"`
	ChunkStrategy    string `json:"chunk_strategy"`
	ChunkSize        int    `json:"chunk_size"`
	ChunkOverlap     int    `json:"chunk_overlap"`
}

type UpdateInput struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	EmbeddingModelID *string `json:"embedding_model_id"`
}

type ListQuery struct {
	Page int
	Size int
}

func (s *Service) Create(ctx context.Context, tenantID, ownerID string, in CreateInput) (*KnowledgeBase, error) {
	if in.Name == "" {
		return nil, errs.BadRequest("name is required")
	}
	if in.EmbeddingModelID == "" {
		in.EmbeddingModelID = s.resolveDefaultEmbeddingModel(tenantID)
	}
	// The embedding model is optional at creation time: a KB without
	// documents (e.g. a minimal LLM-only agent) never needs one. It becomes
	// required when the first document is uploaded (enforced by the doc
	// service). When provided, fail fast if the id does not resolve so the
	// Milvus collection can be created later without surprises.
	if in.EmbeddingModelID != "" && s.embedResolver != nil {
		if _, err := s.embedResolver.Resolve(tenantID, in.EmbeddingModelID); err != nil {
			return nil, errs.BadRequest(err.Error())
		}
	}
	if in.ChunkStrategy == "" {
		in.ChunkStrategy = "parent_child"
	}
	if in.ChunkSize == 0 {
		in.ChunkSize = 500
	}
	if in.ChunkOverlap == 0 {
		in.ChunkOverlap = 50
	}
	k := &KnowledgeBase{
		ID:               uuid.NewString(),
		TenantID:         tenantID,
		Name:             in.Name,
		Description:      in.Description,
		EmbeddingModelID: in.EmbeddingModelID,
		ChunkStrategy:    in.ChunkStrategy,
		ChunkSize:        in.ChunkSize,
		ChunkOverlap:     in.ChunkOverlap,
		OwnerID:          ownerID,
		Visibility:       "private",
	}
	if err := s.repo.Create(k); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create kb", err)
	}
	return k, nil
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (*KnowledgeBase, error) {
	return s.repo.FindByID(tenantID, id)
}

func (s *Service) List(ctx context.Context, tenantID, userID string, q ListQuery, allTenant bool) ([]*KnowledgeBase, int64, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.Size <= 0 || q.Size > 100 {
		q.Size = 20
	}
	return s.repo.List(tenantID, userID, q.Page, q.Size, allTenant)
}

func (s *Service) Update(ctx context.Context, tenantID, id string, in UpdateInput) (*KnowledgeBase, error) {
	k, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		k.Name = *in.Name
	}
	if in.Description != nil {
		k.Description = *in.Description
	}
	// Embedding model change: validate the new id, drop the old Milvus
	// collection (vectors are incompatible across models), and re-embed all
	// existing docs with the new model. An empty string means "use tenant
	// default", resolved the same way as Create.
	if in.EmbeddingModelID != nil && *in.EmbeddingModelID != k.EmbeddingModelID {
		newID := *in.EmbeddingModelID
		if newID == "" {
			newID = s.resolveDefaultEmbeddingModel(tenantID)
		}
		if newID != "" && newID != k.EmbeddingModelID {
			if s.embedResolver != nil {
				if _, err := s.embedResolver.Resolve(tenantID, newID); err != nil {
					return nil, errs.BadRequest(err.Error())
				}
			}
			if s.store != nil {
				if err := s.store.DropCollection(ctx, id); err != nil {
					log.Printf("[kb] drop collection failed kb=%s: %v", id, err)
				}
			}
			k.EmbeddingModelID = newID
			if s.reembedder != nil {
				if err := s.reembedder.ReembedAll(ctx, tenantID, id); err != nil {
					return nil, errs.Wrap(errs.CodeInternal, "reembed docs", err)
				}
			}
			// Annotation question vectors share the KB's embedding model;
			// rebuild them against the new model.
			if s.annSyncer != nil {
				if err := s.annSyncer.SyncOnEmbeddingChange(ctx, tenantID, id); err != nil {
					log.Printf("[kb] rebuild annotation vectors failed kb=%s: %v", id, err)
				}
			}
		}
	}
	if err := s.repo.Update(k); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "update kb", err)
	}
	return k, nil
}

// resolveDefaultEmbeddingModel returns the tenant's default embedding model
// id. Returns an empty string when no resolver is wired or no default model
// is configured; Create/Update turn that into a validation error.
func (s *Service) resolveDefaultEmbeddingModel(tenantID string) string {
	if s.embedDefault != nil {
		if id, err := s.embedDefault(tenantID); err == nil && id != "" {
			return id
		}
	}
	return ""
}

func (s *Service) Delete(ctx context.Context, tenantID, id string) error {
	if s.store != nil {
		if err := s.store.DropCollection(ctx, id); err != nil {
			log.Printf("[kb] drop collection failed kb=%s: %v", id, err)
		}
	}
	if s.annSyncer != nil {
		if err := s.annSyncer.SyncOnDelete(ctx, tenantID, id); err != nil {
			log.Printf("[kb] cleanup annotations failed kb=%s: %v", id, err)
		}
	}
	return s.repo.Delete(tenantID, id)
}

func (s *Service) SetVisibility(ctx context.Context, tenantID, id, visibility string) error {
	if visibility != VisibilityPrivate && visibility != VisibilityTeam {
		return errs.BadRequest("visibility must be private or team")
	}
	k, err := s.repo.FindByID(tenantID, id)
	if err != nil {
		return err
	}
	k.Visibility = visibility
	return s.repo.Update(k)
}
