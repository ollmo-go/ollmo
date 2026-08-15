package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ollmo/ollmo/pkg/clients"
)

// NodeDebugResult is the payload of the single-node debug API. Text carries
// the primary output (LLM reply, classification, condition verdict, message
// text); retrieval additionally reports hits/top_score/context.
type NodeDebugResult struct {
	Type       string  `json:"type"`
	Text       string  `json:"text,omitempty"`
	Hits       int     `json:"hits,omitempty"`
	TopScore   float64 `json:"top_score,omitempty"`
	Context    string  `json:"context,omitempty"`
	DurationMs int64   `json:"duration_ms"`
	// Variables mirrors what the executor would setVar after running this
	// node, keyed exactly as prompts reference them ({slug.output}).
	Variables map[string]string `json:"variables,omitempty"`
}

// DebugNode executes one node in isolation so the canvas can offer "test this
// node" without walking the whole graph. Nothing is persisted. Variable
// references ({slug.output}) resolve against an empty table: upstream outputs
// do not exist in single-node mode, so prompts should be tested end-to-end
// via the test drawer instead.
func DebugNode(ctx context.Context, deps ExecutionDeps, tenantID, kbID string, node Node, query string) (*NodeDebugResult, error) {
	ec := &ExecutionContext{Query: query, Vars: map[string]string{"query": query}}
	start := time.Now()

	switch node.Type {
	case NodeRetrieval:
		if deps.Search == nil {
			return nil, fmt.Errorf("search deps not wired")
		}
		ctxText, hits, top, graphCtx, _, err := deps.Search(ctx, tenantID, kbID, query, nodeInt(node.Data, "top_k", 10), nodeBool(node.Data, "rerank", true), nodeString(node.Data, "rerank_model_id", ""), nodeBool(node.Data, "use_graph", true))
		if err != nil {
			return nil, err
		}
		vars := map[string]string{}
		if slug := nodeString(node.Data, "slug", ""); slug != "" {
			// Same keys execRetrieval sets, so downstream nodes (e.g. a
			// condition on top_score) can be tuned against real values.
			vars[slug+".hit_count"] = strconv.Itoa(hits)
			vars[slug+".top_score"] = strconv.FormatFloat(top, 'f', 3, 64)
			vars[slug+".context"] = truncate(ctxText, 80)
			if graphCtx != "" {
				vars[slug+".graph_context"] = truncate(graphCtx, 80)
			}
		}
		return &NodeDebugResult{Type: node.Type, Hits: hits, TopScore: top, Context: ctxText, Variables: vars, DurationMs: elapsedMs(start)}, nil

	case NodeClassifier:
		cats := nodeCategories(node.Data)
		if len(cats) == 0 {
			return nil, fmt.Errorf("classifier has no categories")
		}
		if deps.ResolveLLM == nil || deps.ChatComplete == nil {
			return nil, fmt.Errorf("llm deps not wired")
		}
		endpoint, model, apiKey, err := deps.ResolveLLM(ctx, tenantID, nodeString(node.Data, "llm_model_id", ""))
		if err != nil {
			return nil, err
		}
		// Same classification prompt as execClassifierWithDetail.
		lines := make([]string, len(cats))
		for i, c := range cats {
			if c.Description != "" {
				lines[i] = c.Name + "：" + c.Description
			} else {
				lines[i] = c.Name
			}
		}
		prompt := fmt.Sprintf(
			"你是一个问题分类器。请将用户的问题分类到以下类别之一，只输出类别名称，不要输出其他内容。\n\n类别：\n%s\n\n用户问题：%s",
			strings.Join(lines, "\n"), query,
		)
		resp, _, err := deps.ChatComplete(ctx, endpoint, apiKey, clients.ChatRequest{Model: model, Messages: []clients.ChatMessage{{Role: "user", Content: prompt}}})
		if err != nil {
			return nil, err
		}
		label := strings.TrimSpace(resp)
		vars := map[string]string{}
		if slug := nodeString(node.Data, "slug", ""); slug != "" && label != "" {
			vars[slug+".label"] = label
		}
		return &NodeDebugResult{Type: node.Type, Text: label, Variables: vars, DurationMs: elapsedMs(start)}, nil

	case NodeCondition:
		variable := nodeString(node.Data, "variable", "hit_count")
		op := nodeString(node.Data, "operator", ">")
		valStr := nodeString(node.Data, "value", "")
		// Left side mirrors execConditionWithDetail, except hit_count and
		// top_score have no upstream retrieval here and stay empty.
		var left string
		switch variable {
		case "hit_count":
			left = "0"
		case "top_score":
			left = "0.000"
		default:
			left = ec.Vars[variable]
		}
		cond := compareValue(left, op, valStr)
		text := fmt.Sprintf("%s %s %s → %v（当前值：%s；检索类变量在单节点调试中为空）", variable, op, valStr, cond, left)
		return &NodeDebugResult{Type: node.Type, Text: text, DurationMs: elapsedMs(start)}, nil

	case NodeLLM:
		if deps.ResolveLLM == nil || deps.ChatComplete == nil {
			return nil, fmt.Errorf("llm deps not wired")
		}
		endpoint, model, apiKey, err := deps.ResolveLLM(ctx, tenantID, nodeString(node.Data, "llm_model_id", ""))
		if err != nil {
			return nil, err
		}
		resp, _, err := deps.ChatComplete(ctx, endpoint, apiKey, clients.ChatRequest{
			Model: model,
			Messages: []clients.ChatMessage{
				{Role: "system", Content: renderVars(nodeString(node.Data, "system_prompt", ""), ec.Vars)},
				{Role: "user", Content: query},
			},
			Temperature: nodeFloat(node.Data, "temperature", 0.7),
			MaxTokens:   nodeInt(node.Data, "max_tokens", 2048),
		})
		if err != nil {
			return nil, err
		}
		out := strings.TrimSpace(resp)
		vars := map[string]string{}
		if slug := nodeString(node.Data, "slug", ""); slug != "" {
			vars[slug+".output"] = truncate(out, 80)
		}
		return &NodeDebugResult{Type: node.Type, Text: out, Variables: vars, DurationMs: elapsedMs(start)}, nil

	case NodeMessage:
		return &NodeDebugResult{Type: node.Type, Text: renderVars(nodeString(node.Data, "text", ""), ec.Vars), DurationMs: elapsedMs(start)}, nil

	default:
		return nil, fmt.Errorf("unsupported node type: %s", node.Type)
	}
}

func elapsedMs(start time.Time) int64 { return time.Since(start).Milliseconds() }

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
