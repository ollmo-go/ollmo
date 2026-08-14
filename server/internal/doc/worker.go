package doc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/minio/minio-go/v7"

	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/graph"
	"ollmo/ollmo/internal/kb"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/internal/pipeline"
	"ollmo/ollmo/pkg/chunker"
	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/localparser"
	"ollmo/ollmo/pkg/tokener"
	"ollmo/ollmo/pkg/vector"
)

// isTextFile returns true for plain-text formats that MinerU is not needed
// for. The worker reads them directly from MinIO and skips the parse API.
func isTextFile(objectKey string) bool {
	k := strings.ToLower(objectKey)
	for _, ext := range []string{".txt", ".md", ".markdown", ".csv", ".json", ".log"} {
		if strings.HasSuffix(k, ext) {
			return true
		}
	}
	return false
}

// PipelineConfigFetcher returns the typed ingestion config for a KB. When nil
// or returning nil, the worker falls back to KB-level defaults. Defined as a
// function type so the doc package stays independent of how the config is
// produced; the server composition layer wires pipeline.Service.ExtractConfig.
type PipelineConfigFetcher func(ctx context.Context, tenantID, kbID string) (*pipeline.IngestionConfig, error)

// Worker is the Asynq handler for doc:parse, doc:embed, and doc:extract tasks.
// One instance is shared across all task invocations; it is safe for concurrent
// use because all mutable state lives on *gorm.DB (pooled) and the injected
// clients.
type Worker struct {
	svc          *Service
	kbRepo       *kb.Repo
	minio        *minio.Client
	bucket       string
	mineru       *clients.MinerUClient
	embedder     embedding.Embedder
	store        *vector.Store
	pipelineCfg  PipelineConfigFetcher
	graphSvc     *graph.Service
	llmRepo      *llm.Repo
	quotaChecker QuotaChecker
}

func NewWorker(
	svc *Service,
	kbRepo *kb.Repo,
	mc *minio.Client,
	bucket string,
	mineru *clients.MinerUClient,
	embedder embedding.Embedder,
	store *vector.Store,
	pipelineCfg PipelineConfigFetcher,
	graphSvc *graph.Service,
	llmRepo *llm.Repo,
) *Worker {
	return &Worker{
		svc: svc, kbRepo: kbRepo, minio: mc, bucket: bucket, mineru: mineru,
		embedder: embedder, store: store,
		pipelineCfg: pipelineCfg, graphSvc: graphSvc, llmRepo: llmRepo,
	}
}

// WithQuotaChecker injects vector quota enforcement into the embed worker.
// When set, HandleEmbed checks that adding the doc's chunks would not exceed
// the tenant's vector quota before running the embedding API.
func (w *Worker) WithQuotaChecker(qc QuotaChecker) *Worker {
	w.quotaChecker = qc
	return w
}

// markFailedAndLog marks the doc as failed and logs any error from the status
// update itself. The original failure reason is already in the message; this
// only guards against the status write failing silently.
func (w *Worker) markFailedAndLog(tenantID, docID, reason string) {
	if err := w.svc.MarkFailed(tenantID, docID, reason); err != nil {
		log.Printf("[worker:doc] failed to mark doc %s as failed: %v", docID, err)
	}
}

