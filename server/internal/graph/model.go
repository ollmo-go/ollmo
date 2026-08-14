package graph

import "time"

// Entity is a named thing extracted from a chunk: a person, organization,
// technology, concept, etc. Entities are the nodes of the knowledge graph.
// The same entity name across chunks is deduplicated into one row per KB so
// the graph connects documents through shared entities.
type Entity struct {
	ID          string `gorm:"primaryKey;size:36" json:"id"`
	TenantID    string `gorm:"size:36;index:idx_ent_tenant_kb,priority:1;not null" json:"tenant_id"`
	KbID        string `gorm:"size:36;index:idx_ent_tenant_kb,priority:2;not null" json:"kb_id"`
	Name        string `gorm:"size:256;not null" json:"name"`
	Type        string `gorm:"size:64" json:"type"`
	Description string `gorm:"type:text" json:"description"`
	// SourceChunkIDs is the list of chunk IDs where this entity was mentioned.
	// Stored as JSON so we can trace back to the original text without a join
	// table.
	SourceChunkIDs string    `gorm:"type:text" json:"source_chunk_ids"`
	MentionCount   int       `gorm:"not null;default:1" json:"mention_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Entity) TableName() string { return "graph_entities" }

// Relation is a directed edge between two entities. source_entity_id and
// target_entity_id reference graph_entities.id. The relation type is a short
// verb or label (e.g. "uses", "integrates_with", "part_of").
type Relation struct {
	ID             string    `gorm:"primaryKey;size:36" json:"id"`
	TenantID       string    `gorm:"size:36;index:idx_rel_tenant_kb,priority:1;not null" json:"tenant_id"`
	KbID           string    `gorm:"size:36;index:idx_rel_tenant_kb,priority:2;not null" json:"kb_id"`
	SourceEntityID string    `gorm:"size:36;index;not null" json:"source_entity_id"`
	TargetEntityID string    `gorm:"size:36;index;not null" json:"target_entity_id"`
	RelationType   string    `gorm:"size:64;not null" json:"relation_type"`
	Description    string    `gorm:"type:text" json:"description"`
	CreatedAt      time.Time `json:"created_at"`
}

func (Relation) TableName() string { return "graph_relations" }

// ExtractionResult is the JSON shape the LLM returns when extracting entities
// from a chunk. The extractor parses this and persists Entity/Relation rows.
type ExtractionResult struct {
	Entities  []ExtractedEntity   `json:"entities"`
	Relations []ExtractedRelation `json:"relations"`
}

type ExtractedEntity struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ExtractedRelation struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Type        string `json:"type"`
	Description string `json:"description"`
}
