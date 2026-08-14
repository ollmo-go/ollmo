package doc

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"

	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/kb"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/vector"
)

type Service struct {
	repo         *Repo
	kbRepo       *kb.Repo
	quotaChecker QuotaChecker
	minio        *minio.Client
	bucket       string
	asynq        *asynq.Client
	store        *vector.Store
	embedder     embedding.Embedder
	events       *EventBus
}

// QuotaChecker verifies tenant resource limits. The router wires this to
// tenant.Repo so doc stays decoupled from the tenant package.
type QuotaChecker interface {
	CheckDocQuota(tenantID string) error
	CheckVectorDelta(tenantID string, delta int) error
}

func NewService(repo *Repo, kbRepo *kb.Repo, mc *minio.Client, bucket string, asynqClient *asynq.Client, store *vector.Store, embedder embedding.Embedder) *Service {
	return &Service{repo: repo, kbRepo: kbRepo, minio: mc, bucket: bucket, asynq: asynqClient, store: store, embedder: embedder}
}

func (s *Service) WithQuotaChecker(qc QuotaChecker) *Service {
	s.quotaChecker = qc
	return s
}

func (s *Service) WithEventBus(bus *EventBus) *Service {
	s.events = bus
	return s
}

type UploadResult struct {
	Doc *Document `json:"document"`
}

// Upload stores the file in MinIO, writes the document row, and enqueues a
// parse task. The caller passes an open reader; the service does not close it.
func (s *Service) Upload(ctx context.Context, tenantID, ownerID, kbID, filename, mimeType string, size int64, body io.Reader) (*Document, error) {
	k, err := s.kbRepo.FindByID(tenantID, kbID)
	if err != nil {
		return nil, err
	}
	// Lazy embedding requirement: a KB may be created without an embedding
	// model (pure-chat, zero docs). Uploading the first document is the
	// moment it becomes mandatory.
	if k.EmbeddingModelID == "" {
		return nil, errs.BadRequest("knowledge base has no embedding model; configure one before uploading documents")
	}

	if s.quotaChecker != nil {
		if err := s.quotaChecker.CheckDocQuota(tenantID); err != nil {
			return nil, err
		}
	}

	docID := uuid.NewString()
	ext := filepath.Ext(filename)
	objectKey := fmt.Sprintf("docs/%s/%s/%s/original%s", tenantID, kbID, docID, ext)

	if _, err := s.minio.PutObject(ctx, s.bucket, objectKey, body, size, minio.PutObjectOptions{
		ContentType: mimeType,
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "upload to minio", err)
	}

	doc := &Document{
		ID:        docID,
		TenantID:  tenantID,
		KbID:      kbID,
		Name:      filename,
		Size:      size,
		MimeType:  mimeType,
		ObjectKey: objectKey,
		Status:    StatusQueued,
		OwnerID:   ownerID,
	}
	// Create doc row and bump KB doc_count in one transaction so the count
	// stays accurate for quota checks.
	if err := s.repo.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(doc).Error; err != nil {
			return err
		}
		return tx.Model(&kb.KnowledgeBase{}).
			Where("tenant_id = ? AND id = ?", tenantID, kbID).
			UpdateColumn("doc_count", gorm.Expr("doc_count + 1")).Error
	}); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create doc + count", err)
	}

	task, err := NewParseDocumentTask(ParseDocumentPayload{
		TenantID: tenantID, KbID: kbID, DocID: docID, ObjectKey: objectKey,
	})
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "build parse task", err)
	}
	if _, err := s.asynq.EnqueueContext(ctx, task,
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Minute),
		asynq.Queue(QueuePipeline),
	); err != nil {
		// Mark the doc as failed but don't roll back the upload; the user can
		// trigger a reparse from the UI.
		if uerr := s.repo.UpdateDocStatus(tenantID, docID, StatusFailed, "enqueue: "+err.Error()); uerr != nil {
			log.Printf("[doc] failed to mark doc %s as failed: %v", docID, uerr)
		}
		return nil, errs.Wrap(errs.CodeInternal, "enqueue parse task", err)
	}

	return doc, nil
}

func (s *Service) Get(ctx context.Context, tenantID, kbID, id string) (*Document, error) {
	return s.repo.FindDoc(tenantID, kbID, id)
}