// HandleParse is the asynq.HandlerFunc for TaskParseDocument.
//
//  1. mark doc parsing
//  2. presign MinIO URL for MinerU
//  3. submit parse task to MinerU + poll for completion
//  4. save parsed markdown to MinIO
//  5. chunk the markdown using KB's chunk_size/overlap
//  6. save chunks to MySQL
//  7. mark doc parsed (ready for embedding in the next phase)
func (w *Worker) HandleParse(ctx context.Context, t *asynq.Task) error {
	p, err := DecodeParseDocumentPayload(t)
	if err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}

	logf := func(format string, args ...any) {
		log.Printf("[worker:doc:parse] tenant=%s kb=%s doc=%s: %s",
			p.TenantID, p.KbID, p.DocID, fmt.Sprintf(format, args...))
	}
	logf("starting")

	if err := w.svc.MarkParsing(p.TenantID, p.DocID); err != nil {
		return fmt.Errorf("mark parsing: %w", err)
	}

	// NOTE: do NOT delete existing chunks/vectors before parsing. A failed
	// parse (or an asynq retry) must leave the previous chunk set intact;
	// replacement happens atomically after the new chunks are ready.

	kbCfg, err := w.kbRepo.FindByID(p.TenantID, p.KbID)
	if err != nil {
		w.markFailedAndLog(p.TenantID, p.DocID, "kb not found: "+err.Error())
		return err
	}

	// Load the canvas-defined pipeline config. Falls back to KB-level fields
	// (chunk_strategy/size/overlap) when no pipeline exists yet, so legacy
	// KBs keep working unchanged.
	pc := w.loadPipelineConfig(ctx, p.TenantID, p.KbID, kbCfg)

	// Parse the document: text files are read directly from MinIO, while
	// binary formats (PDF, DOCX, …) go through MinerU for deep parsing.
	var markdown string
	if isTextFile(p.ObjectKey) {
		logf("text file, reading directly from minio")
		obj, err := w.minio.GetObject(ctx, w.bucket, p.ObjectKey, minio.GetObjectOptions{})
		if err != nil {
			w.markFailedAndLog(p.TenantID, p.DocID, "get object: "+err.Error())
			return fmt.Errorf("get object: %w", err)
		}
		defer obj.Close()
		b, err := io.ReadAll(obj)
		if err != nil {
			w.markFailedAndLog(p.TenantID, p.DocID, "read object: "+err.Error())
			return fmt.Errorf("read object: %w", err)
		}
		markdown = string(b)
	} else {
		// Try MinerU first for high-quality parsing (OCR, layout, tables).
		// Fall back to local parser when MinerU is unavailable.
		md, err := w.parseWithMinerU(ctx, p, pc, logf)
		if err != nil {
			logf("mineru unavailable (%v), falling back to local parser", err)
			md, err = w.parseLocal(ctx, p, logf)
			if err != nil {
				w.markFailedAndLog(p.TenantID, p.DocID, "parse: "+err.Error())
				return fmt.Errorf("parse: %w", err)
			}
		}
		markdown = md
	}
	logf("parsed, markdown=%d bytes", len(markdown))

	// Save parsed markdown next to the original.
	parsedKey := strings.Replace(p.ObjectKey, "/original", "/parsed.md", 1)
	if _, err := w.minio.PutObject(ctx, w.bucket, parsedKey,
		bytes.NewReader([]byte(markdown)), int64(len(markdown)),
		minio.PutObjectOptions{ContentType: "text/markdown"},
	); err != nil {
		w.markFailedAndLog(p.TenantID, p.DocID, "save parsed: "+err.Error())
		return fmt.Errorf("save parsed: %w", err)
	}

	// Chunk and persist using the pipeline chunker config.
	chunks := chunker.SplitMarkdown(markdown, pc.Chunker.Strategy, pc.Chunker.Size, pc.Chunker.Overlap)
	logf("chunked into %d pieces (strategy=%s)", len(chunks), pc.Chunker.Strategy)

	chunkModels := make([]*Chunk, 0, len(chunks))
	for i, c := range chunks {
		chunkModels = append(chunkModels, &Chunk{
			ID:         uuid.NewString(),
			TenantID:   p.TenantID,
			KbID:       p.KbID,
			DocID:      p.DocID,
			Index:      i,
			Content:    c,
			TokenCount: tokener.Estimate(c),
		})
	}
	// Atomically swap the old chunk set for the new one. Old chunks are only
	// deleted once the new parse + chunking succeeded, so a failure above
	// leaves the previous data untouched.
	if err := w.svc.ReplaceChunks(ctx, p.TenantID, p.KbID, p.DocID, chunkModels); err != nil {
		w.markFailedAndLog(p.TenantID, p.DocID, "replace chunks: "+err.Error())
		return fmt.Errorf("replace chunks: %w", err)
	}

	// Old Milvus vectors reference the old chunk IDs and are now orphaned.
	// Derived data only: on failure we log and let the embed pass overwrite
	// what it can; stale IDs are filtered out by the MySQL join at query time.
	if w.store != nil {
		if err := w.store.DeleteByDoc(ctx, p.KbID, p.DocID); err != nil {
			logf("warn: milvus delete by doc failed: %v", err)
		}
	}

	if err := w.svc.MarkParsed(p.TenantID, p.DocID, parsedKey, len(chunks)); err != nil {
		return fmt.Errorf("mark parsed: %w", err)
	}

	logf("done, %d chunks saved, enqueuing embed", len(chunks))
	if err := w.svc.EnqueueEmbed(ctx, p.TenantID, p.KbID, p.DocID); err != nil {
		// Non-fatal for parse: doc is in parsed state and can be re-embedded
		// from the UI. Log and continue so parse does not retry.
		logf("enqueue embed failed: %v", err)
	}
	return nil
}

