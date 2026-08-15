package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"ollmo/ollmo/pkg/errs"
)

// defCacheTTL bounds how long a parsed definition is reused without hitting
// the DB. One chat turn reads the same agent row several times (execution
// config extraction + graph walk), so a short TTL removes the duplicate
// load; Save invalidates immediately.
const defCacheTTL = 30 * time.Second

// Service handles agent definition CRUD and config extraction. The chat
// service calls ExtractConfig to get the customized retrieval and generation
// parameters; the canvas calls Get/Save to manage the definition.
type Service struct {
	repo *Repo

	mu   sync.RWMutex
	defs map[string]defEntry
}

type defEntry struct {
	agent   *Agent      // shared read-only snapshot
	def     *Definition // parsed view; nil when the row has no valid definition
	expires time.Time
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo, defs: make(map[string]defEntry)}
}

func cacheKey(tenantID, kbID string) string { return tenantID + "|" + kbID }

// Get returns the agent for a KB. If no agent exists yet, a default one is
// seeded so the canvas always opens with a valid graph.
func (s *Service) Get(ctx context.Context, tenantID, kbID string) (*Agent, error) {
	a, _, err := s.GetDefinition(ctx, tenantID, kbID)
	return a, err
}

// GetDefinition returns the agent row and its parsed definition in one call,
// backed by a per-(tenant, kb) TTL cache. The returned Definition is shared
// and must be treated as read-only; the Agent is a fresh shallow copy.
func (s *Service) GetDefinition(ctx context.Context, tenantID, kbID string) (*Agent, *Definition, error) {
	key := cacheKey(tenantID, kbID)
	s.mu.RLock()
	e, ok := s.defs[key]
	s.mu.RUnlock()
	if ok && time.Now().Before(e.expires) {
		return cloneAgent(e.agent), e.def, nil
	}

	a, err := s.repo.FindByKB(tenantID, kbID)
	if err != nil {
		return nil, nil, errs.Wrap(errs.CodeInternal, "find agent", err)
	}
	if a == nil {
		a, err = s.seedDefault(tenantID, kbID)
		if err != nil {
			return nil, nil, err
		}
	}
	var def *Definition
	if a.Definition != "" {
		var d Definition
		if err := json.Unmarshal([]byte(a.Definition), &d); err == nil && len(d.Nodes) > 0 {
			def = &d
		}
	}
	s.mu.Lock()
	s.defs[key] = defEntry{agent: a, def: def, expires: time.Now().Add(defCacheTTL)}
	s.mu.Unlock()
	return cloneAgent(a), def, nil
}

// cloneAgent returns a shallow copy; Agent holds only scalar fields, so a
// copy is enough to keep callers from mutating the cached row.
func cloneAgent(a *Agent) *Agent {
	c := *a
	return &c
}

// Save replaces the agent definition. A new row is created if none exists;
// otherwise the existing row's definition and version are updated.
func (s *Service) Save(ctx context.Context, tenantID, kbID string, def Definition) (*Agent, error) {
	if err := validate(&def); err != nil {
		return nil, errs.BadRequest(err.Error())
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "marshal definition", err)
	}
	existing, err := s.repo.FindByKB(tenantID, kbID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "find agent", err)
	}
	if existing == nil {
		a := &Agent{
			ID:         uuid.NewString(),
			TenantID:   tenantID,
			KbID:       kbID,
			Name:       "Default Agent",
			Version:    1,
			Definition: string(raw),
			Active:     true,
		}
		if err := s.repo.Create(a); err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "create agent", err)
		}
		s.invalidate(tenantID, kbID)
		return a, nil
	}
	existing.Definition = string(raw)
	existing.Version++
	if err := s.repo.Update(existing); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "update agent", err)
	}
	s.invalidate(tenantID, kbID)
	return existing, nil
}

// invalidate drops the cached definition so the next read sees the new row.
func (s *Service) invalidate(tenantID, kbID string) {
	s.mu.Lock()
	delete(s.defs, cacheKey(tenantID, kbID))
	s.mu.Unlock()
}

