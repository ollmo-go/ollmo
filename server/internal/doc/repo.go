package doc

import (
	"context"
	"errors"

	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/vector"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB exposes the underlying *gorm.DB for transactional use by the service.
func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) CreateDoc(d *Document) error { return r.db.Create(d).Error }

// FindDoc locates a document scoped to tenant AND knowledge base. The kbID
// condition is a security boundary: it prevents cross-KB IDOR where a member
// with access to KB A references a document ID owned by private KB B.
func (r *Repo) FindDoc(tenantID, kbID, id string) (*Document, error) {
	var d Document
	err := r.db.Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, id).First(&d).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("document not found")
		}
		return nil, err
	}
	return &d, nil
}

func (r *Repo) ListDocs(tenantID, kbID string, page, size int) ([]*Document, int64, error) {
	var items []*Document
	var total int64
	q := r.db.Model(&Document{}).Where("tenant_id = ? AND kb_id = ?", tenantID, kbID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error
	return items, total, err
}

// ListDocIDsByKB returns IDs of parsed docs (chunk_count > 0) for a KB.
// Used when re-embedding all docs after the KB's embedding model changes.
func (r *Repo) ListDocIDsByKB(tenantID, kbID string) ([]string, error) {
	var ids []string
	err := r.db.Model(&Document{}).
		Where("tenant_id = ? AND kb_id = ? AND chunk_count > 0", tenantID, kbID).
		Pluck("id", &ids).Error
	return ids, err
}

func (r *Repo) UpdateDocStatus(tenantID, id, status, parseErr string) error {
	return r.db.Model(&Document{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(map[string]any{"status": status, "parse_error": parseErr}).Error
}

func (r *Repo) SetDocEnabled(tenantID, kbID, id string, enabled bool) error {
	res := r.db.Model(&Document{}).
		Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, id).
		Update("enabled", enabled)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("document not found")
	}
	return nil
}

// FindDocNames returns a map of doc_id -> name for the given IDs in one query.
// Used by the search service to batch-resolve doc names instead of querying
// per hit.
func (r *Repo) FindDocNames(tenantID string, docIDs []string) (map[string]string, error) {
	if len(docIDs) == 0 {
		return map[string]string{}, nil
	}
	type row struct {
		ID   string
		Name string
	}
	var rows []row
	if err := r.db.Model(&Document{}).
		Select("id, name").
		Where("tenant_id = ? AND id IN ?", tenantID, docIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

// ListDisabledDocIDs returns IDs of disabled documents in a KB. Used by the
// search service to exclude disabled docs from dense Milvus results.
func (r *Repo) ListDisabledDocIDs(tenantID, kbID string) ([]string, error) {
	var ids []string
	err := r.db.Model(&Document{}).
		Where("tenant_id = ? AND kb_id = ? AND enabled = false", tenantID, kbID).
		Pluck("id", &ids).Error
	return ids, err
}

func (r *Repo) UpdateDocParsed(tenantID, id, parsedObjectKey string, chunkCount int) error {
	return r.db.Model(&Document{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Updates(map[string]any{
			"parsed_object_key": parsedObjectKey,
			"chunk_count":       chunkCount,
		}).Error
}

func (r *Repo) DecDocChunkCount(tenantID, docID string) error {
	return r.db.Model(&Document{}).
		Where("tenant_id = ? AND id = ?", tenantID, docID).
		UpdateColumn("chunk_count", gorm.Expr("chunk_count - 1")).Error
}

func (r *Repo) SetDocVectorIDs(tenantID, docID string, vectorIDs map[string]string) error {
	// vectorIDs maps chunk_id -> milvus_vector_id
	for chunkID, vectorID := range vectorIDs {
		if err := r.db.Model(&Chunk{}).
			Where("tenant_id = ? AND id = ?", tenantID, chunkID).
			Update("vector_id", vectorID).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) DeleteDoc(tenantID, id string) error {
	res := r.db.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&Document{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("document not found")
	}
	return nil
}

func (r *Repo) ListChunksByDoc(tenantID, kbID, docID string, limit, offset int) ([]*Chunk, error) {
	var items []*Chunk
	q := r.db.Where("tenant_id = ? AND kb_id = ? AND doc_id = ?", tenantID, kbID, docID).
		Order("idx ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	err := q.Find(&items).Error
	return items, err
}

func (r *Repo) ListChunksByKB(tenantID, kbID string) ([]*Chunk, error) {
	var items []*Chunk
	err := r.db.Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).
		Order("doc_id ASC, idx ASC").
		Find(&items).Error
	return items, err
}

// FindChunk is scoped to tenant AND knowledge base, mirroring FindDoc's
// cross-KB IDOR protection.
func (r *Repo) FindChunk(tenantID, kbID, chunkID string) (*Chunk, error) {
	var c Chunk
	err := r.db.Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, chunkID).First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("chunk not found")
		}
		return nil, err
	}
	return &c, nil
}

// FindChunkPageNumbers returns a map of chunk_id -> page_numbers for the given
// chunk IDs. Used by the search service to populate page info in SearchHit.
func (r *Repo) FindChunkPageNumbers(tenantID string, chunkIDs []string) (map[string]string, error) {
	if len(chunkIDs) == 0 {
		return map[string]string{}, nil
	}
	type row struct {
		ID          string
		PageNumbers string
	}
	var rows []row
	err := r.db.Model(&Chunk{}).
		Select("id, page_numbers").
		Where("tenant_id = ? AND id IN ?", tenantID, chunkIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.PageNumbers
	}
	return out, nil
}

func (r *Repo) UpdateChunkContent(tenantID, kbID, chunkID, content string) error {
	return r.db.Model(&Chunk{}).
		Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, chunkID).
		Update("content", content).Error
}

func (r *Repo) DeleteChunk(tenantID, kbID, chunkID string) error {
	res := r.db.Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, chunkID).Delete(&Chunk{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("chunk not found")
	}
	return nil
}

// SparseSearch runs a MySQL FULLTEXT search (BM25-like) over chunk content
// scoped to one KB. Results are ranked by MySQL's built-in relevance score.
// Used as the sparse leg of hybrid retrieval; fused with dense Milvus hits
// via RRF in the search service.
func (r *Repo) SparseSearch(ctx context.Context, tenantID, kbID, query string, topK int) ([]vector.SearchHit, error) {
	if topK <= 0 {
		topK = 10
	}
	type sparseRow struct {
		ID      string
		DocID   string
		Content string
		Score   float64
	}
	var rows []sparseRow
	sql := `SELECT c.id, c.doc_id, c.content,
			MATCH(c.content) AGAINST(? IN NATURAL LANGUAGE MODE) AS score
			FROM chunks c
			INNER JOIN documents d ON c.doc_id = d.id AND c.tenant_id = d.tenant_id
			WHERE c.tenant_id = ? AND c.kb_id = ?
			  AND d.enabled = true
			  AND MATCH(c.content) AGAINST(? IN NATURAL LANGUAGE MODE)
			ORDER BY score DESC
			LIMIT ?`
	if err := r.db.WithContext(ctx).Raw(sql, query, tenantID, kbID, query, topK).Scan(&rows).Error; err != nil {
		return nil, err
	}
	hits := make([]vector.SearchHit, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, vector.SearchHit{
			ID:      row.ID,
			DocID:   row.DocID,
			Content: row.Content,
			Score:   float32(row.Score),
		})
	}
	return hits, nil
}