// HandleEmbed is the asynq.HandlerFunc for TaskEmbedDocument. It re-reads
// chunks from MySQL (so payload drift does not matter), runs them through the
// embedding model pinned on the KB, and upserts vectors into the KB's Milvus
// collection. The collection is created lazily on first use.
//
//  1. mark doc embedding
//  2. load KB (model + chunk strategy)
//  3. ensure Milvus collection exists + index + load
//  4. load chunks from MySQL
//  5. batch embed + upsert into Milvus
//  6. mark doc ready
func (w *Worker) HandleEmbed(ctx context.Context, t *asynq.Task) error {
	p, err := DecodeEmbedDocumentPayload(t)
	if err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}

	logf := func(format string, args ...any) {
		log.Printf("[worker:doc:embed] tenant=%s kb=%s doc=%s: %s",
			p.TenantID, p.KbID, p.DocID, fmt.Sprintf(format, args...))
	}
	logf("starting")

	if err := w.svc.MarkEmbedding(p.TenantID, p.DocID); err != nil {
		return fmt.Errorf("mark embedding: %w", err)
	}

	kbCfg, err := w.kbRepo.FindByID(p.TenantID, p.KbID)
	if err != nil {
		w.markFailedAndLog(p.TenantID, p.DocID, "kb not found: "+err.Error())
		return err
	}
	pc := w.loadPipelineConfig(ctx, p.TenantID, p.KbID, kbCfg)
	// The KB pins an embedding model by id; resolve it to the model name
	// expected by EnsureCollection/Embed and the provider's batch size.
	model, embBatch, err := w.embedder.ResolveModel(ctx, p.TenantID, kbCfg.EmbeddingModelID)
	if err != nil {
		w.markFailedAndLog(p.TenantID, p.DocID, "resolve embedding: "+err.Error())
		return fmt.Errorf("resolve embedding: %w", err)
	}
	batchSize := pc.Embedder.BatchSize
	if batchSize <= 0 {
		batchSize = embBatch
	}
	if batchSize <= 0 {
		batchSize = w.embedder.ResolveBatchSize(ctx, p.TenantID, model)
	}

	if err := w.store.EnsureCollection(ctx, p.KbID, model); err != nil {
		w.markFailedAndLog(p.TenantID, p.DocID, "ensure collection: "+err.Error())
		return fmt.Errorf("ensure collection: %w", err)
	}

	chunks, err := w.svc.ListAllChunksByDoc(ctx, p.TenantID, p.KbID, p.DocID)
	if err != nil {
		w.markFailedAndLog(p.TenantID, p.DocID, "list chunks: "+err.Error())
		return fmt.Errorf("list chunks: %w", err)
	}
	if len(chunks) == 0 {
		w.markFailedAndLog(p.TenantID, p.DocID, "no chunks to embed")
		return fmt.Errorf("no chunks for doc %s", p.DocID)
	}

	// Enforce vector quota before calling the embedding API so we do not
	// burn LLM tokens on a doc that would be rejected at upsert time.
	// Reparse already deleted old chunks, so delta = len(chunks) is correct.
	if w.quotaChecker != nil {
		if err := w.quotaChecker.CheckVectorDelta(p.TenantID, len(chunks)); err != nil {
			w.markFailedAndLog(p.TenantID, p.DocID, "vector quota: "+err.Error())
			return fmt.Errorf("vector quota for doc %s: %w", p.DocID, err)
		}
	}

	logf("loaded %d chunks, embedding with %s (batch=%d)", len(chunks), model, batchSize)

	// Embed in batches and upsert as we go so a late failure leaves partial
	// vectors in Milvus (acceptable; re-running embed is idempotent upsert).
	vectorIDs := make(map[string]string, len(chunks))
	totalUpserted := 0
	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[start:end]

		inputs := make([]string, len(batch))
		for i, c := range batch {
			inputs[i] = c.Content
		}
		vectors, err := w.embedder.Embed(ctx, p.TenantID, model, inputs)
		if err != nil {
			w.markFailedAndLog(p.TenantID, p.DocID, "embed batch: "+err.Error())
			return fmt.Errorf("embed batch %d-%d: %w", start, end, err)
		}

		records := make([]vector.ChunkRecord, len(batch))
		for i, c := range batch {
			records[i] = vector.ChunkRecord{
				ID:        c.ID,
				TenantID:  c.TenantID,
				KbID:      c.KbID,
				DocID:     c.DocID,
				Content:   c.Content,
				Embedding: vectors[i],
			}
			vectorIDs[c.ID] = c.ID
		}
		n, err := w.store.Upsert(ctx, p.KbID, records)
		if err != nil {
			w.markFailedAndLog(p.TenantID, p.DocID, "milvus upsert: "+err.Error())
			return fmt.Errorf("upsert batch %d-%d: %w", start, end, err)
		}
		totalUpserted += n
		logf("upserted batch %d-%d (%d vectors)", start, end, n)
	}

	// Record Milvus primary keys on the chunks. We use chunk.ID as the PK, so
	// this is mostly a flag that embedding succeeded.
	if err := w.svc.SetChunkVectorIDs(p.TenantID, p.DocID, vectorIDs); err != nil {
		logf("warn: set vector ids failed: %v", err)
	}

	if err := w.svc.MarkEmbedded(p.TenantID, p.DocID); err != nil {
		return fmt.Errorf("mark embedded: %w", err)
	}
	logf("done, %d vectors upserted, doc ready", totalUpserted)

	// Enqueue knowledge graph extraction. Non-fatal: if the graph service or
	// LLM provider is unavailable, the doc stays ready for vector retrieval.
	if w.graphSvc != nil && w.llmRepo != nil {
		if err := w.svc.EnqueueExtract(ctx, p.TenantID, p.KbID, p.DocID); err != nil {
			logf("warn: enqueue extract failed: %v", err)
		}
	}
	return nil
}

