package backup

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ollmo/ollmo/pkg/errs"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// ExportKB serializes a knowledge base's metadata, documents, and chunks to
// JSON. Vector embeddings are not included; re-indexing after import rebuilds
// them. The caller must have verified KB access before calling this.
func (s *Service) ExportKB(ctx context.Context, tenantID, kbID string) ([]byte, string, error) {
	var kb KB
	if err := s.db.Table("knowledge_bases").
		Where("tenant_id = ? AND id = ?", tenantID, kbID).
		Select("id, name, description, embedding_model_id, chunk_strategy, chunk_size, chunk_overlap").
		Scan(&kb).Error; err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "load kb", err)
	}
	if kb.ID == "" {
		return nil, "", errs.NotFound("knowledge base not found")
	}

	var docs []Doc
	if err := s.db.Table("documents").
		Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Select("id, name, status").
		Scan(&docs).Error; err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "load docs", err)
	}

	var chunks []Chunk
	if err := s.db.Table("chunks").
		Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Select("id, doc_id, idx AS `index`, content, page_numbers").
		Scan(&chunks).Error; err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "load chunks", err)
	}

	exp := Export{
		KnowledgeBase: kb,
		Documents:     docs,
		Chunks:        chunks,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return nil, "", errs.Wrap(errs.CodeInternal, "marshal backup", err)
	}
	return b, kb.Name, nil
}

// ImportKB restores documents and chunks into an existing KB from a backup
// JSON. Existing documents with the same id are skipped. Chunk indices are
// re-numbered within each document. The caller must have verified KB write
// access and the KB must already exist.
func (s *Service) ImportKB(ctx context.Context, tenantID, kbID string, data []byte) (int, error) {
	var exp Export
	if err := json.Unmarshal(data, &exp); err != nil {
		return 0, errs.BadRequest("invalid backup file: " + err.Error())
	}

	imported := 0
	for _, d := range exp.Documents {
		var existing int64
		s.db.Table("documents").
			Where("tenant_id = ? AND id = ?", tenantID, d.ID).Count(&existing)
		if existing > 0 {
			continue
		}
		// Insert a minimal document row in "ready" status. The original file
		// content is not part of the backup; only the parsed chunks matter.
		if err := s.db.Exec(`INSERT INTO documents (id, tenant_id, kb_id, name, size, mime_type, object_key, status, enabled, chunk_count, owner_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, 0, '', ?, 'ready', true, 0, '', NOW(), NOW())`,
			d.ID, tenantID, kbID, d.Name, "backup/"+d.ID).Error; err != nil {
			return imported, errs.Wrap(errs.CodeInternal, "insert doc", err)
		}
		imported++
	}

	for _, ch := range exp.Chunks {
		// Re-key chunk ids to avoid collisions with existing data.
		chunkID := uuid.NewString()
		if err := s.db.Exec(`INSERT INTO chunks (id, tenant_id, kb_id, doc_id, idx, content, token_count, page_numbers, vector_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, '', NOW())`,
			chunkID, tenantID, kbID, ch.DocID, ch.Index, ch.Content, ch.PageNumbers).Error; err != nil {
			return imported, errs.Wrap(errs.CodeInternal, "insert chunk", err)
		}
	}
	return imported, nil
}
