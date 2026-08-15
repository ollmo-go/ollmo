package doc

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"

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

// UpdateDocMetadata stores the document metadata JSON ("" clears it).
func (r *Repo) UpdateDocMetadata(tenantID, kbID, id, metadata string) error {
	res := r.db.Model(&Document{}).
		Where("tenant_id = ? AND kb_id = ? AND id = ?", tenantID, kbID, id).
		Update("metadata", metadata)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.NotFound("document not found")
	}
	return nil
}

// IncrementChunkHits atomically bumps hit_num for cited chunks. Called after
// a real chat retrieval; failures are non-fatal and logged by the caller.
func (r *Repo) IncrementChunkHits(tenantID string, chunkIDs []string) error {
	if len(chunkIDs) == 0 {
		return nil
	}
	return r.db.Model(&Chunk{}).
		Where("tenant_id = ? AND id IN ?", tenantID, chunkIDs).
		UpdateColumn("hit_num", gorm.Expr("hit_num + 1")).Error
}

// DocHitStats returns doc_id -> summed chunk hit_num for the given documents,
// powering the hit-count column in the document list.
func (r *Repo) DocHitStats(tenantID string, docIDs []string) (map[string]int64, error) {
	out := make(map[string]int64, len(docIDs))
	if len(docIDs) == 0 {
		return out, nil
	}
	type row struct {
		DocID string
		Hits  int64
	}
	var rows []row
	if err := r.db.Model(&Chunk{}).
		Select("doc_id, COALESCE(SUM(hit_num),0) AS hits").
		Where("tenant_id = ? AND doc_id IN ?", tenantID, docIDs).
		Group("doc_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, rw := range rows {
		out[rw.DocID] = rw.Hits
	}
	return out, nil
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

// SparseSearch runs a MySQL FULLTEXT search over chunk content scoped to one
// KB. Results are ranked by InnoDB's relevance score, a TF-IDF variant
// (TF * IDF^2, no document-length normalization), not BM25. Used as the
// lexical leg of hybrid retrieval; fused with dense Milvus hits via RRF in
// the search service. Parent chunks (role=parent) are excluded — children are
// the searchable units; filters narrow results by document metadata (AND).
func (r *Repo) SparseSearch(ctx context.Context, tenantID, kbID, query string, topK int, filters map[string]string) ([]vector.SearchHit, error) {
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

	condSQL, condArgs := metadataFilterSQL(filters)
	sql := `SELECT c.id, c.doc_id, c.content,
			MATCH(c.content) AGAINST(? IN NATURAL LANGUAGE MODE) AS score
			FROM chunks c
			INNER JOIN documents d ON c.doc_id = d.id AND c.tenant_id = d.tenant_id
			WHERE c.tenant_id = ? AND c.kb_id = ?
			  AND c.role <> ?
			  AND d.enabled = true
			  ` + condSQL + `
			  AND MATCH(c.content) AGAINST(? IN NATURAL LANGUAGE MODE)
			ORDER BY score DESC
			LIMIT ?`
	args := append([]any{query, tenantID, kbID, ChunkRoleParent}, condArgs...)
	args = append(args, query, topK)
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
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

// metadataKeyPattern whitelists metadata keys embedded into JSON path SQL.
// Keys cannot be parameterized, so anything outside this set is rejected by
// the caller before reaching SQL.
const metadataKeyPattern = `^[A-Za-z0-9_-]{1,64}$`

// ValidMetadataKey reports whether a filter key is safe for SQL JSON paths.
func ValidMetadataKey(k string) bool {
	ok, _ := regexp.MatchString(metadataKeyPattern, k)
	return ok
}

// metadataFilterSQL builds "AND JSON_UNQUOTE(JSON_EXTRACT(...)) = ?" clauses
// per filter (AND semantics). Keys must already be validated; values are
// parameterized.
func metadataFilterSQL(filters map[string]string) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	args := make([]any, 0, len(filters))
	for _, k := range keys {
		sb.WriteString(" AND JSON_UNQUOTE(JSON_EXTRACT(d.metadata, '$.")
		sb.WriteString(k)
		sb.WriteString("')) = ?")
		args = append(args, filters[k])
	}
	return sb.String(), args
}

// FindChunkParentIDs returns chunk_id -> parent_id for chunks that belong to
// a parent (parent_child strategy). Used by search to expand hits.
func (r *Repo) FindChunkParentIDs(tenantID string, chunkIDs []string) (map[string]string, error) {
	if len(chunkIDs) == 0 {
		return map[string]string{}, nil
	}
	type row struct {
		ID       string
		ParentID string
	}
	var rows []row
	err := r.db.Model(&Chunk{}).
		Select("id, parent_id").
		Where("tenant_id = ? AND id IN ? AND parent_id <> ''", tenantID, chunkIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.ParentID
	}
	return out, nil
}

// FindChunksByIDs returns chunk rows keyed by ID (parents included).
func (r *Repo) FindChunksByIDs(tenantID string, ids []string) (map[string]*Chunk, error) {
	if len(ids) == 0 {
		return map[string]*Chunk{}, nil
	}
	var rows []*Chunk
	if err := r.db.Where("tenant_id = ? AND id IN ?", tenantID, ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]*Chunk, len(rows))
	for _, c := range rows {
		out[c.ID] = c
	}
	return out, nil
}

// ListDocMeta returns doc_id -> metadata JSON text for docs in a KB that have
// metadata set. Used by the dense leg to filter Milvus hits in Go (Milvus
// rows do not carry document metadata).
func (r *Repo) ListDocMeta(tenantID, kbID string) (map[string]string, error) {
	type row struct {
		ID       string
		Metadata string
	}
	var rows []row
	err := r.db.Model(&Document{}).
		Select("id, metadata").
		Where("tenant_id = ? AND kb_id = ? AND metadata <> ''", tenantID, kbID).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Metadata
	}
	return out, nil
}