// HandleExtract is the asynq.HandlerFunc for TaskExtractDocument. It loads the
// document's chunks, resolves the tenant's default LLM provider, and asks the
// LLM to extract entities and relations from each chunk. Failures are per-chunk
// so one bad response does not abort the whole pass.
func (w *Worker) HandleExtract(ctx context.Context, t *asynq.Task) error {
	p, err := DecodeExtractDocumentPayload(t)
	if err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	logf := func(format string, args ...any) {
		log.Printf("[worker:doc:extract] tenant=%s kb=%s doc=%s: %s",
			p.TenantID, p.KbID, p.DocID, fmt.Sprintf(format, args...))
	}
	if w.graphSvc == nil || w.llmRepo == nil {
		logf("graph service or llm repo not configured, skipping")
		return nil
	}

	provider, err := w.llmRepo.FindDefault(p.TenantID)
	if err != nil {
		logf("no default llm provider, skipping: %v", err)
		return nil
	}
	logf("using llm provider %s (%s)", provider.Name, provider.Model)

	chunks, err := w.svc.ListAllChunksByDoc(ctx, p.TenantID, p.KbID, p.DocID)
	if err != nil {
		return fmt.Errorf("list chunks: %w", err)
	}
	if len(chunks) == 0 {
		return nil
	}

	extracted := 0
	for _, c := range chunks {
		if err := w.graphSvc.ExtractFromChunk(ctx, p.TenantID, p.KbID, c.ID, c.Content,
			provider.Endpoint, provider.APIKey, provider.Model); err != nil {
			logf("warn: chunk %s extraction failed: %v", c.ID, err)
			continue
		}
		extracted++
	}
	logf("done, %d/%d chunks extracted", extracted, len(chunks))
	return nil
}