// GetContent returns the parsed document content (markdown/text) stored in
// MinIO. Used by the document viewer for citation tracing.
func (s *Service) GetContent(ctx context.Context, tenantID, kbID, docID string) (*Document, string, error) {
	doc, err := s.repo.FindDoc(tenantID, kbID, docID)
	if err != nil {
		return nil, "", err
	}
	if doc.ParsedObjectKey == "" {
		return doc, "", nil
	}
	obj, err := s.minio.GetObject(ctx, s.bucket, doc.ParsedObjectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "fetch parsed content", err)
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "read parsed content", err)
	}
	return doc, string(data), nil
}

func (s *Service) List(ctx context.Context, tenantID, kbID string, page, size int) ([]*Document, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return s.repo.ListDocs(tenantID, kbID, page, size)
}

func (s *Service) SetEnabled(ctx context.Context, tenantID, kbID, id string, enabled bool) error {
	return s.repo.SetDocEnabled(tenantID, kbID, id, enabled)
}

func (s *Service) ListChunks(ctx context.Context, tenantID, kbID, docID string, page, size int) ([]*Chunk, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 500 {
		size = 200
	}
	return s.repo.ListChunksByDoc(tenantID, kbID, docID, size, (page-1)*size)
}

// ListAllChunksByDoc returns all chunks for a document without pagination.
// Used by the worker (embedding/extraction) which needs every chunk.
func (s *Service) ListAllChunksByDoc(ctx context.Context, tenantID, kbID, docID string) ([]*Chunk, error) {
	return s.repo.ListChunksByDoc(tenantID, kbID, docID, 0, 0)
}

// ReplaceChunks atomically swaps a document's chunks: old rows are deleted and
// new rows inserted in one transaction. Called by the parse worker only AFTER
// parsing and chunking succeeded, so a failed reparse never destroys the
// previous chunk set (issue: reparse used to delete first, losing data on
// parse failure and on every asynq retry).
func (s *Service) ReplaceChunks(ctx context.Context, tenantID, kbID, docID string, chunks []*Chunk) error {
	if err := s.repo.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND kb_id = ? AND doc_id = ?", tenantID, kbID, docID).
			Delete(&Chunk{}).Error; err != nil {
			return err
		}
		if len(chunks) == 0 {
			return nil
		}
		return tx.CreateInBatches(chunks, 100).Error
	}); err != nil {
		return errs.Wrap(errs.CodeInternal, "replace chunks", err)
	}
	return nil
}

// Delete removes the document, its chunks, the MinIO objects, the Milvus
// vectors, and decrements the KB doc count. DB operations run in a single
// transaction so the doc row, chunks, and count stay consistent. Milvus/MinIO
// cleanup failures are recorded in cleanup_tasks for a background sweep to
// retry — the DB transaction is the source of truth for "doc is gone".
func (s *Service) Delete(ctx context.Context, tenantID, kbID, id string) error {
	doc, err := s.repo.FindDoc(tenantID, kbID, id)
	if err != nil {
		return err
	}

	// DB ops in one transaction: chunks + doc + doc_count.
	if err := s.repo.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND doc_id = ?", tenantID, id).Delete(&Chunk{}).Error; err != nil {
			return err
		}
		res := tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&Document{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errs.NotFound("document not found")
		}
		// CASE WHEN instead of GREATEST so the expression also works on
		// SQLite (used by unit tests); MySQL supports both.
		return tx.Model(&kb.KnowledgeBase{}).
			Where("tenant_id = ? AND id = ?", tenantID, doc.KbID).
			UpdateColumn("doc_count", gorm.Expr("CASE WHEN doc_count > 0 THEN doc_count - 1 ELSE 0 END")).Error
	}); err != nil {
		return errs.Wrap(errs.CodeInternal, "delete doc tx", err)
	}

	// Best-effort cleanup of external storage. Failures are logged and
	// recorded in cleanup_tasks rather than failing the request.
	if s.store != nil {
		if err := s.store.DeleteByDoc(ctx, doc.KbID, id); err != nil {
			log.Printf("[cleanup] milvus delete failed doc=%s kb=%s: %v", id, doc.KbID, err)
			s.enqueueCleanup(tenantID, doc.KbID, id, "milvus", doc.KbID, err)
		}
	}
	if doc.ObjectKey != "" {
		if err := s.minio.RemoveObject(ctx, s.bucket, doc.ObjectKey, minio.RemoveObjectOptions{}); err != nil {
			log.Printf("[cleanup] minio delete failed key=%s: %v", doc.ObjectKey, err)
			s.enqueueCleanup(tenantID, doc.KbID, id, "minio", doc.ObjectKey, err)
		}
	}
	if doc.ParsedObjectKey != "" {
		if err := s.minio.RemoveObject(ctx, s.bucket, doc.ParsedObjectKey, minio.RemoveObjectOptions{}); err != nil {
			log.Printf("[cleanup] minio delete failed key=%s: %v", doc.ParsedObjectKey, err)
			s.enqueueCleanup(tenantID, doc.KbID, id, "minio", doc.ParsedObjectKey, err)
		}
	}
	return nil
}

