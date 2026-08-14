package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"ollmo/ollmo/pkg/errs"
)

// KBDefaults carries the KB-level fields that seed a default pipeline. Keeping
// this as a value struct avoids a circular import on the kb package.
type KBDefaults struct {
	KbID           string
	TenantID       string
	EmbeddingModel string
	ChunkStrategy  string
	ChunkSize      int
	ChunkOverlap   int
}

// KBDefaultsFetcher returns the KB-level fields that seed a default pipeline.
// Implemented as a function so the pipeline package stays decoupled from kb.
type KBDefaultsFetcher func(tenantID, kbID string) (KBDefaults, error)

type Service struct {
	repo    *Repo
	fetcher KBDefaultsFetcher
}

func NewService(repo *Repo, fetcher KBDefaultsFetcher) *Service {
	return &Service{repo: repo, fetcher: fetcher}
}

// EnsureForKB creates a default pipeline for a KB if one does not exist yet.
// Called lazily when the canvas first opens a KB that has no pipeline yet.
func (s *Service) EnsureForKB(ctx context.Context, tenantID, kbID string) (*Pipeline, error) {
	if existing, err := s.repo.FindByKB(tenantID, kbID); err == nil {
		return existing, nil
	}
	d, err := s.fetcher(tenantID, kbID)
	if err != nil {
		return nil, err
	}
	def := BuildDefaultDefinition(d)
	raw, err := json.Marshal(def)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "marshal pipeline", err)
	}
	p := &Pipeline{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		KbID:       kbID,
		Version:    1,
		Definition: string(raw),
		Active:     true,
	}
	if err := s.repo.Upsert(p); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "upsert pipeline", err)
	}
	return s.repo.FindByKB(tenantID, kbID)
}

// Get returns the pipeline for a KB, seeding a default if none exists yet.
func (s *Service) Get(ctx context.Context, tenantID, kbID string) (*Pipeline, error) {
	if p, err := s.repo.FindByKB(tenantID, kbID); err == nil {
		return p, nil
	}
	return s.EnsureForKB(ctx, tenantID, kbID)
}

// Save replaces the pipeline definition for a KB and bumps the version.
func (s *Service) Save(ctx context.Context, tenantID, kbID string, def Definition) (*Pipeline, error) {
	if err := Validate(&def); err != nil {
		return nil, errs.BadRequest(err.Error())
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "marshal pipeline", err)
	}
	p := &Pipeline{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		KbID:       kbID,
		Definition: string(raw),
		Active:     true,
	}
	if err := s.repo.Upsert(p); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "upsert pipeline", err)
	}
	return s.repo.FindByKB(tenantID, kbID)
}

// ConfigFetcher returns the typed ingestion config for a KB. The worker uses
// it to drive parsing/chunking/embedding from the canvas-defined pipeline.
type ConfigFetcher func(ctx context.Context, tenantID, kbID string) (*IngestionConfig, error)

// ExtractConfig reads the latest pipeline for a KB and returns the typed
// ingestion config. If no pipeline exists, it returns nil so the caller can
// fall back to KB-level defaults.
func (s *Service) ExtractConfig(ctx context.Context, tenantID, kbID string) (*IngestionConfig, error) {
	p, err := s.repo.FindByKB(tenantID, kbID)
	if err != nil {
		return nil, nil
	}
	var def Definition
	if err := json.Unmarshal([]byte(p.Definition), &def); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "unmarshal pipeline", err)
	}
	return ExtractConfig(&def), nil
}

// DeleteByKB removes the pipeline when a KB is deleted.
func (s *Service) DeleteByKB(ctx context.Context, tenantID, kbID string) error {
	return s.repo.DeleteByKB(tenantID, kbID)
}

