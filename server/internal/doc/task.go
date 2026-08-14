package doc

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

// Asynq task types and payload schemas. doc:parse drives MinerU parsing and
// chunking; doc:embed runs after parse and writes vectors to Milvus; doc:extract
// runs after embed and builds the knowledge graph (entities + relations). They
// are split so each stage can retry independently.
const (
	TaskParseDocument   = "doc:parse"
	TaskEmbedDocument   = "doc:embed"
	TaskExtractDocument = "doc:extract"
)

type ParseDocumentPayload struct {
	TenantID  string `json:"tenant_id"`
	KbID      string `json:"kb_id"`
	DocID     string `json:"doc_id"`
	ObjectKey string `json:"object_key"`
}

func NewParseDocumentTask(p ParseDocumentPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskParseDocument, b), nil
}

func DecodeParseDocumentPayload(t *asynq.Task) (ParseDocumentPayload, error) {
	var p ParseDocumentPayload
	err := json.Unmarshal(t.Payload(), &p)
	return p, err
}

// EmbedDocumentPayload carries only IDs; the worker re-reads chunks from MySQL
// to stay resilient to payload drift between parse and embed.
type EmbedDocumentPayload struct {
	TenantID string `json:"tenant_id"`
	KbID     string `json:"kb_id"`
	DocID    string `json:"doc_id"`
}

func NewEmbedDocumentTask(p EmbedDocumentPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskEmbedDocument, b), nil
}

func DecodeEmbedDocumentPayload(t *asynq.Task) (EmbedDocumentPayload, error) {
	var p EmbedDocumentPayload
	err := json.Unmarshal(t.Payload(), &p)
	return p, err
}

// ExtractDocumentPayload triggers entity extraction for a document's chunks.
// It reuses EmbedDocumentPayload's shape (tenant + kb + doc IDs).
func NewExtractDocumentTask(p EmbedDocumentPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskExtractDocument, b), nil
}

func DecodeExtractDocumentPayload(t *asynq.Task) (EmbedDocumentPayload, error) {
	var p EmbedDocumentPayload
	err := json.Unmarshal(t.Payload(), &p)
	return p, err
}