// enqueueCleanup records a failed storage cleanup so a background sweep can
// retry it. Errors here are logged only — we never fail the Delete request
// because the DB row is already gone.
func (s *Service) enqueueCleanup(tenantID, kbID, docID, kind, objectKey string, cleanupErr error) {
	task := &CleanupTask{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		KbID:      kbID,
		DocID:     docID,
		Kind:      kind,
		ObjectKey: objectKey,
		Status:    "pending",
		Error:     cleanupErr.Error(),
	}
	if err := s.repo.DB().Create(task).Error; err != nil {
		log.Printf("[cleanup] failed to record cleanup_task doc=%s kind=%s: %v", docID, kind, err)
	}
}

// Reparse re-enqueues the parse task for an existing document. Useful when a
// previous parse failed and the user has fixed the upstream issue.
func (s *Service) Reparse(ctx context.Context, tenantID, kbID, id string) error {
	doc, err := s.repo.FindDoc(tenantID, kbID, id)
	if err != nil {
		return err
	}
	task, err := NewParseDocumentTask(ParseDocumentPayload{
		TenantID: tenantID, KbID: doc.KbID, DocID: doc.ID, ObjectKey: doc.ObjectKey,
	})
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "build parse task", err)
	}
	if _, err := s.asynq.EnqueueContext(ctx, task, asynq.MaxRetry(3), asynq.Timeout(30*time.Minute), asynq.Queue(QueuePipeline)); err != nil {
		return errs.Wrap(errs.CodeInternal, "enqueue parse task", err)
	}
	return s.repo.UpdateDocStatus(tenantID, id, StatusQueued, "")
}

// EnqueueEmbed submits a doc:embed task for an already-parsed document. Called
// by the parse worker when chunking finishes.
func (s *Service) EnqueueEmbed(ctx context.Context, tenantID, kbID, docID string) error {
	task, err := NewEmbedDocumentTask(EmbedDocumentPayload{
		TenantID: tenantID, KbID: kbID, DocID: docID,
	})
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "build embed task", err)
	}
	if _, err := s.asynq.EnqueueContext(ctx, task,
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Minute),
		asynq.Queue(QueuePipeline),
	); err != nil {
		return errs.Wrap(errs.CodeInternal, "enqueue embed task", err)
	}
	return nil
}

// ReembedAll re-enqueues the embed task for every document in a KB. Used when
// the KB's embedding model changes: the Milvus collection is dropped by the
// caller, then each doc is re-embedded with the new model. Docs without chunks
// (not yet parsed) are skipped — they will be embedded normally on first parse.
func (s *Service) ReembedAll(ctx context.Context, tenantID, kbID string) error {
	docIDs, err := s.repo.ListDocIDsByKB(tenantID, kbID)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "list docs", err)
	}
	for _, docID := range docIDs {
		if err := s.repo.UpdateDocStatus(tenantID, docID, StatusEmbedding, ""); err != nil {
			log.Printf("[doc] reembed: mark embedding failed doc=%s: %v", docID, err)
		}
		if err := s.EnqueueEmbed(ctx, tenantID, kbID, docID); err != nil {
			return err
		}
	}
	return nil
}