// loadPipelineConfig resolves the ingestion config for a KB. It prefers the
// canvas-defined pipeline; when no pipeline exists (or the fetcher is unset),
// it falls back to KB-level fields so legacy KBs keep working unchanged.
func (w *Worker) loadPipelineConfig(ctx context.Context, tenantID, kbID string, kbCfg *kb.KnowledgeBase) pipeline.IngestionConfig {
	pc := pipeline.IngestionConfig{
		Chunker: pipeline.ChunkerConfig{Strategy: kbCfg.ChunkStrategy, Size: kbCfg.ChunkSize, Overlap: kbCfg.ChunkOverlap},
		// Embedding model is pinned at the KB level via embedding_model_id
		// (resolved in HandleEmbed); the canvas only carries batch size.
		Embedder: pipeline.EmbedderConfig{BatchSize: 32},
		Parser:   pipeline.ParserConfig{Engine: "auto", OCR: true, Formula: true, Table: true},
	}
	if w.pipelineCfg == nil {
		return pc
	}
	pipe, err := w.pipelineCfg(ctx, tenantID, kbID)
	if err != nil || pipe == nil {
		return pc
	}
	if pipe.Parser.Engine != "" {
		pc.Parser.Engine = pipe.Parser.Engine
	}
	pc.Parser.OCR = pipe.Parser.OCR
	pc.Parser.Formula = pipe.Parser.Formula
	pc.Parser.Table = pipe.Parser.Table
	if pipe.Chunker.Strategy != "" {
		pc.Chunker.Strategy = pipe.Chunker.Strategy
	}
	if pipe.Chunker.Size > 0 {
		pc.Chunker.Size = pipe.Chunker.Size
	}
	if pipe.Chunker.Overlap >= 0 {
		pc.Chunker.Overlap = pipe.Chunker.Overlap
	}
	// Embedding model is pinned at the KB level via embedding_model_id (it
	// determines the Milvus collection dimension). The pipeline canvas may
	// only override the batch size, not the model.
	if pipe.Embedder.BatchSize > 0 {
		pc.Embedder.BatchSize = pipe.Embedder.BatchSize
	}
	return pc
}

// parseWithMinerU submits the document to MinerU for high-quality parsing
// (OCR, layout recognition, table extraction). Returns an error if MinerU is
// unavailable so the caller can fall back to local parsing.
func (w *Worker) parseWithMinerU(ctx context.Context, p ParseDocumentPayload, pc pipeline.IngestionConfig, logf func(string, ...any)) (string, error) {
	if w.mineru == nil {
		return "", fmt.Errorf("mineru client not configured")
	}
	presignedURL, err := w.minio.PresignedGetObject(ctx, w.bucket, p.ObjectKey, time.Hour, nil)
	if err != nil {
		return "", fmt.Errorf("presign: %w", err)
	}
	taskID, err := w.mineru.SubmitParse(ctx, clients.ParseRequest{
		FileURL:       presignedURL.String(),
		EnableFormula: pc.Parser.Formula,
		EnableTable:   pc.Parser.Table,
		EnableOCR:     pc.Parser.OCR,
	})
	if err != nil {
		return "", fmt.Errorf("submit: %w", err)
	}
	logf("mineru task_id=%s, waiting", taskID)
	result, err := w.mineru.Wait(ctx, taskID, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("wait: %w", err)
	}
	return result.Markdown, nil
}

// parseLocal reads the file from MinIO and extracts text using built-in
// parsers (DOCX via ZIP/XML, PDF via ledongthuc/pdf). No OCR or layout
// recognition — just plain text extraction as a fallback.
func (w *Worker) parseLocal(ctx context.Context, p ParseDocumentPayload, logf func(string, ...any)) (string, error) {
	obj, err := w.minio.GetObject(ctx, w.bucket, p.ObjectKey, minio.GetObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("get object: %w", err)
	}
	defer obj.Close()
	filename := p.ObjectKey
	if i := strings.LastIndex(filename, "/"); i >= 0 {
		filename = filename[i+1:]
	}
	text, err := localparser.Extract(obj, filename)
	if err != nil {
		return "", fmt.Errorf("local parse: %w", err)
	}
	logf("local parser extracted %d bytes from %s", len(text), filename)
	return text, nil
}
