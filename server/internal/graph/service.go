package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/errs"
)

// Service handles entity extraction and graph queries. Extraction runs in the
// worker after embedding; graph queries run in the search service to enrich
// retrieval with entity relationships.
type Service struct {
	repo *Repo
	llm  *clients.LLMClient
}

func NewService(repo *Repo, llm *clients.LLMClient) *Service {
	return &Service{repo: repo, llm: llm}
}

// ExtractFromChunk asks the LLM to extract entities and relations from a chunk
// and persists them. Failures are non-fatal: a bad LLM response is logged and
// skipped so one chunk does not break the whole extraction pass.
func (s *Service) ExtractFromChunk(ctx context.Context, tenantID, kbID, chunkID, content, endpoint, apiKey, model string) error {
	result, err := s.callExtractor(ctx, content, endpoint, apiKey, model)
	if err != nil {
		return err
	}
	if result == nil || (len(result.Entities) == 0 && len(result.Relations) == 0) {
		return nil
	}

	// Persist entities, building a name->id map so relations can resolve.
	entityIDs := make(map[string]string, len(result.Entities))
	for _, e := range result.Entities {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			continue
		}
		ent := &Entity{
			ID:             uuid.NewString(),
			TenantID:       tenantID,
			KbID:           kbID,
			Name:           name,
			Type:           e.Type,
			Description:    e.Description,
			SourceChunkIDs: chunkID,
			MentionCount:   1,
		}
		if err := s.repo.UpsertEntity(ent); err != nil {
			return errs.Wrap(errs.CodeInternal, "upsert entity", err)
		}
		// Re-read to get the canonical id after merge.
		entities, _ := s.repo.FindByNames(tenantID, kbID, []string{name})
		if len(entities) > 0 {
			entityIDs[name] = entities[0].ID
		}
	}

	// Persist relations, resolving source/target names to entity ids.
	// Endpoints are looked up locally first, then against the KB-wide entity
	// table so edges between entities extracted from DIFFERENT chunks are
	// kept; see resolveEntityID. Without the global lookup the graph
	// degenerated into per-chunk stars and cross-chunk knowledge links were
	// silently dropped.
	if err := s.persistRelations(tenantID, kbID, chunkID, result.Relations, entityIDs); err != nil {
		return err
	}
	return nil
}

// resolveEntityID maps a relation endpoint name to an entity id. Local names
// (extracted from the current chunk) win; otherwise the KB-wide entity table
// is consulted (entity extracted from another chunk); as a last resort a stub
// entity is created so the edge survives and future chunks enrich it via
// UpsertEntity merging.
func (s *Service) resolveEntityID(tenantID, kbID, chunkID, name string, local map[string]string) string {
	if id, ok := local[name]; ok {
		return id
	}
	if entities, err := s.repo.FindByNames(tenantID, kbID, []string{name}); err == nil && len(entities) > 0 {
		return entities[0].ID
	}
	if err := s.repo.UpsertEntity(&Entity{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		KbID:           kbID,
		Name:           name,
		SourceChunkIDs: chunkID,
		MentionCount:   1,
	}); err != nil {
		return ""
	}
	entities, err := s.repo.FindByNames(tenantID, kbID, []string{name})
	if err != nil || len(entities) == 0 {
		return ""
	}
	return entities[0].ID
}

// persistRelations creates relation rows for the extraction result, resolving
// endpoint names to entity ids via resolveEntityID.
func (s *Service) persistRelations(tenantID, kbID, chunkID string, relations []ExtractedRelation, local map[string]string) error {
	for _, rel := range relations {
		src := strings.TrimSpace(rel.Source)
		tgt := strings.TrimSpace(rel.Target)
		if src == "" || tgt == "" {
			continue
		}
		srcID := s.resolveEntityID(tenantID, kbID, chunkID, src, local)
		if srcID == "" {
			continue
		}
		tgtID := s.resolveEntityID(tenantID, kbID, chunkID, tgt, local)
		if tgtID == "" {
			continue
		}
		r := &Relation{
			ID:             uuid.NewString(),
			TenantID:       tenantID,
			KbID:           kbID,
			SourceEntityID: srcID,
			TargetEntityID: tgtID,
			RelationType:   rel.Type,
			Description:    rel.Description,
		}
		if err := s.repo.CreateRelation(r); err != nil {
			return errs.Wrap(errs.CodeInternal, "create relation", err)
		}
	}
	return nil
}

