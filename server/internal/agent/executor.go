package agent

import (
	"context"
	"fmt"
	"strings"

	"ollmo/ollmo/pkg/clients"
)

// ExecutionDeps holds the external collaborators the graph executor needs.
// The chat service builds this from its wired services so the agent package
// stays free of search/llm/repo imports.
type ExecutionDeps struct {
	// Search runs KB retrieval. Returns formatted context text, hit count,
	// graph context, and an error.
	Search func(ctx context.Context, tenantID, kbID, query string, topK int, rerank bool, rerankModelID string, useGraph bool) (context string, hitCount int, graphContext string, err error)

	// ResolveLLM returns the LLM provider for a node. modelID empty = tenant
	// default.
	ResolveLLM func(ctx context.Context, tenantID, modelID string) (endpoint, model, apiKey string, err error)

	// ChatComplete does a non-streaming LLM completion. Used by classifier.
	ChatComplete func(ctx context.Context, endpoint, apiKey string, req clients.ChatRequest) (string, error)
}

// ExecutionEvent is emitted by the executor to signal retrieve/citation/trace
// phases back to the caller (which translates them into SSE replies).
type ExecutionEvent struct {
	Phase        string // "retrieve" | "warning" | "trace"
	Citations    []any
	HitCount     int
	GraphContext string
	Warning      string
	Trace        *TraceStep
}

const (
	EvRetrieve = "retrieve"
	EvWarning  = "warning"
	EvTrace    = "trace"
)

// ExecutionContext carries mutable state between nodes during a single walk.
type ExecutionContext struct {
	Query           string
	SystemContext   string
	GraphContext    string
	HitCount        int
	History         []clients.ChatMessage
	MemoryContext   string
	DirectReply     string // set when a message node is the terminal
	DirectCitations []any
}

