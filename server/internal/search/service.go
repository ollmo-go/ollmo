package search

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"ollmo/ollmo/internal/doc"
	"ollmo/ollmo/internal/embedding"
	"ollmo/ollmo/internal/graph"
	"ollmo/ollmo/internal/kb"
	"ollmo/ollmo/internal/rerank"
	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/vector"
)

// Service orchestrates retrieval for a single KB. It owns the embedding call
// (query -> vector), the dense Milvus search, the lexical MySQL FULLTEXT
// search (TF-IDF variant ranking), RRF fusion across the two rank lists, and optional
// graph-based entity context from LLM entity extraction. When a
// Reranker is configured the fused results are re-scored by a cross-encoder
// for better precision.
type Service struct {
	kbRepo   *kb.Repo
	docRepo  *doc.Repo
	embedder embedding.Embedder
	store    *vector.Store
	reranker rerank.Reranker
	graphSvc *graph.Service
}

func NewService(kbRepo *kb.Repo, docRepo *doc.Repo, embedder embedding.Embedder, store *vector.Store, reranker rerank.Reranker, graphSvc *graph.Service) *Service {
	return &Service{kbRepo: kbRepo, docRepo: docRepo, embedder: embedder, store: store, reranker: reranker, graphSvc: graphSvc}
}

// denseSearch runs the vector leg: embed the query, search Milvus, then
// filter disabled documents and metadata in Go (Milvus rows carry neither).
func (s *Service) denseSearch(ctx context.Context, tenantID, kbID string, req SearchRequest) ([]vector.SearchHit, error) {
	// Embed query. The KB pins the embedding model by id; resolve it to the
	// model name the embedder expects.
	kbCfg, err := s.kbRepo.FindByID(tenantID, kbID)
	if err != nil {
		return nil, err
	}
	model, _, err := s.embedder.ResolveModel(ctx, tenantID, kbCfg.EmbeddingModelID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "resolve embedding model", err)
	}
	vectors, err := s.embedder.Embed(ctx, tenantID, model, []string{req.Query})
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "embed query", err)
	}
	if len(vectors) == 0 {
		return nil, errs.Internal("empty embedding for query")
	}

	denseHits, err := s.store.Search(ctx, kbID, tenantID, vectors[0], req.TopK*2)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "dense search", err)
	}

	// Exclude chunks belonging to disabled documents. The sparse leg handles
	// this via SQL JOIN; for dense we filter in Go because Milvus metadata
	// does not carry the enabled flag.
	disabledIDs, dErr := s.docRepo.ListDisabledDocIDs(tenantID, kbID)
	if dErr != nil {
		log.Printf("[search] list disabled docs failed tenant=%s kb=%s: %v", tenantID, kbID, dErr)
	}
	if len(disabledIDs) > 0 {
		disabledSet := make(map[string]bool, len(disabledIDs))
		for _, id := range disabledIDs {
			disabledSet[id] = true
		}
		filtered := denseHits[:0]
		for _, h := range denseHits {
			if !disabledSet[h.DocID] {
				filtered = append(filtered, h)
			}
		}
		denseHits = filtered
	}

	// Metadata filters: Milvus rows carry no document metadata, so dense
	// hits are filtered in Go against the doc metadata map. The sparse
	// leg applies the same filters in SQL.
	if len(req.Filters) > 0 {
		denseHits = s.filterDenseByMeta(tenantID, kbID, denseHits, req.Filters)
	}
	return denseHits, nil
}

// MaxTopK caps retrieval breadth for every caller (chat, agent nodes,
// retrieval test). TopK is often client-supplied; unbounded values inflate
// the prompt and multiply rerank API cost (candidates are fetched at TopK*3).
const MaxTopK = 20

