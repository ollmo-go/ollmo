package analytics

import (
	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Overview(tenantID string) (*Overview, error) {
	var o Overview
	if err := r.db.Table("knowledge_bases").
		Where("tenant_id = ?", tenantID).Count(&o.KnowledgeBases).Error; err != nil {
		return nil, err
	}
	if err := r.db.Table("documents").
		Where("tenant_id = ?", tenantID).Count(&o.Documents).Error; err != nil {
		return nil, err
	}
	if err := r.db.Table("chunks").
		Where("tenant_id = ?", tenantID).Count(&o.Chunks).Error; err != nil {
		return nil, err
	}
	if err := r.db.Table("conversations").
		Where("tenant_id = ?", tenantID).Count(&o.Conversations).Error; err != nil {
		return nil, err
	}
	if err := r.db.Table("messages").
		Where("tenant_id = ?", tenantID).Count(&o.Messages).Error; err != nil {
		return nil, err
	}
	if err := r.db.Table("documents").
		Where("tenant_id = ?", tenantID).
		Select("COALESCE(SUM(size),0)").Scan(&o.StorageBytes).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repo) DocStats(tenantID string) (*DocStats, error) {
	var s DocStats
	if err := r.db.Table("documents").
		Where("tenant_id = ?", tenantID).Count(&s.Total).Error; err != nil {
		return nil, err
	}
	if err := r.db.Table("documents").
		Select("status, COUNT(*) as count").
		Where("tenant_id = ?", tenantID).
		Group("status").
		Scan(&s.ByStatus).Error; err != nil {
		return nil, err
	}
	if err := r.db.Table("chunks").
		Where("tenant_id = ?", tenantID).Count(&s.TotalChunks).Error; err != nil {
		return nil, err
	}
	ready := int64(0)
	for _, c := range s.ByStatus {
		if c.Status == "ready" {
			ready = c.Count
		}
	}
	if s.Total > 0 {
		s.SuccessRate = float64(ready) / float64(s.Total)
	}
	return &s, nil
}

func (r *Repo) KBUsage(tenantID string) ([]KBUsage, error) {
	var items []KBUsage
	err := r.db.Table("knowledge_bases AS k").
		Select(`k.id AS kb_id, k.name AS kb_name,
			COALESCE(d.cnt, 0) AS doc_count,
			COALESCE(c.cnt, 0) AS chunk_count,
			COALESCE(d.bytes, 0) AS storage_bytes,
			COALESCE(cv.cnt, 0) AS conversations`).
		Joins("LEFT JOIN (SELECT kb_id, COUNT(*) cnt, SUM(size) bytes FROM documents WHERE tenant_id = ? GROUP BY kb_id) d ON d.kb_id = k.id", tenantID).
		Joins("LEFT JOIN (SELECT kb_id, COUNT(*) cnt FROM chunks WHERE tenant_id = ? GROUP BY kb_id) c ON c.kb_id = k.id", tenantID).
		Joins("LEFT JOIN (SELECT kb_id, COUNT(*) cnt FROM conversations WHERE tenant_id = ? GROUP BY kb_id) cv ON cv.kb_id = k.id", tenantID).
		Where("k.tenant_id = ?", tenantID).
		Order("doc_count DESC").
		Scan(&items).Error
	return items, err
}

func (r *Repo) RecentActivity(tenantID string, limit int) ([]ActivityItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var docs []ActivityItem
	if err := r.db.Table("documents").
		Select("id, name, status, kb_id, created_at").
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Scan(&docs).Error; err != nil {
		return nil, err
	}
	for i := range docs {
		docs[i].Kind = "document"
	}
	var convs []ActivityItem
	if err := r.db.Table("conversations").
		Select("id, title AS name, kb_id, created_at").
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Scan(&convs).Error; err != nil {
		return nil, err
	}
	for i := range convs {
		convs[i].Kind = "chat"
	}
	merged := append(docs, convs...)
	return merged, nil
}