// ExtractConfig derives the flat ExecutionConfig from the agent definition.
// It reads opening_message from the definition level, then walks all nodes
// (the graph is small enough that a full scan is fine). Missing nodes fall
// back to defaults. For backward compatibility, opening_message is also
// read from a legacy start node if present.
func ExtractConfig(def *Definition) ExecutionConfig {
	cfg := DefaultExecutionConfig()
	if def == nil || len(def.Nodes) == 0 {
		return cfg
	}

	// Opening message: prefer definition-level, fall back to legacy start node.
	cfg.OpeningMessage = def.OpeningMessage
	if cfg.OpeningMessage == "" {
		for _, n := range def.Nodes {
			if n.Type == NodeInput {
				if v, ok := n.Data["opening_message"]; ok {
					if s, ok := v.(string); ok {
						cfg.OpeningMessage = s
					}
				}
				break
			}
		}
	}

	// Build adjacency list for walk.
	adj := make(map[string][]string, len(def.Nodes))
	for _, e := range def.Edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
	}

	// Find entry point: prefer legacy start node, otherwise use nodes with
	// no incoming edges.
	var queue []string
	hasStart := false
	for _, n := range def.Nodes {
		if n.Type == NodeInput {
			queue = append(queue, n.ID)
			hasStart = true
			break
		}
	}
	if !hasStart {
		hasIncoming := make(map[string]bool, len(def.Nodes))
		for _, e := range def.Edges {
			hasIncoming[e.Target] = true
		}
		for _, n := range def.Nodes {
			if !hasIncoming[n.ID] {
				queue = append(queue, n.ID)
			}
		}
	}
	if len(queue) == 0 {
		// All nodes have incoming edges (cycle); just scan all nodes.
		for _, n := range def.Nodes {
			applyNodeConfig(&cfg, n)
		}
		return cfg
	}

	// Walk the DAG following edges. Visit each node once.
	visited := make(map[string]bool, len(def.Nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true
		node := findNode(def, id)
		if node == nil {
			continue
		}
		applyNodeConfig(&cfg, *node)
		queue = append(queue, adj[id]...)
	}
	return cfg
}

// validate checks the definition has at least one node. Start/end are
// implicit, so they are not required. This prevents saving an empty canvas.
func validate(def *Definition) error {
	if len(def.Nodes) == 0 {
		return fmt.Errorf("definition has no nodes")
	}
	return nil
}

// applyNodeConfig reads a node's Data map and updates the execution config.
// Only fields present in the node data are overridden; absent fields keep
// their defaults.
func applyNodeConfig(cfg *ExecutionConfig, n Node) {
	switch n.Type {
	case NodeInput:
		if v, ok := n.Data["opening_message"]; ok {
			if s, ok := v.(string); ok {
				cfg.OpeningMessage = s
			}
		}
	case NodeRetrieval:
		cfg.UseRetrieval = true
		if v, ok := n.Data["top_k"]; ok {
			if f, ok := toFloat(v); ok && f > 0 {
				cfg.TopK = int(f)
			}
		}
		if v, ok := n.Data["rerank"]; ok {
			if b, ok := v.(bool); ok {
				cfg.Rerank = b
			}
		}
		if v, ok := n.Data["rerank_model_id"]; ok {
			if s, ok := v.(string); ok {
				cfg.RerankModelID = s
			}
		}
		if v, ok := n.Data["use_graph"]; ok {
			if b, ok := v.(bool); ok {
				cfg.UseGraph = b
			}
		}
	case NodeLLM:
		if v, ok := n.Data["llm_model_id"]; ok {
			if s, ok := v.(string); ok && s != "" {
				cfg.LLMModelID = s
			}
		}
		if v, ok := n.Data["system_prompt"]; ok {
			if s, ok := v.(string); ok && s != "" {
				cfg.SystemPrompt = s
			}
		}
		if v, ok := n.Data["temperature"]; ok {
			if f, ok := toFloat(v); ok {
				cfg.Temperature = f
			}
		}
		if v, ok := n.Data["max_tokens"]; ok {
			if f, ok := toFloat(v); ok && f > 0 {
				cfg.MaxTokens = int(f)
			}
		}
		if v, ok := n.Data["top_p"]; ok {
			if f, ok := toFloat(v); ok {
				cfg.TopP = f
			}
		}
		if v, ok := n.Data["reasoning_effort"]; ok {
			if s, ok := v.(string); ok && s != "" {
				cfg.ReasoningEffort = s
			}
		}
	}
}

func findNode(def *Definition, id string) *Node {
	for i := range def.Nodes {
		if def.Nodes[i].ID == id {
			return &def.Nodes[i]
		}
	}
	return nil
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// seedDefault creates a linear agent (input -> retrieval -> llm -> output)
// so the canvas opens with a sensible starting point.
func (s *Service) seedDefault(tenantID, kbID string) (*Agent, error) {
	def := BuildDefaultDefinition()
	raw, err := json.Marshal(def)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "marshal default", err)
	}
	a := &Agent{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		KbID:       kbID,
		Name:       "Default Agent",
		Version:    1,
		Definition: string(raw),
		Active:     true,
	}
	if err := s.repo.Create(a); err != nil {
		// Create may fail on the kb_id unique index if a row already exists.
		// Fall back to fetching the existing row so the canvas still works.
		existing, ferr := s.repo.FindByKBID(kbID)
		if ferr != nil || existing == nil {
			return nil, errs.Wrap(errs.CodeInternal, "create default agent", err)
		}
		return existing, nil
	}
	return a, nil
}

