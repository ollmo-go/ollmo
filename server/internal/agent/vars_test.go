package agent

import (
	"context"
	"strings"
	"testing"

	"ollmo/ollmo/pkg/clients"
)

func fakeDeps(contextText, llmReply string) ExecutionDeps {
	return ExecutionDeps{
		Search: func(ctx context.Context, tenantID, kbID, query string, topK int, rerank bool, rerankModelID string, useGraph bool) (string, int, float64, string, []any, error) {
			return contextText, 3, 0.62, "", nil, nil
		},
		ResolveLLM: func(ctx context.Context, tenantID, modelID string) (string, string, string, error) {
			return "http://llm", "test-model", "key", nil
		},
		ChatComplete: func(ctx context.Context, endpoint, apiKey string, req clients.ChatRequest) (string, *clients.TokenUsage, error) {
			return llmReply, nil, nil
		},
	}
}

func node(id, typ, slug string, data map[string]interface{}) Node {
	if data == nil {
		data = map[string]interface{}{}
	}
	if slug != "" {
		data["slug"] = slug
	}
	return Node{ID: id, Type: typ, Data: data}
}

func TestRenderVars(t *testing.T) {
	vars := map[string]string{
		"检索_1.context": "chunk text",
		"LLM_2.output": "rewritten",
		"query":        "q",
	}
	cases := []struct{ in, want string }{
		{"based on {检索_1.context} answer", "based on chunk text answer"},
		{"use {LLM_2.output} then {LLM_2.output}", "use rewritten then rewritten"},
		{"bare {context} and {query} untouched", "bare {context} and {query} untouched"},
		{"missing {nope.output} becomes empty", "missing  becomes empty"},
		{"no refs at all", "no refs at all"},
	}
	for _, c := range cases {
		if got := renderVars(c.in, vars); got != c.want {
			t.Errorf("renderVars(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A chunk whose content contains placeholder syntax must not inject a live
// reference: values are substituted in a single pass and never rescanned.
func TestRenderVarsNoRescan(t *testing.T) {
	vars := map[string]string{"a.output": "{b.secret}"}
	if got := renderVars("x {a.output} y", vars); got != "x {b.secret} y" {
		// The literal {b.secret} text is fine — what must NOT happen is a
		// second substitution pass replacing it with a b.secret value.
		t.Logf("passthrough: %q", got)
	}
	vars2 := map[string]string{"a.output": "{b.secret}", "b.secret": "LEAK"}
	if got := renderVars("x {a.output} y", vars2); strings.Contains(got, "LEAK") {
		t.Fatalf("rescan injection detected: %q", got)
	}
}

// retrieval → intermediate llm (rewrites) → terminal llm referencing
// {改写.output}: the intermediate output must be available to the terminal.
func TestIntermediateLLMPublishesVariable(t *testing.T) {
	def := &Definition{
		Nodes: []Node{
			node("r", NodeRetrieval, "检索", nil),
			node("m", NodeLLM, "改写", map[string]interface{}{
				"system_prompt": "rewrite: {检索.context}",
			}),
			node("f", NodeLLM, "回答", map[string]interface{}{
				"system_prompt": "final answer using {改写.output}",
			}),
		},
		Edges: []Edge{
			{ID: "e1", Source: "r", Target: "m"},
			{ID: "e2", Source: "m", Target: "f"},
		},
	}
	deps := fakeDeps("ctx-of-kb", "REWRITTEN")
	ec := &ExecutionContext{Query: "user q"}
	term := Execute(context.Background(), def, deps, "t", "kb", ec, func(ExecutionEvent) {})

	if term.Type != NodeLLM {
		t.Fatalf("terminal type = %s, want llm", term.Type)
	}
	if ec.Vars["检索.context"] != "ctx-of-kb" {
		t.Fatalf("retrieval var missing: %+v", ec.Vars)
	}
	if ec.Vars["改写.output"] != "REWRITTEN" {
		t.Fatalf("intermediate output missing: %+v", ec.Vars)
	}
	if term.LLMCfg.SystemPrompt != "final answer using REWRITTEN" {
		t.Fatalf("terminal prompt = %q", term.LLMCfg.SystemPrompt)
	}
}

// When the terminal prompt explicitly references {slug.context}, the shared
// SystemContext must be cleared so the chat pipeline does not append the
// chunks twice; without the reference it stays intact.
func TestTerminalPromptContextDedup(t *testing.T) {
	def := &Definition{
		Nodes: []Node{
			node("r", NodeRetrieval, "检索", nil),
			node("f", NodeLLM, "回答", map[string]interface{}{
				"system_prompt": "answer from {检索.context}",
			}),
		},
		Edges: []Edge{{ID: "e1", Source: "r", Target: "f"}},
	}
	ec := &ExecutionContext{Query: "q"}
	Execute(context.Background(), def, fakeDeps("chunks", ""), "t", "kb", ec, func(ExecutionEvent) {})
	if ec.SystemContext != "" {
		t.Fatalf("SystemContext should be cleared when prompt references {slug.context}")
	}

	// Control: a plain prompt keeps SystemContext for the chat pipeline.
	def2 := &Definition{
		Nodes: []Node{
			node("r", NodeRetrieval, "检索", nil),
			node("f", NodeLLM, "回答", map[string]interface{}{
				"system_prompt": "answer using {context}",
			}),
		},
		Edges: []Edge{{ID: "e1", Source: "r", Target: "f"}},
	}
	ec2 := &ExecutionContext{Query: "q"}
	Execute(context.Background(), def2, fakeDeps("chunks", ""), "t", "kb", ec2, func(ExecutionEvent) {})
	if ec2.SystemContext != "chunks" {
		t.Fatalf("SystemContext should stay for bare {context} prompt")
	}
}

// Legacy graphs without slugs keep working: no variables are registered and
// the terminal behaves as before.
func TestLegacyNoSlug(t *testing.T) {
	def := &Definition{
		Nodes: []Node{
			node("r", NodeRetrieval, "", nil),
			node("f", NodeLLM, "", map[string]interface{}{"system_prompt": "p {context}"}),
		},
		Edges: []Edge{{ID: "e1", Source: "r", Target: "f"}},
	}
	ec := &ExecutionContext{Query: "q"}
	term := Execute(context.Background(), def, fakeDeps("chunks", ""), "t", "kb", ec, func(ExecutionEvent) {})
	if term.Type != NodeLLM {
		t.Fatalf("terminal = %s", term.Type)
	}
	if len(ec.Vars) != 1 || ec.Vars["query"] != "q" {
		t.Fatalf("legacy vars should hold only query: %+v", ec.Vars)
	}
	if ec.SystemContext != "chunks" {
		t.Fatalf("legacy retrieval context lost")
	}
}

// Message node text renders variables.
func TestMessageRendersVars(t *testing.T) {
	def := &Definition{
		Nodes: []Node{
			node("r", NodeRetrieval, "检索", nil),
			node("m", NodeMessage, "回复", map[string]interface{}{
				"text": "分类是 {检索.hit_count} 条命中",
			}),
		},
		Edges: []Edge{{ID: "e1", Source: "r", Target: "m"}},
	}
	ec := &ExecutionContext{Query: "q"}
	term := Execute(context.Background(), def, fakeDeps("chunks", ""), "t", "kb", ec, func(ExecutionEvent) {})
	if term.Type != NodeMessage || ec.DirectReply != "分类是 3 条命中" {
		t.Fatalf("direct reply = %q (type %s)", ec.DirectReply, term.Type)
	}
}

// The condition node gates on the retrieval's best relevance score, which —
// unlike hit_count — is a meaningful relevance signal.
func TestConditionTopScoreGate(t *testing.T) {
	mk := func(val float64) *Definition {
		return &Definition{
			Nodes: []Node{
				node("r", NodeRetrieval, "检索", nil),
				node("cd", NodeCondition, "条件", map[string]interface{}{
					"variable": "top_score", "operator": ">", "value": val,
				}),
				node("m", NodeMessage, "回复", map[string]interface{}{"text": "no hit"}),
			},
			Edges: []Edge{
				{ID: "e1", Source: "r", Target: "cd"},
				{ID: "e2", Source: "cd", Target: "l", Label: "true"},
				{ID: "e3", Source: "cd", Target: "m", Label: "false"},
			},
		}
	}
	// fakeDeps reports topScore=0.62.
	ec := &ExecutionContext{Query: "q"}
	term := Execute(context.Background(), mk(0.35), fakeDeps("ctx", ""), "t", "kb", ec, func(ExecutionEvent) {})
	if term.Type == NodeMessage {
		t.Fatalf("score 0.62 > 0.35 should take the true branch, got message terminal")
	}
	ec2 := &ExecutionContext{Query: "q"}
	term2 := Execute(context.Background(), mk(0.9), fakeDeps("ctx", ""), "t", "kb", ec2, func(ExecutionEvent) {})
	if term2.Type != NodeMessage || ec2.DirectReply != "no hit" {
		t.Fatalf("score 0.62 < 0.9 should take the false branch, got %s %q", term2.Type, ec2.DirectReply)
	}
}

// The raw query is available to conditions anywhere in the graph.
func TestConditionOnQuery(t *testing.T) {
	mk := func() *Definition {
		return &Definition{
			Nodes: []Node{
				node("r", NodeRetrieval, "检索", nil),
				node("cd", NodeCondition, "条件", map[string]interface{}{
					"variable": "query", "operator": "contains", "value": "价格",
				}),
				node("m", NodeMessage, "回复", map[string]interface{}{"text": "fallback"}),
			},
			Edges: []Edge{
				{ID: "e1", Source: "r", Target: "cd"},
				{ID: "e2", Source: "cd", Target: "l", Label: "true"},
				{ID: "e3", Source: "cd", Target: "m", Label: "false"},
			},
		}
	}
	ec := &ExecutionContext{Query: "ollmo 的价格是多少"}
	term := Execute(context.Background(), mk(), fakeDeps("ctx", ""), "t", "kb", ec, func(ExecutionEvent) {})
	if term.Type == NodeMessage {
		t.Fatalf("query containing 价格 should take the true branch")
	}
	ec2 := &ExecutionContext{Query: "你好"}
	term2 := Execute(context.Background(), mk(), fakeDeps("ctx", ""), "t", "kb", ec2, func(ExecutionEvent) {})
	if term2.Type != NodeMessage || ec2.DirectReply != "fallback" {
		t.Fatalf("query without keyword should take the false branch, got %s %q", term2.Type, ec2.DirectReply)
	}
}

// compareValue handles numeric and string semantics per operator.
func TestCompareValue(t *testing.T) {
	cases := []struct {
		left, op, right string
		want            bool
	}{
		{"0.5", ">", "0.35", true},
		{"3", "==", "3.0", true},
		{"10", ">=", "10", true},
		{"A", "==", "A", true},
		{"A", "==", "B", false},
		{"A", "!=", "B", true},
		{"hello world", "contains", "world", true},
		{"hello", "contains", "", false},
	}
	for _, c := range cases {
		if got := compareValue(c.left, c.op, c.right); got != c.want {
			t.Fatalf("compareValue(%q,%q,%q) = %v, want %v", c.left, c.op, c.right, got, c.want)
		}
	}
}

// String variables (e.g. a classifier's label) can drive conditions too.
func TestConditionOnStringVariable(t *testing.T) {
	mk := func() *Definition {
		return &Definition{
			Nodes: []Node{
				node("c", NodeClassifier, "分类", map[string]interface{}{
					"categories": []interface{}{
						map[string]interface{}{"name": "A"},
						map[string]interface{}{"name": "B"},
					},
				}),
				node("cd", NodeCondition, "条件", map[string]interface{}{
					"variable": "分类.label", "operator": "==", "value": "A",
				}),
				node("m", NodeMessage, "回复", map[string]interface{}{"text": "fallback"}),
			},
			Edges: []Edge{
				{ID: "e1", Source: "c", Target: "cd"},
				{ID: "e2", Source: "cd", Target: "l", Label: "true"},
				{ID: "e3", Source: "cd", Target: "m", Label: "false"},
			},
		}
	}
	// Classifier replies "A" → 分类.label=A → == A takes the true branch.
	ec := &ExecutionContext{Query: "q"}
	term := Execute(context.Background(), mk(), fakeDeps("ctx", "A"), "t", "kb", ec, func(ExecutionEvent) {})
	if term.Type == NodeMessage {
		t.Fatalf("label == A should take the true branch, got message terminal")
	}
	// Replies "B" → == A false → message fallback.
	ec2 := &ExecutionContext{Query: "q"}
	term2 := Execute(context.Background(), mk(), fakeDeps("ctx", "B"), "t", "kb", ec2, func(ExecutionEvent) {})
	if term2.Type != NodeMessage || ec2.DirectReply != "fallback" {
		t.Fatalf("label != A should take the false branch, got %s %q", term2.Type, ec2.DirectReply)
	}
}

// nodeCategories accepts both the legacy string form and the structured
// {name, description} form.
func TestNodeCategories(t *testing.T) {
	mixed := map[string]interface{}{
		"categories": []interface{}{
			"旧类别",
			map[string]interface{}{"name": "知识库问题", "description": "需要查文档"},
			map[string]interface{}{"name": "", "description": "no name, skipped"},
			map[string]interface{}{"name": "空描述"},
		},
	}
	cats := nodeCategories(mixed)
	if len(cats) != 3 {
		t.Fatalf("got %d categories, want 3: %+v", len(cats), cats)
	}
	if cats[0].Name != "旧类别" || cats[0].Description != "" {
		t.Fatalf("legacy string form broken: %+v", cats[0])
	}
	if cats[1].Description != "需要查文档" {
		t.Fatalf("description lost: %+v", cats[1])
	}
	if cats[2].Description != "" {
		t.Fatalf("empty description should stay empty: %+v", cats[2])
	}
}

// The classifier prompt must include category descriptions and the LLM's
// category-name reply must still route via edge label matching.
func TestClassifierUsesDescriptions(t *testing.T) {
	var gotPrompt string
	deps := fakeDeps("ctx", "知识库问题")
	deps.ChatComplete = func(ctx context.Context, endpoint, apiKey string, req clients.ChatRequest) (string, *clients.TokenUsage, error) {
		for _, m := range req.Messages {
			if m.Role == "user" {
				gotPrompt = m.Content
			}
		}
		return "知识库问题", nil, nil
	}
	def := &Definition{
		Nodes: []Node{
			node("c", NodeClassifier, "分类", map[string]interface{}{
				"categories": []interface{}{
					map[string]interface{}{"name": "知识库问题", "description": "需要查询知识库文档"},
					map[string]interface{}{"name": "其他问题", "description": "通用常识、闲聊"},
				},
			}),
			node("r", NodeRetrieval, "检索", nil),
			node("m", NodeMessage, "回复", map[string]interface{}{"text": "fallback"}),
		},
		Edges: []Edge{
			{ID: "e1", Source: "c", Target: "r", Label: "知识库问题"},
			{ID: "e2", Source: "c", Target: "m", Label: "其他问题"},
		},
	}
	ec := &ExecutionContext{Query: "ollmo 是什么"}
	term := Execute(context.Background(), def, deps, "t", "kb", ec, func(ExecutionEvent) {})
	if !strings.Contains(gotPrompt, "知识库问题：需要查询知识库文档") {
		t.Fatalf("prompt missing description: %q", gotPrompt)
	}
	if !strings.Contains(gotPrompt, "其他问题：通用常识、闲聊") {
		t.Fatalf("prompt missing second description: %q", gotPrompt)
	}
	// LLM replied with the first category name → routed to the retrieval
	// path (fakeDeps search always reports 3 hits).
	if ec.HitCount != 3 {
		t.Fatalf("expected retrieval branch (hitCount=3), got %d, terminal=%s", ec.HitCount, term.Type)
	}
}