// Search runs retrieval against one KB. The KB id pins the embedding model
// and the Milvus collection, so the caller must pass the same kb_id used at
// upload time.
func (s *Service) Search(ctx context.Context, tenantID, kbID string, req SearchRequest) (*SearchResult, error) {
	if req.Query == "" {
		return nil, errs.BadRequest("query is required")
	}
	if req.TopK <= 0 {
		req.TopK = 10
	}
	if req.TopK > MaxTopK {
		req.TopK = MaxTopK
	}

	kbCfg, err := s.kbRepo.FindByID(tenantID, kbID)
	if err != nil {
		return nil, err
	}

	// Sanitize metadata filters: keys outside [A-Za-z0-9_-] would break the
	// JSON path in the sparse SQL, so they are dropped instead of erroring.
	req.Filters = sanitizeFilters(req.Filters)

	// Empty KB shortcut: no documents means no Milvus collection was ever
	// created, so dense search would fail. Return an empty result instead of
	// erroring — the caller (chat/agent) simply answers without context.
	// This also saves the embedding API call for an empty KB.
	if kbCfg.DocCount == 0 {
		return &SearchResult{Hits: []SearchHit{}}, nil
	}

	// Defensive: a KB without an embedding model cannot do dense search.
	// Upload is blocked for such KBs so this should not happen in practice;
	// fall back to sparse-only retrieval rather than erroring.
	hasEmbedding := kbCfg.EmbeddingModelID != ""

	// Run the dense and sparse legs in parallel: neither depends on the
	// other's output and the embedding call dominates dense latency. Errors
	// keep their original severity — dense is fatal, sparse falls back to
	// dense-only retrieval.
	var (
		wg         sync.WaitGroup
		denseErr   error
		denseHits  []vector.SearchHit
		sparseHits []vector.SearchHit
	)
	if hasEmbedding {
		wg.Add(1)
		go func() {
			defer wg.Done()
			denseHits, denseErr = s.denseSearch(ctx, tenantID, kbID, req)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Lexical search via MySQL FULLTEXT on chunk content. Non-fatal:
		// a missing index or an empty match degrades to dense-only.
		sparseHits, _ = s.docRepo.SparseSearch(ctx, tenantID, kbID, req.Query, req.TopK*2, req.Filters)
	}()
	wg.Wait()
	if denseErr != nil {
		return nil, denseErr
	}

	// 4. RRF fusion across rank lists. VectorWeight (0..1) biases the fusion
	// towards the dense leg; nil keeps the equal-weight default.
	denseWeight, sparseWeight := normalizeVectorWeight(req.VectorWeight)
	var lists []rrfList
	if len(denseHits) > 0 {
		lists = append(lists, rrfList{hits: denseHits, weight: denseWeight})
	}
	if len(sparseHits) > 0 {
		lists = append(lists, rrfList{hits: sparseHits, weight: sparseWeight})
	}
	fused := rrf(lists)
	// Capture the full RRF ranking (before candidate trimming) for debug
	// observability so callers can see why each chunk was selected.
	var debugFusedScores map[string]float64
	if req.Debug {
		debugFusedScores = make(map[string]float64, len(fused))
		for _, f := range fused {
			debugFusedScores[f.ID] = f.Score
		}
	}
	// Keep enough candidates for rerank; trim to TopK only if rerank is off.
	candidateN := req.TopK
	rerankWanted := req.Rerank == nil || *req.Rerank
	rerankAvailable := s.reranker != nil && s.reranker.Available(ctx, tenantID, req.RerankModelID)
	if rerankWanted && rerankAvailable {
		candidateN = req.TopK * 3
		if candidateN < 20 {
			candidateN = 20
		}
	}
	if len(fused) > candidateN {
		fused = fused[:candidateN]
	}

	// 5. rehydrate doc names from MySQL. Chunk content already comes from
	// Milvus to avoid an extra round trip per hit.
	hits := make([]SearchHit, 0, len(fused))
	chunkIDs := make([]string, 0, len(fused))
	docIDs := make([]string, 0, len(fused))
	seen := make(map[string]bool, len(fused))
	for _, h := range fused {
		if !seen[h.DocID] {
			seen[h.DocID] = true
			docIDs = append(docIDs, h.DocID)
		}
		chunkIDs = append(chunkIDs, h.ID)
	}
	docNames, _ := s.docRepo.FindDocNames(tenantID, docIDs)
	pageMap, _ := s.docRepo.FindChunkPageNumbers(tenantID, chunkIDs)
	for _, h := range fused {
		hits = append(hits, SearchHit{
			ChunkID:     h.ID,
			DocID:       h.DocID,
			DocName:     docNames[h.DocID],
			Content:     h.Content,
			Score:       h.Score,
			PageNumbers: pageMap[h.ID],
		})
	}

	// Debug: snapshot the raw dense/sparse hits (pre-fusion) using the
	// already-loaded docNames so callers can compare leg scores to the
	// final RRF/rerank ranking. Empty when SearchRequest.Debug is false.
	var debugDenseHits, debugSparseHits []SearchHit
	if req.Debug {
		debugDenseHits = toDebugHits(denseHits, docNames)
		debugSparseHits = toDebugHits(sparseHits, docNames)
	}

	// 6. Optional cross-encoder rerank. Reranking re-scores the candidate
	// hits against the query with a bge-reranker (or similar) model and
	// returns them sorted by cross-encoder relevance. Failures degrade to
	// the RRF order instead of failing the whole search.
	if rerankWanted && rerankAvailable && len(hits) > 1 {
		inputs := make([]clients.RerankInput, len(hits))
		for i, h := range hits {
			inputs[i] = clients.RerankInput{DocID: h.ChunkID, Content: h.Content}
		}
		topN := req.TopK
		reranked, err := s.reranker.Rerank(ctx, tenantID, req.RerankModelID, req.Query, inputs, topN)
		if err == nil && len(reranked) > 0 {
			// Rebuild hits in rerank order, swapping scores to cross-encoder
			// relevance so the frontend can display the new ranking.
			ordered := make([]SearchHit, 0, len(reranked))
			for _, r := range reranked {
				for _, h := range hits {
					if h.ChunkID == r.DocID {
						h.Score = r.Score
						ordered = append(ordered, h)
						break
					}
				}
			}
			if len(ordered) > 0 {
				hits = ordered
			}
		}
	}
	// Final TopK trim. Rerank failures degrade to the RRF order, but the
	// expanded candidate set (TopK*3, min 20) must not leak through: callers
	// would stuff every candidate into the prompt. Also guards against a
	// rerank endpoint returning more entries than the requested top_n.
	if len(hits) > req.TopK {
		hits = hits[:req.TopK]
	}

	// Parent expansion for parent_child chunking: replace the matched child's
	// content with its wider parent text and dedupe siblings hitting the same
	// parent (keep the highest-scoring child's score/page). Legacy chunks
	// (no parent) pass through unchanged.
	hits = s.expandParents(tenantID, hits)

	// Hit statistics: bump chunks.hit_num for the final cited set. Only real
	// chat retrievals pass TrackHits; failures are non-fatal by design.
	if req.TrackHits && len(hits) > 0 {
		ids := make([]string, 0, len(hits))
		seenHit := make(map[string]bool, len(hits))
		for _, h := range hits {
			if !seenHit[h.ChunkID] {
				seenHit[h.ChunkID] = true
				ids = append(ids, h.ChunkID)
			}
		}
		if err := s.docRepo.IncrementChunkHits(tenantID, ids); err != nil {
			log.Printf("[search] track hits failed tenant=%s kb=%s: %v", tenantID, kbID, err)
		}
	}

	// 7. GraphRAG: query the knowledge graph for entities mentioned in the
	// query and include their descriptions + relationships as supplementary
	// context. Non-fatal: empty graph context is fine for KBs without
	// extraction or when no entities match the query.
	var graphContext string
	if s.graphSvc != nil {
		var err error
		graphContext, err = s.graphSvc.QueryForRetrieval(ctx, tenantID, kbID, req.Query, 8)
		if err != nil {
			log.Printf("[search] graph query failed tenant=%s kb=%s: %v", tenantID, kbID, err)
		}
	}

	return &SearchResult{
		Hits:             hits,
		Sparse:           len(sparseHits) > 0,
		Rerank:           rerankWanted && rerankAvailable,
		GraphContext:     graphContext,
		DebugDenseHits:   debugDenseHits,
		DebugSparseHits:  debugSparseHits,
		DebugFusedScores: debugFusedScores,
	}, nil
}

// SearchDebug runs Search with Debug enabled so the response carries the
// intermediate dense/sparse hits and RRF scores. Used by the debug endpoint
// to explain why each chunk was selected.
func (s *Service) SearchDebug(ctx context.Context, tenantID, kbID string, req SearchRequest) (*SearchResult, error) {
	req.Debug = true
	return s.Search(ctx, tenantID, kbID, req)
}

// toDebugHits converts raw vector hits (dense Milvus or lexical FULLTEXT) into the
// JSON-friendly SearchHit form, reusing docNames already loaded for the main
// hits to avoid extra DB round trips. DocName may be empty for hits whose
// doc wasn't looked up during the main path.
func toDebugHits(raw []vector.SearchHit, docNames map[string]string) []SearchHit {
	if len(raw) == 0 {
		return nil
	}
	out := make([]SearchHit, 0, len(raw))
	for _, h := range raw {
		out = append(out, SearchHit{
			ChunkID: h.ID,
			DocID:   h.DocID,
			DocName: docNames[h.DocID],
			Content: h.Content,
			Score:   float64(h.Score),
		})
	}
	return out
}

// expandParents maps child hits to their parent chunks and dedupes by parent.
// ChunkID stays the matched child (for page citations); Content becomes the
// parent text that reaches the LLM prompt.
func (s *Service) expandParents(tenantID string, hits []SearchHit) []SearchHit {
	if len(hits) == 0 {
		return hits
	}
	ids := make([]string, 0, len(hits))
	for _, h := range hits {
		ids = append(ids, h.ChunkID)
	}
	parentOf, err := s.docRepo.FindChunkParentIDs(tenantID, ids)
	if err != nil || len(parentOf) == 0 {
		return hits
	}
	parentIDs := make([]string, 0, len(parentOf))
	seen := make(map[string]bool, len(parentOf))
	for _, pid := range parentOf {
		if !seen[pid] {
			seen[pid] = true
			parentIDs = append(parentIDs, pid)
		}
	}
	parents, err := s.docRepo.FindChunksByIDs(tenantID, parentIDs)
	if err != nil {
		return hits
	}

	out := make([]SearchHit, 0, len(hits))
	emitted := make(map[string]bool, len(parentIDs))
	for _, h := range hits {
		pid := parentOf[h.ChunkID]
		if pid == "" {
			out = append(out, h)
			continue
		}
		if emitted[pid] {
			continue // sibling of an already-emitted parent
		}
		emitted[pid] = true
		if p, ok := parents[pid]; ok && p.Content != "" {
			h.Content = p.Content
		}
		h.ParentID = pid
		out = append(out, h)
	}
	return out
}

// filterDenseByMeta drops dense hits whose document metadata does not match
// every filter pair. Metadata JSON is small; parse per doc on demand.
func (s *Service) filterDenseByMeta(tenantID, kbID string, hits []vector.SearchHit, filters map[string]string) []vector.SearchHit {
	docMeta, err := s.docRepo.ListDocMeta(tenantID, kbID)
	if err != nil {
		log.Printf("[search] list doc meta failed tenant=%s kb=%s: %v", tenantID, kbID, err)
		return hits
	}
	if len(docMeta) == 0 {
		return nil // no doc carries metadata: nothing can match a filter
	}
	matched := make(map[string]bool, len(docMeta))
	for docID, raw := range docMeta {
		var meta map[string]string
		if json.Unmarshal([]byte(raw), &meta) != nil {
			continue
		}
		ok := true
		for k, v := range filters {
			if meta[k] != v {
				ok = false
				break
			}
		}
		if ok {
			matched[docID] = true
		}
	}
	out := hits[:0]
	for _, h := range hits {
		if matched[h.DocID] {
			out = append(out, h)
		}
	}
	return out
}

// sanitizeFilters drops entries with invalid keys or empty values.
func sanitizeFilters(filters map[string]string) map[string]string {
	if len(filters) == 0 {
		return nil
	}
	out := make(map[string]string, len(filters))
	for k, v := range filters {
		if doc.ValidMetadataKey(k) && v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
