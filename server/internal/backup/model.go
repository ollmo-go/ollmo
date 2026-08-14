package backup

// Export is the JSON structure of a KB backup. It contains the KB metadata
// and all its documents and chunks so a KB can be fully restored on import.
type Export struct {
	KnowledgeBase KB      `json:"knowledge_base"`
	Documents     []Doc   `json:"documents"`
	Chunks        []Chunk `json:"chunks"`
	ExportedAt    string  `json:"exported_at"`
}

type KB struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	EmbeddingModelID string `json:"embedding_model_id"`
	ChunkStrategy    string `json:"chunk_strategy"`
	ChunkSize        int    `json:"chunk_size"`
	ChunkOverlap     int    `json:"chunk_overlap"`
}

type Doc struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Chunk struct {
	ID          string `json:"id"`
	DocID       string `json:"doc_id"`
	Index       int    `json:"index"`
	Content     string `json:"content"`
	PageNumbers string `json:"page_numbers"`
}