// TraceStep records one node visit during the graph walk.
type TraceStep struct {
	NodeID   string `json:"node_id"`
	NodeType string `json:"node_type"`
	EdgeID   string `json:"edge_id,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// Terminal describes where the graph walk ended:
//   - NodeLLM: the caller should stream an LLM reply using the returned config.
//   - NodeMessage: the caller should emit ec.DirectReply as a direct reply.
type Terminal struct {
	Type    string
	LLMCfg  *ExecutionConfig
	Warning string
	Trace   []TraceStep
}

// Execute walks the agent graph starting from entry nodes (no incoming edges)
// and emits events via the callback as it processes each node. The walk stops
// at the first terminal node (llm or message). For branching nodes (condition,
// classifier) exactly one outgoing edge is followed based on the branch result.
func Execute(
	ctx context.Context,
	def *Definition,
	deps ExecutionDeps,
	tenantID, kbID string,
	ec *ExecutionContext,
	emit func(ExecutionEvent),
) Terminal {
	if def == nil || len(def.Nodes) == 0 {
		cfg := DefaultExecutionConfig()
		return Terminal{Type: NodeLLM, LLMCfg: &cfg}
	}

	nodeByID := make(map[string]Node, len(def.Nodes))
	for _, n := range def.Nodes {
		nodeByID[n.ID] = n
	}
	out := make(map[string][]Edge, len(def.Nodes))
	hasIncoming := make(map[string]bool, len(def.Nodes))
	for _, e := range def.Edges {
		out[e.Source] = append(out[e.Source], e)
		hasIncoming[e.Target] = true
	}

	// Entry nodes: no incoming edges.
	var queue []string
	for _, n := range def.Nodes {
		if !hasIncoming[n.ID] {
			queue = append(queue, n.ID)
		}
	}
	if len(queue) == 0 {
		for _, n := range def.Nodes {
			if n.Type == NodeClassifier || n.Type == NodeRetrieval {
				queue = append(queue, n.ID)
				break
			}
		}
	}
	if len(queue) == 0 && len(def.Nodes) > 0 {
		queue = []string{def.Nodes[0].ID}
	}

	var trace []TraceStep
	visited := make(map[string]bool, len(def.Nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true
		node, ok := nodeByID[id]
		if !ok {
			continue
		}

		switch node.Type {
		case NodeRetrieval:
			execRetrieval(ctx, deps, tenantID, kbID, node, ec, emit)
			ts := TraceStep{NodeID: id, NodeType: node.Type, Detail: fmt.Sprintf("hits=%d", ec.HitCount)}
			trace = append(trace, ts)
			emit(ExecutionEvent{Phase: EvTrace, Trace: &ts})
			if next, edgeID := firstTargetWithEdge(out[id]); next != "" {
				es := TraceStep{EdgeID: edgeID}
				trace = append(trace, es)
				emit(ExecutionEvent{Phase: EvTrace, Trace: &es})
				queue = append(queue, next)
			}

		case NodeClassifier:
			next, detail := execClassifierWithDetail(ctx, deps, tenantID, node, ec, out[id])
			ts := TraceStep{NodeID: id, NodeType: node.Type, Detail: detail}
			trace = append(trace, ts)
			emit(ExecutionEvent{Phase: EvTrace, Trace: &ts})
			if next != "" {
				if edgeID := findEdgeID(out[id], next); edgeID != "" {
					es := TraceStep{EdgeID: edgeID}
					trace = append(trace, es)
					emit(ExecutionEvent{Phase: EvTrace, Trace: &es})
				}
				queue = append(queue, next)
			}

		case NodeCondition:
			next, detail := execConditionWithDetail(node, ec, out[id])
			ts := TraceStep{NodeID: id, NodeType: node.Type, Detail: detail}
			trace = append(trace, ts)
			emit(ExecutionEvent{Phase: EvTrace, Trace: &ts})
			if next != "" {
				if edgeID := findEdgeID(out[id], next); edgeID != "" {
					es := TraceStep{EdgeID: edgeID}
					trace = append(trace, es)
					emit(ExecutionEvent{Phase: EvTrace, Trace: &es})
				}
				queue = append(queue, next)
			}

		case NodeLLM:
			cfg := ExecutionConfigFromNode(node, ec)
			ts := TraceStep{NodeID: id, NodeType: node.Type}
			trace = append(trace, ts)
			emit(ExecutionEvent{Phase: EvTrace, Trace: &ts})
			return Terminal{Type: NodeLLM, LLMCfg: &cfg, Trace: trace}

		case NodeMessage:
			ec.DirectReply = nodeString(node.Data, "text", "")
			ec.DirectCitations = nil
			ts := TraceStep{NodeID: id, NodeType: node.Type}
			trace = append(trace, ts)
			emit(ExecutionEvent{Phase: EvTrace, Trace: &ts})
			return Terminal{Type: NodeMessage, Trace: trace}

		default:
			if next, edgeID := firstTargetWithEdge(out[id]); next != "" {
				es := TraceStep{EdgeID: edgeID}
				trace = append(trace, es)
				emit(ExecutionEvent{Phase: EvTrace, Trace: &es})
				queue = append(queue, next)
			}
		}
	}

	cfg := DefaultExecutionConfig()
	if ec.SystemContext != "" {
		cfg.UseRetrieval = true
	}
	return Terminal{Type: NodeLLM, LLMCfg: &cfg, Warning: "agent graph did not reach a terminal node; using default reply", Trace: trace}
}

// firstTargetWithEdge returns the target node ID and edge ID of the first edge.
func firstTargetWithEdge(edges []Edge) (target, edgeID string) {
	if len(edges) == 0 {
		return "", ""
	}
	return edges[0].Target, edges[0].ID
}

// findEdgeID returns the edge ID for the edge whose target matches.
func findEdgeID(edges []Edge, target string) string {
	for _, e := range edges {
		if e.Target == target {
			return e.ID
		}
	}
	return ""
}

// firstTarget returns the target of the first edge, or empty if none.
func firstTarget(edges []Edge) string {
	if len(edges) == 0 {
		return ""
	}
	return edges[0].Target
}

// execRetrieval runs KB search and stores results in the execution context.
func execRetrieval(ctx context.Context, deps ExecutionDeps, tenantID, kbID string, node Node, ec *ExecutionContext, emit func(ExecutionEvent)) {
	topK := nodeInt(node.Data, "top_k", 10)
	rerank := nodeBool(node.Data, "rerank", true)
	useGraph := nodeBool(node.Data, "use_graph", true)
	rerankModelID := nodeString(node.Data, "rerank_model_id", "")

	if deps.Search == nil {
		return
	}
	ctxText, hitCount, graphCtx, err := deps.Search(ctx, tenantID, kbID, ec.Query, topK, rerank, rerankModelID, useGraph)
	if err != nil {
		emit(ExecutionEvent{Phase: EvWarning, Warning: "Retrieval failed; answering without context."})
		return
	}
	ec.SystemContext = ctxText
	ec.HitCount = hitCount
	ec.GraphContext = graphCtx
}

// execClassifierWithDetail classifies the query and returns the target node ID
// plus a human-readable detail string for the execution trace.
func execClassifierWithDetail(ctx context.Context, deps ExecutionDeps, tenantID string, node Node, ec *ExecutionContext, edges []Edge) (string, string) {
	cats := nodeStringSlice(node.Data, "categories")
	if len(cats) == 0 || len(edges) == 0 {
		return firstTarget(edges), "no categories"
	}
	if deps.ResolveLLM == nil || deps.ChatComplete == nil {
		return firstTarget(edges), "llm deps not wired"
	}

	modelID := nodeString(node.Data, "llm_model_id", "")
	endpoint, model, apiKey, err := deps.ResolveLLM(ctx, tenantID, modelID)
	if err != nil {
		return firstTarget(edges), "llm resolve failed"
	}

	prompt := fmt.Sprintf(
		"你是一个问题分类器。请将用户的问题分类到以下类别之一，只输出类别名称，不要输出其他内容。\n\n类别：\n%s\n\n用户问题：%s",
		strings.Join(cats, "\n"), ec.Query,
	)
	resp, err := deps.ChatComplete(ctx, endpoint, apiKey, clients.ChatRequest{
		Model: model,
		Messages: []clients.ChatMessage{
			{Role: "system", Content: "You are a question classifier. Reply with only the category name, nothing else."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0,
	})
	if err != nil {
		return firstTarget(edges), "classify error: " + err.Error()
	}
	resp = strings.TrimSpace(resp)
	target := matchEdgeByLabel(edges, resp, cats[0])
	return target, "classified: " + resp
}

// execConditionWithDetail evaluates the condition and returns the target node
// ID plus a human-readable detail for the execution trace.
func execConditionWithDetail(node Node, ec *ExecutionContext, edges []Edge) (string, string) {
	if len(edges) == 0 {
		return "", "no edges"
	}
	variable := nodeString(node.Data, "variable", "hit_count")
	op := nodeString(node.Data, "operator", ">")
	val := nodeInt(node.Data, "value", 0)

	cond := false
	switch variable {
	case "hit_count":
		cond = compareInt(ec.HitCount, op, val)
	}

	// Match true/false branch by edge label.
	for _, e := range edges {
		lbl := strings.ToLower(strings.TrimSpace(e.Label))
		if cond && (lbl == "true" || lbl == "条件成立" || lbl == "是") {
			return e.Target, fmt.Sprintf("%s %s %d = true (%d)", variable, op, val, ec.HitCount)
		}
		if !cond && (lbl == "false" || lbl == "条件不成立" || lbl == "否") {
			return e.Target, fmt.Sprintf("%s %s %d = false (%d)", variable, op, val, ec.HitCount)
		}
	}
	// Positional fallback.
	if cond && len(edges) >= 1 {
		return edges[0].Target, fmt.Sprintf("%s %s %d = true (%d)", variable, op, val, ec.HitCount)
	}
	if !cond && len(edges) >= 2 {
		return edges[1].Target, fmt.Sprintf("%s %s %d = false (%d)", variable, op, val, ec.HitCount)
	}
	return edges[0].Target, fmt.Sprintf("%s %s %d = %v (fallback)", variable, op, val, cond)
}

func compareInt(a int, op string, b int) bool {
	switch op {
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "==":
		return a == b
	case "<":
		return a < b
	case "<=":
		return a <= b
	}
	return false
}

// matchEdgeByLabel finds the edge whose label matches the text
// (case-insensitive). Falls back to fallbackLabel, then the first edge.
func matchEdgeByLabel(edges []Edge, text, fallbackLabel string) string {
	lc := strings.ToLower(strings.TrimSpace(text))
	for _, e := range edges {
		if strings.ToLower(strings.TrimSpace(e.Label)) == lc {
			return e.Target
		}
	}
	// Fuzzy match: check if the response contains a category name.
	for _, e := range edges {
		lbl := strings.ToLower(strings.TrimSpace(e.Label))
		if lbl != "" && (strings.Contains(lc, lbl) || strings.Contains(lbl, lc)) {
			return e.Target
		}
	}
	if fallbackLabel != "" {
		lf := strings.ToLower(fallbackLabel)
		for _, e := range edges {
			if strings.ToLower(strings.TrimSpace(e.Label)) == lf {
				return e.Target
			}
		}
	}
	return edges[0].Target
}

// ExecutionConfigFromNode builds an ExecutionConfig from an llm node's data,
// enriched with retrieval results already gathered in the context.
func ExecutionConfigFromNode(node Node, ec *ExecutionContext) ExecutionConfig {
	cfg := DefaultExecutionConfig()
	cfg.UseRetrieval = ec.SystemContext != ""
	cfg.SystemPrompt = nodeString(node.Data, "system_prompt", "")
	cfg.Temperature = nodeFloat(node.Data, "temperature", 0.7)
	cfg.MaxTokens = nodeInt(node.Data, "max_tokens", 2048)
	cfg.TopP = nodeFloat(node.Data, "top_p", 0.9)
	cfg.LLMModelID = nodeString(node.Data, "llm_model_id", "")
	cfg.ReasoningEffort = nodeString(node.Data, "reasoning_effort", "")
	cfg.RerankModelID = nodeString(node.Data, "rerank_model_id", "")
	return cfg
}

func nodeString(m map[string]interface{}, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func nodeInt(m map[string]interface{}, key string, def int) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

func nodeFloat(m map[string]interface{}, key string, def float64) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		}
	}
	return def
}

func nodeBool(m map[string]interface{}, key string, def bool) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

func nodeStringSlice(m map[string]interface{}, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