// BuildDefaultDefinition constructs the canonical 5-node linear pipeline from KB
// defaults. The canvas opens on this graph for new KBs.
func BuildDefaultDefinition(d KBDefaults) Definition {
	strategy := d.ChunkStrategy
	if strategy == "" {
		strategy = "parent_child"
	}
	size := d.ChunkSize
	if size == 0 {
		size = 500
	}
	overlap := d.ChunkOverlap
	if overlap == 0 {
		overlap = 50
	}
	model := d.EmbeddingModel
	if model == "" {
		model = "bge-large-zh-v1.5"
	}
	return Definition{
		Nodes: []Node{
			{ID: NodeSource, Type: NodeSource, Position: Position{X: 0, Y: 160},
				Data: map[string]interface{}{"label": "File Source"}},
			{ID: NodeParser, Type: NodeParser, Position: Position{X: 260, Y: 160},
				Data: map[string]interface{}{"engine": "auto", "ocr": true, "formula": true, "table": true}},
			{ID: NodeChunker, Type: NodeChunker, Position: Position{X: 520, Y: 160},
				Data: map[string]interface{}{"strategy": strategy, "size": size, "overlap": overlap}},
			{ID: NodeEmbedder, Type: NodeEmbedder, Position: Position{X: 780, Y: 160},
				Data: map[string]interface{}{"model": model, "batch_size": 32}},
			{ID: NodeSink, Type: NodeSink, Position: Position{X: 1040, Y: 160},
				Data: map[string]interface{}{"type": "milvus"}},
		},
		Edges: []Edge{
			{ID: "e1", Source: NodeSource, Target: NodeParser},
			{ID: "e2", Source: NodeParser, Target: NodeChunker},
			{ID: "e3", Source: NodeChunker, Target: NodeEmbedder},
			{ID: "e4", Source: NodeEmbedder, Target: NodeSink},
		},
	}
}

// Validate checks that the definition has the required node types and that the
// linear chain source → parser → chunker → embedder → sink is intact. This
// keeps the canvas free-form for layout while preventing broken pipelines.
func Validate(def *Definition) error {
	if def == nil {
		return fmt.Errorf("definition is required")
	}
	have := map[string]bool{}
	for _, n := range def.Nodes {
		have[n.Type] = true
	}
	for _, want := range []string{NodeSource, NodeParser, NodeChunker, NodeEmbedder, NodeSink} {
		if !have[want] {
			return fmt.Errorf("missing required node: %s", want)
		}
	}
	// Edges must connect the chain in order; extra edges are allowed.
	chain := []struct{ from, to string }{
		{NodeSource, NodeParser},
		{NodeParser, NodeChunker},
		{NodeChunker, NodeEmbedder},
		{NodeEmbedder, NodeSink},
	}
	edges := map[string]bool{}
	for _, e := range def.Edges {
		edges[e.Source+">"+e.Target] = true
	}
	for _, c := range chain {
		if !edges[c.from+">"+c.to] {
			return fmt.Errorf("pipeline must connect %s -> %s", c.from, c.to)
		}
	}
	return nil
}

// ExtractConfig pulls the typed ingestion config out of a Definition. Missing
// fields fall back to the same defaults as BuildDefaultDefinition.
func ExtractConfig(def *Definition) *IngestionConfig {
	cfg := &IngestionConfig{
		Parser:   ParserConfig{Engine: "auto", OCR: true, Formula: true, Table: true},
		Chunker:  ChunkerConfig{Strategy: "parent_child", Size: 500, Overlap: 50},
		Embedder: EmbedderConfig{Model: "bge-large-zh-v1.5", BatchSize: 32},
	}
	for _, n := range def.Nodes {
		switch n.Type {
		case NodeParser:
			if v, ok := n.Data["engine"].(string); ok && v != "" {
				cfg.Parser.Engine = v
			}
			cfg.Parser.OCR = boolVal(n.Data["ocr"], true)
			cfg.Parser.Formula = boolVal(n.Data["formula"], true)
			cfg.Parser.Table = boolVal(n.Data["table"], true)
		case NodeChunker:
			if v, ok := n.Data["strategy"].(string); ok && v != "" {
				cfg.Chunker.Strategy = v
			}
			if v, ok := n.Data["size"].(float64); ok && v > 0 {
				cfg.Chunker.Size = int(v)
			}
			if v, ok := n.Data["overlap"].(float64); ok && v >= 0 {
				cfg.Chunker.Overlap = int(v)
			}
		case NodeEmbedder:
			if v, ok := n.Data["model"].(string); ok && v != "" {
				cfg.Embedder.Model = v
			}
			if v, ok := n.Data["batch_size"].(float64); ok && v > 0 {
				cfg.Embedder.BatchSize = int(v)
			}
		}
	}
	return cfg
}

func boolVal(v interface{}, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}
