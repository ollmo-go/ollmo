package search

// SearchRequest is the input for a retrieval query. The query is embedded
// via the model pinned on the KB for dense Milvus search, and also run
// through MySQL FULLTEXT for the lexical leg; the two rank lists are
// fused via RRF. VectorWeight (0..1) biases the fusion towards the dense
// leg; the sparse leg receives 1-VectorWeight. Nil keeps equal weights.
// Rerank toggles the optional cross-encoder rerank leg;
// when false or the reranker is not configured, results are returned in
// dense+sparse RRF order. Debug enables intermediate retrieval details
// (raw dense/sparse hits and RRF scores) on the response for observability.
type SearchRequest struct {
	Query         string   `json:"query"`
	TopK          int      `json:"top_k"`
	Rerank        *bool    `json:"rerank,omitempty"`
	RerankModelID string   `json:"rerank_model_id,omitempty"`
	VectorWeight  *float64 `json:"vector_weight,omitempty"`
	Debug         bool     `json:"debug,omitempty"`
}

// SearchHit is one retrieved chunk plus the doc it came from. The chat domain
// consumes this list to build the LLM context.
type SearchHit struct {
	ChunkID     string  `json:"chunk_id"`
	DocID       string  `json:"doc_id"`
	DocName     string  `json:"doc_name"`
	Content     string  `json:"content"`
	Score       float64 `json:"score"`
	PageNumbers string  `json:"page_numbers,omitempty"`
}

// SearchResult is the response envelope. Sparse and Rerank indicate which
// retrieval legs were active for this query. GraphContext carries the
// entity/relationship summary from GraphRAG; the chat service appends it to
// the system prompt so the LLM can reason about entity connections.
//
// The Debug* fields are only populated when SearchRequest.Debug is true, so
// callers can inspect why each chunk was selected (raw dense/sparse scores
// and the RRF-fused score per chunk_id).
type SearchResult struct {
	Hits         []SearchHit `json:"hits"`
	Sparse       bool        `json:"sparse"`
	Rerank       bool        `json:"rerank"`
	GraphContext string      `json:"graph_context,omitempty"`
	// Debug fields — only populated when SearchRequest.Debug is true.
	DebugDenseHits   []SearchHit        `json:"debug_dense_hits,omitempty"`
	DebugSparseHits  []SearchHit        `json:"debug_sparse_hits,omitempty"`
	DebugFusedScores map[string]float64 `json:"debug_fused_scores,omitempty"` // chunk_id -> RRF score
}
