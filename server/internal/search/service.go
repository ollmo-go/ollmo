package search

import (
	"context"
	"log"

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
// (query -> vector), the dense Milvus search, the sparse MySQL FULLTEXT
// (BM25-like) search, RRF fusion across the two rank lists, and optional
// graph-based entity context from the GraphRAG extraction pass. When a
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

	kbCfg, err := s.kbRepo.FindByID(tenantID, kbID)
	if err != nil {
		return nil, err
	}

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

	var denseHits []vector.SearchHit
	if hasEmbedding {
		// 1. embed query. The KB pins the embedding model by id; resolve it to
		// the model name the embedder expects.
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

		// 2. dense search on Milvus
		denseHits, err = s.store.Search(ctx, kbID, tenantID, vectors[0], req.TopK*2)
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
	}

	// 3. sparse search (BM25) via MySQL FULLTEXT on chunk content. Non-fatal:
	// if the index is missing or the query has no matches, we fall back to
	// dense-only retrieval so the search still returns results.
	sparseHits, _ := s.docRepo.SparseSearch(ctx, tenantID, kbID, req.Query, req.TopK*2)

	// 4. RRF fusion across rank lists
	var lists []rrfList
	if len(denseHits) > 0 {
		lists = append(lists, rrfList{hits: denseHits, weight: 1.0})
	}
	if len(sparseHits) > 0 {
		lists = append(lists, rrfList{hits: sparseHits, weight: 1.0})
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

// toDebugHits converts raw vector hits (dense Milvus or sparse BM25) into the
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
