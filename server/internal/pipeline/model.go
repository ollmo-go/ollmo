package pipeline

import "time"

// Pipeline is the per-KB ingestion configuration stored as a JSON DAG. The
// frontend renders it on a React Flow canvas; the worker extracts a typed
// IngestionConfig from it to drive parsing, chunking, and embedding.
//
// One row per KB (unique on kb_id). A new version is written when the user
// saves edits, but only the latest is read by the worker.
type Pipeline struct {
	ID         string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID   string    `gorm:"size:36;index;not null" json:"tenant_id"`
	KbID       string    `gorm:"size:36;uniqueIndex;not null" json:"kb_id"`
	Version    int       `gorm:"not null;default:1" json:"version"`
	Definition string    `gorm:"type:text" json:"definition"`
	Active     bool      `gorm:"not null;default:true" json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Pipeline) TableName() string { return "pipelines" }

// Definition is the typed view of the pipeline JSON. It mirrors the React Flow
// graph shape so the frontend can round-trip the canvas state without translation.
type Definition struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Node is one processing stage. Type selects the config schema in Data.
type Node struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Position Position              `json:"position"`
	Data     map[string]interface{} `json:"data"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// Node type identifiers. The canvas ships one of each by default; future node
// types (e.g. reranker-in-pipeline, knowledge graph extractor) extend this set.
const (
	NodeSource   = "source"
	NodeParser   = "parser"
	NodeChunker  = "chunker"
	NodeEmbedder = "embedder"
	NodeSink     = "sink"
)

// IngestionConfig is the flat, typed config the worker consumes. It is derived
// from the Definition so the worker does not handle raw JSON at execution time.
type IngestionConfig struct {
	Parser   ParserConfig
	Chunker  ChunkerConfig
	Embedder EmbedderConfig
}

type ParserConfig struct {
	Engine  string // "auto" picks MinerU for binary files, direct read for text
	OCR     bool
	Formula bool
	Table   bool
}

type ChunkerConfig struct {
	Strategy string
	Size     int
	Overlap  int
}

type EmbedderConfig struct {
	Model     string
	BatchSize int
}