// callExtractor sends the extraction prompt to the LLM and parses the JSON
// response. The prompt is strict about JSON-only output to make parsing robust.
func (s *Service) callExtractor(ctx context.Context, content, endpoint, apiKey, model string) (*ExtractionResult, error) {
	prompt := fmt.Sprintf(`Extract entities and relationships from the text below.
Return ONLY valid JSON, no markdown fences, no commentary.
Schema:
{"entities":[{"name":"","type":"","description":""}],"relations":[{"source":"","target":"","type":"","description":""}]}
- name: the entity name (short, canonical form)
- type: one of person, organization, technology, concept, location, event, other
- source/target: entity names as written in "name"
Keep entities to the most important 5-10 per chunk. Skip generic words.

Text:
%s`, truncate(content, 4000))

	req := clients.ChatRequest{
		Model: model,
		Messages: []clients.ChatMessage{
			{Role: "system", Content: "You are an information extraction assistant that outputs only JSON."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0,
		MaxTokens:   1024,
	}
	raw, err := s.llm.Chat(ctx, endpoint, apiKey, req)
	if err != nil {
		return nil, fmt.Errorf("llm extract: %w", err)
	}
	cleaned := stripFences(raw)
	var result ExtractionResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parse extraction json: %w (raw: %s)", err, truncate(raw, 200))
	}
	return &result, nil
}

// QueryForRetrieval finds entities mentioned in the query text and returns
// their descriptions plus related entities. The search service includes this
// as supplementary context alongside vector hits.
func (s *Service) QueryForRetrieval(ctx context.Context, tenantID, kbID, query string, topEntities int) (string, error) {
	if s.repo == nil {
		return "", nil
	}
	// Match entities whose name appears in the query. Simple substring match
	// avoids an extra LLM call per retrieval; an LLM-based entity linker can
	// be added later for fuzzy matches.
	all, _, err := s.repo.ListEntities(tenantID, kbID, 1, 200)
	if err != nil || len(all) == 0 {
		return "", nil
	}
	q := strings.ToLower(query)
	var matched []*Entity
	for _, e := range all {
		if strings.Contains(q, strings.ToLower(e.Name)) {
			matched = append(matched, e)
			if len(matched) >= topEntities {
				break
			}
		}
	}
	if len(matched) == 0 {
		return "", nil
	}
	ids := make([]string, len(matched))
	for i, e := range matched {
		ids[i] = e.ID
	}
	relations, _ := s.repo.FindRelations(tenantID, kbID, ids)

	var b strings.Builder
	b.WriteString("Knowledge graph entities mentioned in the question:\n")
	for _, e := range matched {
		fmt.Fprintf(&b, "- %s (%s): %s\n", e.Name, e.Type, e.Description)
	}
	if len(relations) > 0 {
		b.WriteString("\nRelationships:\n")
		// Build id->name map for readable relation strings.
		names := make(map[string]string)
		for _, e := range all {
			names[e.ID] = e.Name
		}
		for _, r := range relations {
			fmt.Fprintf(&b, "- %s --[%s]--> %s\n", names[r.SourceEntityID], r.RelationType, names[r.TargetEntityID])
		}
	}
	return b.String(), nil
}

func (s *Service) ListEntities(tenantID, kbID string, page, size int) ([]*Entity, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return s.repo.ListEntities(tenantID, kbID, page, size)
}

func (s *Service) DeleteByKB(tenantID, kbID string) error {
	return s.repo.DeleteByKB(tenantID, kbID)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// stripFences removes ```json ... ``` fences that some models wrap around JSON
// despite instructions not to.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