// BuildDefaultDefinition creates a 2-node agent graph: retrieval -> llm
// (matching the frontend "minimal" template). Start/end are implicit.
// OpeningMessage is definition-level. The LLM node gets a default system
// prompt so the agent works out of the box without manual configuration.
func BuildDefaultDefinition() Definition {
	return Definition{
		OpeningMessage: "你好！我是知识库助手，你可以向我提问知识库中的内容。",
		Nodes: []Node{
			{ID: "n1", Type: NodeRetrieval, Position: Position{X: 80, Y: 200},
				Data: map[string]interface{}{"top_k": 10, "rerank": true, "use_graph": true}},
			{ID: "n2", Type: NodeLLM, Position: Position{X: 400, Y: 200},
				Data: map[string]interface{}{"system_prompt": "你是一个友好的助手，请根据上下文回答用户问题。如果上下文没有相关信息，可以与用户自由对话。", "temperature": 0.7, "max_tokens": 2048, "top_p": 0.9}},
		},
		Edges: []Edge{
			{ID: "e1-2", Source: "n1", Target: "n2"},
		},
	}
}

// Standard-template prompts, mirroring the frontend "standard" template so a
// seeded graph behaves identically to one built in the canvas UI.
const (
	standardRAGPrompt    = "你是知识库问答助手。请根据检索到的上下文回答用户的问题 {query}。只基于提供的上下文回答，不要编造信息；如果上下文中没有答案，请如实说明。"
	standardFreePrompt   = "你是一个友好的助手，请根据上下文回答用户问题。如果上下文没有相关信息，可以与用户自由对话。"
	standardFallbackText = "抱歉，未在知识库中找到相关内容，请换个问法或补充更多细节。"
	standardOpeningMsg   = "你好！我是知识库助手，你可以向我提问知识库中的内容。"
	standardCatKB        = "知识库问题"
	standardCatKBDest    = "需要查询知识库文档才能回答的问题，例如文档中涉及的产品、技术或流程"
	standardCatChat      = "闲聊"
	standardCatChatDesc  = "问候、闲聊、通用常识等与知识库文档无关的对话"
	standardBranchTrue   = "满足"
	standardBranchFalse  = "不满足"
)

// BuildStandardDefinition creates the teaching template graph:
// classifier → retrieval → condition → llm/message, plus a free-chat llm
// branch off the classifier. llmModelID/rerankModelID are the tenant's
// freshly seeded defaults; empty strings fall back to tenant defaults at
// execution time, so the graph works without manual wiring.
//
// Node positions are the exact output of the frontend canvas "standard"
// template after dagre auto-layout (rankdir=LR, nodesep=50, ranksep=110,
// 208x76 nodes), so a seeded canvas is stored already tidied up and renders
// identically to one applied in the UI.
func BuildStandardDefinition(llmModelID, rerankModelID string) Definition {
	return Definition{
		OpeningMessage: standardOpeningMsg,
		Nodes: []Node{
			{ID: "n1", Type: NodeClassifier, Position: Position{X: 40, Y: 166},
				Data: map[string]interface{}{
					"slug":         "classifier_1",
					"llm_model_id": llmModelID,
					"categories": []map[string]interface{}{
						{"name": standardCatKB, "description": standardCatKBDest},
						{"name": standardCatChat, "description": standardCatChatDesc},
					},
				}},
			{ID: "n2", Type: NodeRetrieval, Position: Position{X: 358, Y: 103},
				Data: map[string]interface{}{
					"slug": "retrieval_1", "top_k": 10, "rerank": true, "use_graph": true, "rerank_model_id": rerankModelID,
				}},
			{ID: "n3", Type: NodeCondition, Position: Position{X: 676, Y: 103},
				Data: map[string]interface{}{
					"variable": "retrieval_1.top_score", "operator": ">", "value": "0.35",
				}},
			{ID: "n4", Type: NodeLLM, Position: Position{X: 994, Y: 40},
				Data: map[string]interface{}{
					"slug": "llm_1", "llm_model_id": llmModelID, "system_prompt": standardRAGPrompt, "temperature": 0.7, "max_tokens": 2048, "top_p": 0.9,
				}},
			{ID: "n5", Type: NodeMessage, Position: Position{X: 994, Y: 166},
				Data: map[string]interface{}{
					"slug": "message_1", "text": standardFallbackText,
				}},
			{ID: "n6", Type: NodeLLM, Position: Position{X: 358, Y: 229},
				Data: map[string]interface{}{
					"slug": "llm_2", "llm_model_id": llmModelID, "system_prompt": standardFreePrompt, "temperature": 0.7, "max_tokens": 2048, "top_p": 0.9,
				}},
		},
		Edges: []Edge{
			{ID: "e1-2", Source: "n1", Target: "n2", Label: standardCatKB},
			{ID: "e1-6", Source: "n1", Target: "n6", Label: standardCatChat},
			{ID: "e2-3", Source: "n2", Target: "n3"},
			{ID: "e3-4", Source: "n3", Target: "n4", Label: standardBranchTrue},
			{ID: "e3-5", Source: "n3", Target: "n5", Label: standardBranchFalse},
		},
	}
}
