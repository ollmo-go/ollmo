package graph

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newTestSvc(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Entity{}, &Relation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(NewRepo(db), nil)
}

// TestPersistExtraction_CrossChunkEdge verifies the fix for silently dropped
// cross-chunk relations: a relation endpoint not extracted from the current
// chunk resolves to the KB-wide entity created from ANOTHER chunk, so the
// edge connects to the same entity instead of being discarded.
func TestPersistExtraction_CrossChunkEdge(t *testing.T) {
	s := newTestSvc(t)
	const tenantID, kbID = "t1", "k1"

	// Chunk 1 extracted and persisted entity "Zhang San" earlier.
	chunk1Entity := &Entity{
		ID: uuid.NewString(), TenantID: tenantID, KbID: kbID,
		Name: "Zhang San", Type: "person", MentionCount: 1, SourceChunkIDs: "chunk-1",
	}
	// Chunk 2 extracted "Ollmo" as its own entity earlier.
	ollmo := &Entity{
		ID: uuid.NewString(), TenantID: tenantID, KbID: kbID,
		Name: "Ollmo", MentionCount: 1, SourceChunkIDs: "chunk-2",
	}
	if err := s.repo.CreateEntities([]*Entity{chunk1Entity, ollmo}); err != nil {
		t.Fatalf("seed entities: %v", err)
	}

	// Chunk 2 mentions only "Ollmo" locally; its relation references
	// "Zhang San" from chunk 1.
	res := &ExtractionResult{
		Entities:  []ExtractedEntity{{Name: "Ollmo"}},
		Relations: []ExtractedRelation{{Source: "Zhang San", Target: "Ollmo", Type: "founded"}},
	}
	if err := s.persistExtraction(tenantID, kbID, "chunk-2", res); err != nil {
		t.Fatalf("persistExtraction: %v", err)
	}

	relations, err := s.repo.FindRelations(tenantID, kbID, []string{ollmo.ID})
	if err != nil {
		t.Fatalf("findRelations: %v", err)
	}
	if len(relations) != 1 {
		t.Fatalf("got %d relations, want 1 — cross-chunk edge must survive", len(relations))
	}
	// The edge must point at the chunk-1 entity, not a duplicate.
	if relations[0].SourceEntityID != chunk1Entity.ID {
		t.Errorf("edge source %s, want chunk1 entity %s", relations[0].SourceEntityID, chunk1Entity.ID)
	}
}

// TestPersistExtraction_StubEntityForUnknownEndpoint verifies that an endpoint
// unknown anywhere in the KB gets a stub entity so the edge still persists; a
// later extraction for the same name merges into the stub.
func TestPersistExtraction_StubEntityForUnknownEndpoint(t *testing.T) {
	s := newTestSvc(t)
	const tenantID, kbID = "t1", "k1"

	res := &ExtractionResult{
		Entities:  []ExtractedEntity{{Name: "Ollmo"}},
		Relations: []ExtractedRelation{{Source: "Ollmo", Target: "Wang Wu", Type: "uses"}},
	}
	if err := s.persistExtraction(tenantID, kbID, "chunk-1", res); err != nil {
		t.Fatalf("persistExtraction: %v", err)
	}

	entities, err := s.repo.FindByNames(tenantID, kbID, []string{"Wang Wu"})
	if err != nil || len(entities) == 0 {
		t.Fatalf("stub entity not created: %v", err)
	}
	stub := entities[0]
	relations, err := s.repo.FindRelations(tenantID, kbID, []string{stub.ID})
	if err != nil || len(relations) != 1 {
		t.Fatalf("edge to stub missing: relations=%d err=%v", len(relations), err)
	}

	// A later chunk extracting "Wang Wu" with a description merges into the
	// stub instead of creating a duplicate row.
	enrich := &ExtractionResult{
		Entities: []ExtractedEntity{{Name: "Wang Wu", Type: "person", Description: "enriched"}},
	}
	if err := s.persistExtraction(tenantID, kbID, "chunk-9", enrich); err != nil {
		t.Fatalf("enrich: %v", err)
	}
	entities, _ = s.repo.FindByNames(tenantID, kbID, []string{"Wang Wu"})
	if len(entities) != 1 {
		t.Fatalf("got %d entities named Wang Wu, want 1 (merged)", len(entities))
	}
	if entities[0].Description != "enriched" || entities[0].MentionCount != 2 {
		t.Errorf("merge incomplete: desc=%q mentions=%d", entities[0].Description, entities[0].MentionCount)
	}
}

// TestPersistExtraction_SameChunkEdge verifies entities and relations from
// one chunk connect through the newly minted ids without extra lookups.
func TestPersistExtraction_SameChunkEdge(t *testing.T) {
	s := newTestSvc(t)
	const tenantID, kbID = "t1", "k1"
	res := &ExtractionResult{
		Entities:  []ExtractedEntity{{Name: "A"}, {Name: "B"}},
		Relations: []ExtractedRelation{{Source: "A", Target: "B", Type: "related"}},
	}
	if err := s.persistExtraction(tenantID, kbID, "chunk-1", res); err != nil {
		t.Fatalf("persistExtraction: %v", err)
	}
	entities, _ := s.repo.FindByNames(tenantID, kbID, []string{"A", "B"})
	if len(entities) != 2 {
		t.Fatalf("got %d entities, want 2", len(entities))
	}
	byName := map[string]Entity{}
	for _, e := range entities {
		byName[e.Name] = *e
	}
	relations, _ := s.repo.FindRelations(tenantID, kbID, []string{byName["A"].ID})
	if len(relations) != 1 || relations[0].SourceEntityID != byName["A"].ID || relations[0].TargetEntityID != byName["B"].ID {
		t.Fatalf("got %+v, want single A->B edge", relations)
	}
}
