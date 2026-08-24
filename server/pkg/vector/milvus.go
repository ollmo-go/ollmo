package vector

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"

	"ollmo/ollmo/pkg/errs"
)

// MaxContentLen is the upper bound for the chunk content stored inline in
// Milvus. Milvus VarChar caps at 65535; we keep room and use 8192 which
// comfortably holds the default 500-byte chunks plus margin for headers.
const MaxContentLen = 8192

// ChunkRecord is the per-chunk payload the doc worker builds and passes to
// the store for upsert. IDs become the Milvus primary key so deletes can
// reach the right vector without an extra mapping table.
type ChunkRecord struct {
	ID        string
	TenantID  string
	KbID      string
	DocID     string
	Content   string
	Embedding []float32
}

// Store owns Milvus collection lifecycle for knowledge bases. Each KB gets
// one collection named `kb_<kb_id>` because the embedding dimension and model
// are pinned at KB creation. Collections are created lazily on first upsert.
type Store struct {
	cli     client.Client
	mu      sync.Mutex
	ensured map[string]bool // collection name -> ensured
}

func NewStore(cli client.Client) *Store {
	return &Store{cli: cli, ensured: make(map[string]bool)}
}

// CollectionName returns the Milvus collection name for a KB.
func CollectionName(kbID string) string {
	return "kb_" + sanitize(kbID)
}

// AnnCollectionName returns the collection holding annotation question
// embeddings for a KB (annotation reply matching).
func AnnCollectionName(kbID string) string {
	return "ann_" + sanitize(kbID)
}

// EmbeddingDim returns the vector dimension for a well-known model name.
// Returns 0 for unknown models — callers should fall back to API detection
// (EmbeddingClient.DetectDim) in that case. The org prefix (e.g. "BAAI/" on
// SiliconFlow) is stripped before matching so both "bge-large-zh-v1.5" and
// "BAAI/bge-large-zh-v1.5" resolve the same.
func EmbeddingDim(model string) int {
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		model = model[idx+1:]
	}
	switch strings.ToLower(model) {
	case "bge-large-zh-v1.5", "bge-large-en-v1.5", "bge-m3":
		return 1024
	case "bge-base-zh-v1.5", "bge-base-en-v1.5":
		return 768
	case "bge-small-zh-v1.5", "bge-small-en-v1.5":
		return 512
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	case "embedding-3": // Zhipu embedding-3 default output dim
		return 2048
	case "text-embedding-v4": // Alibaba Qwen3-Embedding default dim
		return 1024
	default:
		return 0
	}
}

// EnsureCollection creates the KB chunk collection + HNSW index if missing
// and loads it. Idempotent; safe to call on every upsert. The same KB always
// maps to the same dim, so re-creation never collides.
func (s *Store) EnsureCollection(ctx context.Context, kbID string, dim int) error {
	return s.ensureNamed(ctx, CollectionName(kbID), dim)
}

// EnsureAnnotationCollection ensures the collection for annotation question
// embeddings of a KB. Same schema as the chunk collection; doc_id stays empty.
func (s *Store) EnsureAnnotationCollection(ctx context.Context, kbID string, dim int) error {
	return s.ensureNamed(ctx, AnnCollectionName(kbID), dim)
}

