package graph

import (
	"errors"

	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// UpsertEntity inserts a new entity or merges into an existing one with the
// same (kb_id, name). When merging, the mention count is bumped and the new
// chunk ID is appended to source_chunk_ids (deduplicated).
func (r *Repo) UpsertEntity(e *Entity) error {
	var existing Entity
	err := r.db.Where("tenant_id = ? AND kb_id = ? AND name = ?", e.TenantID, e.KbID, e.Name).First(&existing).Error
	if err == nil {
		existing.MentionCount += e.MentionCount
		if e.Description != "" && existing.Description == "" {
			existing.Description = e.Description
		}
		if e.Type != "" && existing.Type == "" {
			existing.Type = e.Type
		}
		existing.SourceChunkIDs = mergeChunkIDs(existing.SourceChunkIDs, e.SourceChunkIDs)
		return r.db.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.Create(e).Error
}

func (r *Repo) CreateRelation(rel *Relation) error {
	return r.db.Create(rel).Error
}

// FindByNames returns entities matching any of the given names (case-insensitive)
// within a KB. Used by graph retrieval to resolve entities mentioned in a query.
func (r *Repo) FindByNames(tenantID, kbID string, names []string) ([]*Entity, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var items []*Entity
	err := r.db.Where("tenant_id = ? AND kb_id = ? AND name IN ?", tenantID, kbID, names).
		Order("mention_count DESC").
		Find(&items).Error
	return items, err
}

// FindRelations returns relations connected to any of the given entity IDs.
func (r *Repo) FindRelations(tenantID, kbID string, entityIDs []string) ([]*Relation, error) {
	if len(entityIDs) == 0 {
		return nil, nil
	}
	var items []*Relation
	err := r.db.Where("tenant_id = ? AND kb_id = ? AND (source_entity_id IN ? OR target_entity_id IN ?)",
		tenantID, kbID, entityIDs, entityIDs).
		Find(&items).Error
	return items, err
}

// ListEntities returns all entities in a KB, paginated.
func (r *Repo) ListEntities(tenantID, kbID string, page, size int) ([]*Entity, int64, error) {
	var items []*Entity
	var total int64
	q := r.db.Model(&Entity{}).Where("tenant_id = ? AND kb_id = ?", tenantID, kbID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("mention_count DESC, created_at DESC").
		Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

// DeleteByKB removes all entities and relations for a KB. Called when a KB is
// deleted or when re-extracting entities for a document.
func (r *Repo) DeleteByKB(tenantID, kbID string) error {
	if err := r.db.Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).Delete(&Relation{}).Error; err != nil {
		return err
	}
	return r.db.Where("tenant_id = ? AND kb_id = ?", tenantID, kbID).Delete(&Entity{}).Error
}

func mergeChunkIDs(existing, add string) string {
	if add == "" {
		return existing
	}
	set := map[string]bool{}
	if existing != "" {
		for _, id := range splitCSV(existing) {
			set[id] = true
		}
	}
	for _, id := range splitCSV(add) {
		if id != "" {
			set[id] = true
		}
	}
	out := ""
	for id := range set {
		if out != "" {
			out += ","
		}
		out += id
	}
	return out
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	cur := ""
	for _, c := range s {
		if c == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
