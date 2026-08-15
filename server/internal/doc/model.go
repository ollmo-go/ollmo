package doc

import "time"

// Document status lifecycle:
//   queued -> parsing -> parsed -> embedding -> ready
//   any  -> failed (parse_error holds the reason)
const (
	StatusQueued    = "queued"
	StatusParsing   = "parsing"
	StatusParsed    = "parsed"
	StatusFailed    = "failed"
	StatusEmbedding = "embedding"
	StatusReady     = "ready"
)

// Document is a tenant-scoped file uploaded to a knowledge base. ObjectKey
// points at the original in MinIO; ParsedObjectKey holds the parsed markdown.
type Document struct {
	ID              string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID        string    `gorm:"size:36;not null;index:idx_doc_tenant_kb,priority:1;index:idx_doc_tenant_kb_status,priority:1" json:"tenant_id"`
	KbID            string    `gorm:"size:36;not null;index:idx_doc_tenant_kb,priority:2;index:idx_doc_tenant_kb_status,priority:2" json:"kb_id"`
	Name            string    `gorm:"size:255;not null" json:"name"`
	Size            int64     `gorm:"not null;default:0" json:"size"`
	MimeType        string    `gorm:"size:128" json:"mime_type"`
	ObjectKey       string    `gorm:"size:512;not null" json:"object_key"`
	ParsedObjectKey string    `gorm:"size:512" json:"parsed_object_key"`
	SourceURL       string    `gorm:"size:1024" json:"source_url,omitempty"`
	// Metadata holds a JSON object of user-defined key/value pairs used as
	// retrieval filters (e.g. {"source":"hr"}). Stored as text for portability;
	// MySQL JSON functions still operate on the valid JSON string.
	Metadata    string    `gorm:"type:text" json:"metadata,omitempty"`
	Status      string    `gorm:"size:32;not null;default:queued;index:idx_doc_tenant_kb_status,priority:3" json:"status"`
	ParseError  string    `gorm:"size:512" json:"parse_error,omitempty"`
	Enabled     bool      `gorm:"not null;default:true" json:"enabled"`
	ChunkCount  int       `gorm:"not null;default:0" json:"chunk_count"`
	OwnerID     string    `gorm:"size:36;index;not null" json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Chunk roles for the parent_child strategy. Children ("") are embedded and
// searched; parents ("parent") carry the wider context that replaces the child
// text in the final prompt. Legacy chunks keep the empty role.
const ChunkRoleParent = "parent"

// Chunk is a segment of a parsed document. VectorID is the Milvus primary key;
// it mirrors Chunk.ID, so a non-empty value also signals successful indexing.
type Chunk struct {
	ID          string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID    string    `gorm:"size:36;not null;index:idx_chunk_tenant_kb,priority:1;index:idx_chunk_tenant_doc,priority:1" json:"tenant_id"`
	KbID        string    `gorm:"size:36;not null;index:idx_chunk_tenant_kb,priority:2" json:"kb_id"`
	DocID       string    `gorm:"size:36;not null;index:idx_chunk_tenant_doc,priority:2" json:"doc_id"`
	ParentID    string    `gorm:"size:36;index:idx_chunk_parent" json:"parent_id,omitempty"`
	Role        string    `gorm:"size:16;not null;default:''" json:"role,omitempty"`
	Index       int       `gorm:"column:idx;not null" json:"index"`
	Content     string    `gorm:"type:text" json:"content"`
	TokenCount  int       `gorm:"not null;default:0" json:"token_count"`
	PageNumbers string    `gorm:"size:64" json:"page_numbers"`
	VectorID    string    `gorm:"size:36" json:"vector_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// CleanupTask records a pending Milvus/MinIO cleanup that failed during doc
// deletion. A background sweep retries these to reclaim orphaned resources.
type CleanupTask struct {
	ID          string     `gorm:"primaryKey;size:36" json:"id"`
	TenantID    string     `gorm:"size:36;index;not null" json:"tenant_id"`
	KbID        string     `gorm:"size:36;not null" json:"kb_id"`
	DocID       string     `gorm:"size:36;not null" json:"doc_id"`
	Kind        string     `gorm:"size:16;not null" json:"kind"` // "milvus" or "minio"
	ObjectKey   string     `gorm:"size:512" json:"object_key"`   // minio key or milvus collection name
	Status      string     `gorm:"size:16;not null;default:pending;index" json:"status"`
	Attempts    int        `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts int        `gorm:"not null;default:5" json:"max_attempts"`
	NextRetryAt *time.Time `gorm:"index" json:"next_retry_at,omitempty"`
	Error       string     `gorm:"size:512" json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (CleanupTask) TableName() string { return "cleanup_tasks" }
