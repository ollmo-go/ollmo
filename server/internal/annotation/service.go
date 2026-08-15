package annotation

import (
	"context"
	"log"
	"strings"

	"github.com/google/uuid"

	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/kb"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/vector"
)

// matchThreshold is the minimum cosine similarity between the user question
// and an annotation question for the stored answer to be served directly.
// Paraphrases of the same question typically score 0.80+ with bge-class
// embedding models, while unrelated questions stay below 0.5.
const matchThreshold = 0.80

// syncBatch bounds how many questions are embedded per call, mirroring the
// doc worker's embedding batching.
const syncBatch = 32

type Service struct {
	repo     *Repo
	kbRepo   *kb.Repo
	store    *vector.Store
	embedder embedding.Embedder
}

func NewService(repo *Repo, kbRepo *kb.Repo, store *vector.Store, embedder embedding.Embedder) *Service {
	return &Service{repo: repo, kbRepo: kbRepo, store: store, embedder: embedder}
}

type CreateInput struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

type UpdateInput struct {
	Question *string `json:"question,omitempty"`
	Answer   *string `json:"answer,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
}

// MatchResult is a similarity hit: the annotation whose question matched and
// the cosine score that qualified it.
type MatchResult struct {
	Annotation *Annotation
	Score      float64
}

func (s *Service) List(ctx context.Context, tenantID, kbID string, page, size int) ([]*Annotation, int64, error) {
	return s.repo.List(tenantID, kbID, page, size)
}

func (s *Service) Create(ctx context.Context, tenantID, kbID string, in CreateInput) (*Annotation, error) {
	in.Question = strings.TrimSpace(in.Question)
	in.Answer = strings.TrimSpace(in.Answer)
	if in.Question == "" || in.Answer == "" {
		return nil, errs.BadRequest("question and answer are required")
	}
	modelName, err := s.requireEmbedding(ctx, tenantID, kbID)
	if err != nil {
		return nil, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	a := &Annotation{
		ID: uuid.NewString(), TenantID: tenantID, KbID: kbID,
		Question: in.Question, Answer: in.Answer, Enabled: enabled,
	}
	if err := s.repo.Create(a); err != nil {
		return nil, err
	}
	if a.Enabled {
		if err := s.syncVectors(ctx, tenantID, kbID, modelName, []*Annotation{a}); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (s *Service) Update(ctx context.Context, tenantID, kbID, id string, in UpdateInput) (*Annotation, error) {
	if _, err := s.repo.Find(tenantID, kbID, id); err != nil {
		return nil, err
	}
	if in.Question != nil {
		*in.Question = strings.TrimSpace(*in.Question)
		if *in.Question == "" {
			return nil, errs.BadRequest("question must not be empty")
		}
	}
	if in.Answer != nil && strings.TrimSpace(*in.Answer) == "" {
		return nil, errs.BadRequest("answer must not be empty")
	}

	cols := map[string]any{}
	if in.Question != nil {
		cols["question"] = *in.Question
	}
	if in.Answer != nil {
		cols["answer"] = strings.TrimSpace(*in.Answer)
	}
	if in.Enabled != nil {
		cols["enabled"] = *in.Enabled
	}
	if err := s.repo.Update(tenantID, kbID, id, cols); err != nil {
		return nil, err
	}
	updated, err := s.repo.Find(tenantID, kbID, id)
	if err != nil {
		return nil, err
	}

	// Re-sync the vector: question text changed while enabled, or the
	// enabled flag flipped in either direction.
	questionChanged := in.Question != nil
	if questionChanged || in.Enabled != nil {
		if updated.Enabled {
			modelName, err := s.requireEmbedding(ctx, tenantID, kbID)
			if err != nil {
				return nil, err
			}
			if err := s.syncVectors(ctx, tenantID, kbID, modelName, []*Annotation{updated}); err != nil {
				return nil, err
			}
		} else if s.store != nil {
			if err := s.store.DeleteAnnotation(ctx, kbID, id); err != nil {
				log.Printf("[annotation] delete vector failed kb=%s ann=%s: %v", kbID, id, err)
			}
		}
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, tenantID, kbID, id string) error {
	if err := s.repo.Delete(tenantID, kbID, id); err != nil {
		return err
	}
	if s.store != nil {
		if err := s.store.DeleteAnnotation(ctx, kbID, id); err != nil {
			log.Printf("[annotation] delete vector failed kb=%s ann=%s: %v", kbID, id, err)
		}
	}
	return nil
}

// Match embeds the query with the KB's embedding model and searches the
// annotation collection. Returns nil (never an error) when there is no
// qualified hit: annotation matching is best-effort and must not break chat.
func (s *Service) Match(ctx context.Context, tenantID, kbID, query string) *MatchResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	// Short-circuit before any external call: KBs without enabled
	// annotations are the common case and must not pay for an embedding.
	has, err := s.repo.HasEnabled(tenantID, kbID)
	if err != nil || !has {
		return nil
	}
	k, err := s.kbRepo.FindByID(tenantID, kbID)
	if err != nil || k.EmbeddingModelID == "" {
		return nil
	}
	modelName, _, err := s.embedder.ResolveModel(ctx, tenantID, k.EmbeddingModelID)
	if err != nil {
		return nil
	}
	vecs, err := s.embedder.Embed(ctx, tenantID, modelName, []string{query})
	if err != nil || len(vecs) == 0 {
		return nil
	}
	hits, err := s.store.SearchAnnotation(ctx, kbID, tenantID, vecs[0], 1)
	if err != nil {
		// Most common case: no annotation was ever created for this KB, so
		// the collection does not exist. Log and fall through to chat.
		log.Printf("[annotation] search failed kb=%s: %v", kbID, err)
		return nil
	}
	if len(hits) == 0 {
		return nil
	}
	log.Printf("[annotation] match kb=%s query=%q top=%.4f threshold=%.2f", kbID, query, hits[0].Score, matchThreshold)
	if hits[0].Score < matchThreshold {
		return nil
	}
	a, err := s.repo.Find(tenantID, kbID, hits[0].ID)
	if err != nil || !a.Enabled {
		return nil
	}
	return &MatchResult{Annotation: a, Score: float64(hits[0].Score)}
}

// SyncOnDelete implements kb.AnnSyncer: drop vectors, then rows.
func (s *Service) SyncOnDelete(ctx context.Context, tenantID, kbID string) error {
	if s.store != nil {
		if err := s.store.DropAnnotationCollection(ctx, kbID); err != nil {
			return err
		}
	}
	return s.repo.CleanupByKB(tenantID, kbID)
}

// SyncOnEmbeddingChange implements kb.AnnSyncer: rebuild all question
// vectors with the KB's new embedding model.
func (s *Service) SyncOnEmbeddingChange(ctx context.Context, tenantID, kbID string) error {
	items, err := s.repo.ListEnabled(tenantID, kbID)
	if err != nil {
		return err
	}
	if s.store != nil {
		if err := s.store.DropAnnotationCollection(ctx, kbID); err != nil {
			return err
		}
	}
	if len(items) == 0 {
		return nil
	}
	modelName, err := s.requireEmbedding(ctx, tenantID, kbID)
	if err != nil {
		return err
	}
	for i := 0; i < len(items); i += syncBatch {
		end := i + syncBatch
		if end > len(items) {
			end = len(items)
		}
		if err := s.syncVectors(ctx, tenantID, kbID, modelName, items[i:end]); err != nil {
			return err
		}
	}
	return nil
}

// requireEmbedding resolves the KB's embedding model name, failing with a
// clear message when the KB has none configured (annotations are useless
// without an embedding model).
func (s *Service) requireEmbedding(ctx context.Context, tenantID, kbID string) (string, error) {
	k, err := s.kbRepo.FindByID(tenantID, kbID)
	if err != nil {
		return "", err
	}
	if k.EmbeddingModelID == "" {
		return "", errs.BadRequest("knowledge base has no embedding model; configure one before adding annotations")
	}
	modelName, _, err := s.embedder.ResolveModel(ctx, tenantID, k.EmbeddingModelID)
	if err != nil {
		return "", err
	}
	return modelName, nil
}

// syncVectors embeds the questions and upserts them into the annotation
// collection, creating it on first use.
func (s *Service) syncVectors(ctx context.Context, tenantID, kbID, modelName string, items []*Annotation) error {
	inputs := make([]string, 0, len(items))
	for _, a := range items {
		inputs = append(inputs, a.Question)
	}
	vecs, err := s.embedder.Embed(ctx, tenantID, modelName, inputs)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "embed annotation questions", err)
	}
	records := make([]vector.ChunkRecord, 0, len(items))
	for i, a := range items {
		if i >= len(vecs) {
			break
		}
		records = append(records, vector.ChunkRecord{
			ID: a.ID, TenantID: tenantID, KbID: kbID,
			Content: a.Question, Embedding: vecs[i],
		})
	}
	if err := s.store.EnsureAnnotationCollection(ctx, kbID, modelName); err != nil {
		return err
	}
	_, err = s.store.UpsertAnnotation(ctx, kbID, records)
	return err
}
