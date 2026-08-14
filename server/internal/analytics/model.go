package analytics

// Overview aggregates high-level counts for a tenant.
type Overview struct {
	KnowledgeBases int64 `json:"knowledge_bases"`
	Documents      int64 `json:"documents"`
	Chunks         int64 `json:"chunks"`
	Conversations  int64 `json:"conversations"`
	Messages       int64 `json:"messages"`
	StorageBytes   int64 `json:"storage_bytes"`
}

// DocStatusCount is one row in the document-status breakdown.
type DocStatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

// DocStats summarizes document processing for a tenant.
type DocStats struct {
	Total       int64            `json:"total"`
	ByStatus    []DocStatusCount `json:"by_status"`
	SuccessRate float64          `json:"success_rate"`
	TotalChunks int64            `json:"total_chunks"`
}

// KBUsage is per-knowledge-base usage.
type KBUsage struct {
	KbID          string `json:"kb_id"`
	KbName        string `json:"kb_name"`
	DocCount      int64  `json:"doc_count"`
	ChunkCount    int64  `json:"chunk_count"`
	StorageBytes  int64  `json:"storage_bytes"`
	Conversations int64  `json:"conversations"`
}

// ActivityItem is one entry in the recent activity feed.
type ActivityItem struct {
	Kind      string `json:"kind"` // "document" or "chat"
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status,omitempty"`
	KbID      string `json:"kb_id"`
	CreatedAt string `json:"created_at"`
}