// ensureNamed creates a collection with the shared chunk schema if missing,
// creates the HNSW index, and loads it. Safe to call repeatedly.
func (s *Store) ensureNamed(ctx context.Context, coll string, dim int) error {
	if dim <= 0 {
		return fmt.Errorf("invalid embedding dimension %d for collection %q", dim, coll)
	}
	s.mu.Lock()
	if s.ensured[coll] {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	has, err := s.cli.HasCollection(ctx, coll)
	if err != nil {
		return fmt.Errorf("has collection: %w", err)
	}
	if !has {
		schema, err := buildSchema(coll, dim)
		if err != nil {
			return err
		}
		if err := s.cli.CreateCollection(ctx, schema, 1); err != nil {
			return fmt.Errorf("create collection: %w", err)
		}
		idx, err := entity.NewIndexHNSW(entity.COSINE, 16, 200)
		if err != nil {
			return fmt.Errorf("build hnsw index: %w", err)
		}
		if err := s.cli.CreateIndex(ctx, coll, "embedding", idx, false); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}
	if err := s.cli.LoadCollection(ctx, coll, true); err != nil {
		return fmt.Errorf("load collection: %w", err)
	}

	s.mu.Lock()
	s.ensured[coll] = true
	s.mu.Unlock()
	return nil
}

func buildSchema(name string, dim int) (*entity.Schema, error) {
	schema := entity.NewSchema().WithName(name).WithAutoID(false).WithDynamicFieldEnabled(false)
	fields := []*entity.Field{
		entity.NewField().WithName("id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(64).WithIsPrimaryKey(true),
		entity.NewField().WithName("tenant_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(36),
		entity.NewField().WithName("kb_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(36),
		entity.NewField().WithName("doc_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(36),
		entity.NewField().WithName("content").WithDataType(entity.FieldTypeVarChar).WithMaxLength(MaxContentLen),
		entity.NewField().WithName("embedding").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(dim)),
	}
	for _, f := range fields {
		schema = schema.WithField(f)
	}
	return schema, nil
}

// Upsert batches records into the KB's chunk collection. Records with no
// embedding (e.g. empty chunk) are skipped. Content is truncated to
// MaxContentLen so over-long chunks do not fail the whole batch.
func (s *Store) Upsert(ctx context.Context, kbID string, records []ChunkRecord) (int, error) {
	return s.UpsertNamed(ctx, CollectionName(kbID), records)
}

// UpsertAnnotation upserts annotation question embeddings into the KB's
// annotation collection. The record ID is the annotation ID.
func (s *Store) UpsertAnnotation(ctx context.Context, kbID string, records []ChunkRecord) (int, error) {
	return s.UpsertNamed(ctx, AnnCollectionName(kbID), records)
}

// UpsertNamed batches records into an arbitrary collection.
func (s *Store) UpsertNamed(ctx context.Context, coll string, records []ChunkRecord) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}

	ids := make([]string, 0, len(records))
	tenants := make([]string, 0, len(records))
	kbs := make([]string, 0, len(records))
	docs := make([]string, 0, len(records))
	contents := make([]string, 0, len(records))
	vectors := make([][]float32, 0, len(records))

	for _, r := range records {
		if len(r.Embedding) == 0 {
			continue
		}
		ids = append(ids, r.ID)
		tenants = append(tenants, r.TenantID)
		kbs = append(kbs, r.KbID)
		docs = append(docs, r.DocID)
		c := r.Content
		if len(c) > MaxContentLen {
			c = c[:MaxContentLen]
		}
		contents = append(contents, c)
		vectors = append(vectors, r.Embedding)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	colID := entity.NewColumnVarChar("id", ids)
	colTenant := entity.NewColumnVarChar("tenant_id", tenants)
	colKb := entity.NewColumnVarChar("kb_id", kbs)
	colDoc := entity.NewColumnVarChar("doc_id", docs)
	colContent := entity.NewColumnVarChar("content", contents)
	dim := vectorDim(vectors)
	colEmb := entity.NewColumnFloatVector("embedding", dim, vectors)

	if _, err := s.cli.Upsert(ctx, coll, "", colID, colTenant, colKb, colDoc, colContent, colEmb); err != nil {
		return 0, fmt.Errorf("milvus upsert: %w", err)
	}
	return len(ids), nil
}

// vectorDim returns the first vector's length, or 0 if empty. The Milvus
// column constructor requires the dimension up front.
func vectorDim(vectors [][]float32) int {
	if len(vectors) == 0 {
		return 0
	}
	return len(vectors[0])
}

// DeleteByDoc removes all vectors for a document. Used by doc.Delete.
func (s *Store) DeleteByDoc(ctx context.Context, kbID, docID string) error {
	return s.DeleteExpr(ctx, CollectionName(kbID), fmt.Sprintf("doc_id == %q", docID), "milvus delete by doc")
}

// DeleteByID removes a single vector by its chunk ID (Milvus primary key).
// Used when a user deletes an individual chunk from the UI.
func (s *Store) DeleteByID(ctx context.Context, kbID, chunkID string) error {
	return s.DeleteExpr(ctx, CollectionName(kbID), fmt.Sprintf("id == %q", chunkID), "milvus delete by id")
}

// DeleteAnnotation removes an annotation's question vector.
func (s *Store) DeleteAnnotation(ctx context.Context, kbID, annotationID string) error {
	return s.DeleteExpr(ctx, AnnCollectionName(kbID), fmt.Sprintf("id == %q", annotationID), "milvus delete annotation")
}

// DeleteExpr removes vectors matching a boolean expression in the collection.
func (s *Store) DeleteExpr(ctx context.Context, coll, expr, what string) error {
	if err := s.cli.Delete(ctx, coll, "", expr); err != nil {
		return errs.Wrap(errs.CodeInternal, what, err)
	}
	return nil
}

// SearchHit is one dense retrieval result. Score is the cosine similarity
// returned by Milvus (higher is better).
type SearchHit struct {
	ID      string
	DocID   string
	Content string
	Score   float32
}

// Search runs a dense ANN search on the KB's chunk collection, scoped to the
// tenant via an expression filter. The collection must already exist
// (EnsureCollection is called by the embed worker on first embed).
func (s *Store) Search(ctx context.Context, kbID, tenantID string, query []float32, topK int) ([]SearchHit, error) {
	return s.SearchNamed(ctx, CollectionName(kbID), tenantID, query, topK)
}

// SearchAnnotation runs a dense search over the KB's annotation questions.
func (s *Store) SearchAnnotation(ctx context.Context, kbID, tenantID string, query []float32, topK int) ([]SearchHit, error) {
	return s.SearchNamed(ctx, AnnCollectionName(kbID), tenantID, query, topK)
}

// SearchNamed runs a dense ANN search on an arbitrary collection.
func (s *Store) SearchNamed(ctx context.Context, coll, tenantID string, query []float32, topK int) ([]SearchHit, error) {
	if topK <= 0 {
		topK = 10
	}

	sp, err := entity.NewIndexHNSWSearchParam(64) // ef=64, good quality/speed tradeoff
	if err != nil {
		return nil, fmt.Errorf("build search param: %w", err)
	}
	vec := entity.FloatVector(query)
	expr := fmt.Sprintf("tenant_id == %q", tenantID)

	results, err := s.cli.Search(ctx, coll, []string{},
		expr,
		[]string{"id", "doc_id", "content"},
		[]entity.Vector{vec},
		"embedding",
		entity.COSINE,
		topK,
		sp,
	)
	if err != nil {
		return nil, fmt.Errorf("milvus search: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	r := results[0]
	if r.Err != nil {
		return nil, fmt.Errorf("milvus search result: %w", r.Err)
	}

	hits := make([]SearchHit, 0, r.ResultCount)
	for i := 0; i < r.ResultCount; i++ {
		idVal, err := r.IDs.GetAsString(i)
		if err != nil {
			continue
		}
		docCol := r.Fields.GetColumn("doc_id")
		contentCol := r.Fields.GetColumn("content")
		var docID, content string
		if docCol != nil {
			docID, err = docCol.GetAsString(i)
			if err != nil {
				log.Printf("[vector] get doc_id at index %d failed: %v", i, err)
			}
		}
		if contentCol != nil {
			content, err = contentCol.GetAsString(i)
			if err != nil {
				log.Printf("[vector] get content at index %d failed: %v", i, err)
			}
		}
		var score float32
		if i < len(r.Scores) {
			score = r.Scores[i]
		}
		hits = append(hits, SearchHit{
			ID: idVal, DocID: docID, Content: content, Score: score,
		})
	}
	return hits, nil
}

// DropCollection removes the KB chunk collection entirely. Used when a KB is
// deleted.
func (s *Store) DropCollection(ctx context.Context, kbID string) error {
	return s.DropNamed(ctx, CollectionName(kbID))
}

// DropAnnotationCollection removes the KB annotation collection. Used when a
// KB is deleted.
func (s *Store) DropAnnotationCollection(ctx context.Context, kbID string) error {
	return s.DropNamed(ctx, AnnCollectionName(kbID))
}

// DropNamed removes a collection by name if it exists.
func (s *Store) DropNamed(ctx context.Context, coll string) error {
	has, err := s.cli.HasCollection(ctx, coll)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	if err := s.cli.DropCollection(ctx, coll); err != nil {
		return fmt.Errorf("drop collection: %w", err)
	}
	s.mu.Lock()
	delete(s.ensured, coll)
	s.mu.Unlock()
	return nil
}

func sanitize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return errors.New("empty collection id").Error()
	}
	return string(out)
}