// EnqueueExtract submits a doc:extract task after embedding completes. The
// extract worker builds the knowledge graph (entities + relations) from the
// document's chunks using the LLM. Non-fatal: if enqueue fails, the doc is
// still ready for retrieval; entities can be extracted on reparse.
func (s *Service) EnqueueExtract(ctx context.Context, tenantID, kbID, docID string) error {
	task, err := NewExtractDocumentTask(EmbedDocumentPayload{
		TenantID: tenantID, KbID: kbID, DocID: docID,
	})
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "build extract task", err)
	}
	if _, err := s.asynq.EnqueueContext(ctx, task,
		asynq.MaxRetry(2),
		asynq.Timeout(20*time.Minute),
		asynq.Queue(QueuePipeline),
	); err != nil {
		return errs.Wrap(errs.CodeInternal, "enqueue extract task", err)
	}
	return nil
}

func (s *Service) SetChunkVectorIDs(tenantID, docID string, vectorIDs map[string]string) error {
	return s.repo.SetDocVectorIDs(tenantID, docID, vectorIDs)
}

// WithTx clones the service with a tx-bound repo. Used when the caller needs
// transactional consistency (e.g. parse worker writing chunks + status).
func (s *Service) WithTx(tx *gorm.DB) *Service {
	return &Service{
		repo:   NewRepo(tx),
		kbRepo: kb.NewRepo(tx),
		minio:  s.minio, bucket: s.bucket, asynq: s.asynq, store: s.store, embedder: s.embedder,
		quotaChecker: s.quotaChecker,
		events:       s.events,
	}
}

// UpdateChunkContent edits a chunk's text and re-embeds it so the new
// content is immediately searchable. The Milvus upsert uses the same chunk
// ID (primary key), so the old vector is overwritten in place.
func (s *Service) UpdateChunkContent(ctx context.Context, tenantID, kbID, chunkID, content string) (*Chunk, error) {
	chunk, err := s.repo.FindChunk(tenantID, kbID, chunkID)
	if err != nil {
		return nil, err
	}
	if content == "" {
		return nil, errs.BadRequest("content is required")
	}

	if err := s.repo.UpdateChunkContent(tenantID, kbID, chunkID, content); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "update chunk", err)
	}
	chunk.Content = content

	// Re-embed if the KB has an embedding model and a configured embedder.
	if s.embedder != nil && s.store != nil {
		kbCfg, err := s.kbRepo.FindByID(tenantID, chunk.KbID)
		if err == nil {
			model, _, err := s.embedder.ResolveModel(ctx, tenantID, kbCfg.EmbeddingModelID)
			if err != nil {
				return nil, errs.Wrap(errs.CodeInternal, "resolve embedding model", err)
			}
			vectors, err := s.embedder.Embed(ctx, tenantID, model, []string{content})
			if err != nil {
				return nil, errs.Wrap(errs.CodeInternal, "re-embed chunk", err)
			}
			if len(vectors) > 0 {
				record := vector.ChunkRecord{
					ID: chunk.ID, TenantID: tenantID, KbID: chunk.KbID,
					DocID: chunk.DocID, Content: content, Embedding: vectors[0],
				}
				if _, err := s.store.Upsert(ctx, chunk.KbID, []vector.ChunkRecord{record}); err != nil {
					return nil, errs.Wrap(errs.CodeInternal, "re-embed chunk upsert", err)
				}
			}
		}
	}
	return chunk, nil
}

// DeleteChunk removes a single chunk and its Milvus vector. The document's
// chunk_count is decremented; the doc itself stays.
func (s *Service) DeleteChunk(ctx context.Context, tenantID, kbID, chunkID string) error {
	chunk, err := s.repo.FindChunk(tenantID, kbID, chunkID)
	if err != nil {
		return err
	}
	if s.store != nil {
		if err := s.store.DeleteByID(ctx, chunk.KbID, chunkID); err != nil {
			return errs.Wrap(errs.CodeInternal, "delete chunk vector", err)
		}
	}
	if err := s.repo.DeleteChunk(tenantID, kbID, chunkID); err != nil {
		return err
	}
	// Decrement doc chunk_count; non-fatal if it drifts.
	if err := s.repo.DecDocChunkCount(tenantID, chunk.DocID); err != nil {
		log.Printf("[doc] decrement chunk_count failed doc=%s: %v", chunk.DocID, err)
	}
	return nil
}
