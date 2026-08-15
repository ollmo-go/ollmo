package chat

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"ollmo/ollmo/internal/agent"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/internal/search"
	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/errs"
)

// Context-budget knobs. The prompt must fit the provider's window before it
// is sent; otherwise the request fails provider-side with an opaque error.
const (
	// defaultContextLength is assumed when a provider has no ContextLength
	// configured; conservative for current mainstream models.
	defaultContextLength = 32768
	// promptSafetyMargin absorbs estimator error plus message framing.
	promptSafetyMargin = 512
	// minPromptBudget guards against degenerate window/reserve combos.
	minPromptBudget = 512
)

// promptBudget converts the provider's window into an estimated-token budget
// for prompt assembly: window minus the output reserve (MaxTokens) minus a
// safety margin. buildPrompt trims graph/memory/history to fit this budget.
func promptBudget(p *llm.LLMModel) int {
	window := p.ContextLength
	if window <= 0 {
		window = defaultContextLength
	}
	b := window - p.MaxTokens - promptSafetyMargin
	if b < minPromptBudget {
		b = minPromptBudget
	}
	return b
}

// graphDeps builds the ExecutionDeps for the graph executor from the chat
// service's wired collaborators. Called once per stream; the closures capture
// tenantID/kbID/query from the surrounding scope.
func (s *Service) graphDeps(tenantID, kbID, query string) agent.ExecutionDeps {
	return agent.ExecutionDeps{
		Search: func(ctx context.Context, tid, kid, q string, topK int, rerank bool, rerankModelID string, useGraph bool) (string, int, float64, string, []any, error) {
			r, err := s.searchSvc.Search(ctx, tid, kid, search.SearchRequest{
				Query: q, TopK: topK, Rerank: &rerank, RerankModelID: rerankModelID,
			})
			if err != nil {
				return "", 0, 0, "", nil, err
			}
			ctxText, cits := formatContext(r.Hits)
			graphCtx := r.GraphContext
			if !useGraph {
				graphCtx = ""
			}
			var top float64
			if len(r.Hits) > 0 {
				top = r.Hits[0].Score
			}
			return ctxText, len(r.Hits), top, graphCtx, citationsToAny(cits), nil
		},
		ResolveLLM: func(ctx context.Context, tid, modelID string) (string, string, string, error) {
			p, err := s.resolveProvider(ctx, tid, modelID)
			if err != nil {
				return "", "", "", err
			}
			return p.Endpoint, p.Model, p.APIKey, nil
		},
		ChatComplete: s.llm.Chat,
	}
}

// DebugAgentNode runs one agent node in isolation for the canvas "test this
// node" action. Nothing is persisted.
func (s *Service) DebugAgentNode(ctx context.Context, tenantID, kbID string, node agent.Node, query string) (*agent.NodeDebugResult, error) {
	deps := s.graphDeps(tenantID, kbID, query)
	return agent.DebugNode(ctx, deps, tenantID, kbID, node, query)
}

// send delivers a reply to the client channel, respecting ctx cancellation.
// Returns false when ctx is cancelled (client disconnected) so the caller can
// stop streaming and persist partial results instead of blocking forever on a
// full channel buffer.
func send(ctx context.Context, out chan<- StreamReply, r StreamReply) bool {
	select {
	case out <- r:
		return true
	case <-ctx.Done():
		return false
	}
}

// streamLLM is the shared LLM streaming core used by runStream (foreground
// chat) and testStream (agent test drawer). It applies the agent-config
// parameter overrides, builds the ChatRequest, runs the delta loop emitting
// thinking/generate phases, and returns the accumulated content/reasoning,
// token usage, generate duration, and termination status:
//   - streamErr != "": the LLM stream reported an error (already sent as
//     PhaseError); content/reasoning hold whatever was produced before it.
//   - cancelled: the client disconnected mid-stream (send returned false).
//   - otherwise: the stream completed normally.
//
// This function performs no DB writes; callers own persistence.
func (s *Service) streamLLM(
	ctx context.Context,
	out chan<- StreamReply,
	provider *llm.LLMModel,
	cfg agent.ExecutionConfig,
	msgs []clients.ChatMessage,
) (content, reasoning string, usage *clients.TokenUsage, generateMs int, streamErr string, cancelled bool) {
	// LLM parameters: agent config overrides provider defaults so the canvas
	// can tune temperature/max_tokens without editing the provider.
	temp := provider.Temperature
	if cfg.Temperature > 0 {
		temp = cfg.Temperature
	}
	maxTok := provider.MaxTokens
	if cfg.MaxTokens > 0 {
		maxTok = cfg.MaxTokens
	}
	topP := provider.TopP
	if cfg.TopP > 0 {
		topP = cfg.TopP
	}
	req := clients.ChatRequest{
		Model:           provider.Model,
		Messages:        msgs,
		Temperature:     temp,
		MaxTokens:       maxTok,
		TopP:            topP,
		ReasoningEffort: cfg.ReasoningEffort,
		Stream:          true,
	}

	var sb, rb strings.Builder
	generateStart := time.Now()
	for delta := range s.llm.ChatStream(ctx, provider.Endpoint, provider.APIKey, req) {
		if delta.Err != nil {
			send(ctx, out, StreamReply{Phase: PhaseError, Error: delta.Err.Error()})
			return sb.String(), rb.String(), usage, int(time.Since(generateStart).Milliseconds()), delta.Err.Error(), false
		}
		if delta.Done {
			break
		}
		if delta.Usage != nil {
			usage = delta.Usage
		}
		if delta.Reasoning != "" {
			rb.WriteString(delta.Reasoning)
			if !send(ctx, out, StreamReply{Phase: PhaseThinking, Token: delta.Reasoning}) {
				return sb.String(), rb.String(), usage, int(time.Since(generateStart).Milliseconds()), "", true
			}
		}
		if delta.Content != "" {
			sb.WriteString(delta.Content)
			if !send(ctx, out, StreamReply{Phase: PhaseGenerate, Token: delta.Content}) {
				return sb.String(), rb.String(), usage, int(time.Since(generateStart).Milliseconds()), "", true
			}
		}
	}
	return sb.String(), rb.String(), usage, int(time.Since(generateStart).Milliseconds()), "", false
}

func (s *Service) runStream(
	ctx context.Context,
	tenantID string,
	conv *Conversation,
	in SendInput, provider *llm.LLMModel,
	userMsg *Message, out chan<- StreamReply,
	cfg agent.ExecutionConfig,
) {
	defer close(out)
	defer func() {
		if r := recover(); r != nil {
			select {
			case out <- StreamReply{Phase: "error", Error: fmt.Sprintf("internal error: %v", r)}:
			default:
			}
		}
	}()

	totalStart := time.Now()
	query := in.Query
	if query == "" {
		query = in.Message
	}

	// Try graph-based execution when a full agent definition is available.
	// This supports classifier/condition/message routing. Falls back to the
	// flat ExecutionConfig path when no definition is wired.
	if def := s.loadAgentDefinition(ctx, tenantID, conv.KbID); def != nil {
		s.runGraph(ctx, tenantID, conv.OwnerID, conv.KbID, conv.ID, query, in.Message, userMsg.ID, def, provider, out, true)
		return
	}

	// Flat config path (original flow): retrieval -> build prompt -> stream LLM.
	retrieveStart := time.Now()
	var res *search.SearchResult
	if cfg.UseRetrieval {
		topK := in.TopK
		if topK <= 0 {
			topK = cfg.TopK
		}
		rerank := cfg.Rerank
		r, err := s.searchSvc.Search(ctx, tenantID, conv.KbID, search.SearchRequest{
			Query: query, TopK: topK, Rerank: &rerank, RerankModelID: cfg.RerankModelID,
		})
		if err != nil {
			log.Printf("[chat] retrieval failed tenant=%s kb=%s: %v", tenantID, conv.KbID, err)
			out <- StreamReply{Phase: PhaseWarning, Warning: "Retrieval failed; answering without context."}
			res = &search.SearchResult{}
		} else {
			res = r
		}
	} else {
		res = &search.SearchResult{}
	}
	retrieveMs := int(time.Since(retrieveStart).Milliseconds())

	// Trim the retrieval context to its budget share BEFORE formatting so
	// citations stay consistent with what actually reaches the model.
	budget := promptBudget(provider)
	ctxTrimmed := false
	if hits, trimmed := trimHitsToBudget(res.Hits, int(float64(budget)*retrievalShare)); trimmed {
		res.Hits = hits
		ctxTrimmed = true
	}
	systemContext, cits := formatContext(res.Hits)
	if !send(ctx, out, StreamReply{Phase: PhaseRetrieve, Citations: cits}) {
		return
	}

	history, err := s.repo.RecentMsgs(tenantID, conv.ID, s.history)
	if err != nil {
		log.Printf("[chat] load history failed tenant=%s conv=%s: %v", tenantID, conv.ID, err)
	}

	graphCtx := res.GraphContext
	if !cfg.UseGraph {
		graphCtx = ""
	}
	// conv.OwnerID is the chatting user (Stream resolves the conversation
	// via FindConvOwned), so memories stay scoped per user on shared KBs.
	memoryCtx := s.loadMemoryContext(ctx, tenantID, conv.OwnerID, conv.KbID)
	msgs, promptTrimmed := buildPrompt(systemContext, graphCtx, memoryCtx, history, in.Message, userMsg.ID, cfg.SystemPrompt, budget)
	if ctxTrimmed || promptTrimmed {
		if !send(ctx, out, StreamReply{Phase: PhaseWarning, Warning: "Context trimmed to fit the model window."}) {
			return
		}
	}

	content, reasoning, usage, generateMs, streamErr, cancelled := s.streamLLM(ctx, out, provider, cfg, msgs)
	totalMs := int(time.Since(totalStart).Milliseconds())

	if streamErr != "" || cancelled {
		s.saveAssistant(tenantID, conv.ID, content, cits, nil, reasoning)
		return
	}
	stats := &ReplyStats{
		RetrieveMs: retrieveMs,
		GenerateMs: generateMs,
		TotalMs:    totalMs,
	}
	if usage != nil {
		stats.PromptTokens = usage.PromptTokens
		stats.CompletionTokens = usage.CompletionTokens
		stats.TotalTokens = usage.TotalTokens
	}
	assistantID := s.saveAssistant(tenantID, conv.ID, content, cits, stats, reasoning)
	s.triggerAutoMemory(tenantID, conv.ID)
	send(ctx, out, StreamReply{Phase: PhaseDone, MessageID: assistantID, Stats: stats})
	if err := s.repo.TouchConv(tenantID, conv.ID); err != nil {
		log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, conv.ID, err)
	}
}

// runGraph executes the agent graph and handles the terminal node. When the
// terminal is a message node, it emits the direct reply without calling the
// LLM. When it's an LLM node, it streams the reply using the node's config.
// persist controls whether assistant messages are saved (true for Stream,
// false for TestStream).
func (s *Service) runGraph(
	ctx context.Context,
	tenantID, userID, kbID, convID, query, userMessage, excludeMsgID string,
	def *agent.Definition,
	provider *llm.LLMModel,
	out chan<- StreamReply,
	persist bool,
) {
	totalStart := time.Now()
	deps := s.graphDeps(tenantID, kbID, query)

	// Load history for LLM nodes (test mode has no history).
	var history []*Message
	if persist {
		var err error
		history, err = s.repo.RecentMsgs(tenantID, convID, s.history)
		if err != nil {
			log.Printf("[chat] load history failed tenant=%s conv=%s: %v", tenantID, convID, err)
		}
	}

	histMsgs := make([]clients.ChatMessage, 0, len(history))
	for _, m := range history {
		if m.ID == excludeMsgID {
			continue
		}
		histMsgs = append(histMsgs, clients.ChatMessage{Role: m.Role, Content: m.Content})
	}

	ec := &agent.ExecutionContext{
		Query:   query,
		History: histMsgs,
	}
	ec.MemoryContext = s.loadMemoryContext(ctx, tenantID, userID, kbID)

	var retrieveMs int
	retrieveStart := time.Now()

	terminal := agent.Execute(ctx, def, deps, tenantID, kbID, ec, func(ev agent.ExecutionEvent) {
		if ev.Phase == agent.EvWarning && ev.Warning != "" {
			send(ctx, out, StreamReply{Phase: PhaseWarning, Warning: ev.Warning})
		}
		if ev.Phase == agent.EvTrace && ev.Trace != nil {
			send(ctx, out, StreamReply{Phase: "trace", Trace: []agent.TraceStep{*ev.Trace}})
		}
	})
	retrieveMs = int(time.Since(retrieveStart).Milliseconds())

	cits := citationsFromAny(ec.Citations)
	if !send(ctx, out, StreamReply{Phase: PhaseRetrieve, Citations: cits}) {
		return
	}

	// Handle terminal node.
	switch terminal.Type {
	case agent.NodeMessage:
		// Direct reply without LLM. Emit the text as a single generate
		// token, then done.
		text := ec.DirectReply
		if text == "" {
			text = "（空回复）"
		}
		if !send(ctx, out, StreamReply{Phase: PhaseGenerate, Token: text}) {
			return
		}
		var assistantID string
		if persist {
			assistantID = s.saveAssistant(tenantID, convID, text, cits, &ReplyStats{
				RetrieveMs: retrieveMs,
				TotalMs:    int(time.Since(totalStart).Milliseconds()),
			}, "")
			s.triggerAutoMemory(tenantID, convID)
			if err := s.repo.TouchConv(tenantID, convID); err != nil {
				log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, convID, err)
			}
		}
		send(ctx, out, StreamReply{Phase: PhaseDone, MessageID: assistantID, Stats: &ReplyStats{
			RetrieveMs: retrieveMs,
			TotalMs:    int(time.Since(totalStart).Milliseconds()),
		}, Trace: terminal.Trace})

	case agent.NodeLLM:
		cfg := terminal.LLMCfg
		if cfg == nil {
			cfg2 := agent.DefaultExecutionConfig()
			cfg = &cfg2
		}
		// Resolve LLM provider for this node (may differ from the default).
		llmProvider := provider
		if cfg.LLMModelID != "" {
			if p, err := s.resolveProvider(ctx, tenantID, cfg.LLMModelID); err == nil {
				llmProvider = p
			}
		}
		budget := promptBudget(llmProvider)
		msgs, promptTrimmed := buildPrompt(ec.SystemContext, ec.GraphContext, ec.MemoryContext, history, userMessage, excludeMsgID, cfg.SystemPrompt, budget)
		if promptTrimmed {
			if !send(ctx, out, StreamReply{Phase: PhaseWarning, Warning: "Context trimmed to fit the model window."}) {
				return
			}
		}
		content, reasoning, usage, generateMs, streamErr, cancelled := s.streamLLM(ctx, out, llmProvider, *cfg, msgs)
		totalMs := int(time.Since(totalStart).Milliseconds())

		if streamErr != "" || cancelled {
			if persist {
				s.saveAssistant(tenantID, convID, content, cits, nil, reasoning)
			}
			return
		}
		stats := &ReplyStats{
			RetrieveMs: retrieveMs,
			GenerateMs: generateMs,
			TotalMs:    totalMs,
		}
		if usage != nil {
			stats.PromptTokens = usage.PromptTokens
			stats.CompletionTokens = usage.CompletionTokens
			stats.TotalTokens = usage.TotalTokens
		}
		var assistantID string
		if persist {
			assistantID = s.saveAssistant(tenantID, convID, content, cits, stats, reasoning)
			s.triggerAutoMemory(tenantID, convID)
			if err := s.repo.TouchConv(tenantID, convID); err != nil {
				log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, convID, err)
			}
		}
		send(ctx, out, StreamReply{Phase: PhaseDone, MessageID: assistantID, Stats: stats, Trace: terminal.Trace})

	default:
		send(ctx, out, StreamReply{Phase: PhaseError, Error: "agent graph ended without a terminal node"})
	}
}

// saveAssistant writes the assistant message row and returns its id. Errors
// are swallowed because the user already saw the streamed reply; losing the
// persisted row is recoverable (user can re-ask).
func (s *Service) saveAssistant(tenantID, convID, content string, cits []Citation, stats *ReplyStats, reasoning string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	m := &Message{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		ConversationID: convID,
		Role:           RoleAssistant,
		Content:        content,
		Reasoning:      reasoning,
		Citations:      encodeCitations(cits),
	}
	if stats != nil {
		m.RetrieveMs = stats.RetrieveMs
		m.GenerateMs = stats.GenerateMs
		m.TotalMs = stats.TotalMs
		m.PromptTokens = stats.PromptTokens
		m.CompletionTokens = stats.CompletionTokens
		m.TotalTokens = stats.TotalTokens
	}
	if err := s.repo.CreateMsg(m); err != nil {
		return ""
	}
	return m.ID
}

// resolveProvider returns the LLM provider for a tenant: the explicit id from
// the agent config, else the tenant default.
func (s *Service) resolveProvider(ctx context.Context, tenantID, providerID string) (*llm.LLMModel, error) {
	if providerID != "" {
		return s.llmRepo.FindByID(tenantID, providerID)
	}
	p, err := s.llmRepo.FindDefault(tenantID)
	if err != nil {
		return nil, errs.BadRequest("no llm provider configured for tenant; create one first")
	}
	return p, nil
}

// testStream runs the agent pipeline (retrieval + LLM streaming) without
// persisting anything — no conversation, no messages, no stats rows. Used by
// the agent config test drawer so users can try prompts/settings without
// creating throwaway conversations. The LLM-streaming logic is shared with
// runStream via streamLLM; only persistence and history loading differ.
func (s *Service) testStream(
	ctx context.Context,
	tenantID, userID, kbID, query string,
	provider *llm.LLMModel,
	out chan<- StreamReply,
	cfg agent.ExecutionConfig,
) {
	defer close(out)
	defer func() {
		if r := recover(); r != nil {
			select {
			case out <- StreamReply{Phase: "error", Error: fmt.Sprintf("internal error: %v", r)}:
			default:
			}
		}
	}()

	// Graph-based execution when a definition is available.
	if def := s.loadAgentDefinition(ctx, tenantID, kbID); def != nil {
		s.runGraph(ctx, tenantID, userID, kbID, "", query, query, "", def, provider, out, false)
		return
	}

	// Flat config path (original flow).
	totalStart := time.Now()
	retrieveStart := totalStart

	var res *search.SearchResult
	if cfg.UseRetrieval {
		r, err := s.searchSvc.Search(ctx, tenantID, kbID, search.SearchRequest{
			Query: query, TopK: cfg.TopK, Rerank: &cfg.Rerank, RerankModelID: cfg.RerankModelID,
		})
		if err != nil {
			log.Printf("[chat] test retrieval failed tenant=%s kb=%s: %v", tenantID, kbID, err)
			send(ctx, out, StreamReply{Phase: PhaseWarning, Warning: "Retrieval failed; answering without context."})
			res = &search.SearchResult{}
		} else {
			res = r
		}
	} else {
		res = &search.SearchResult{}
	}
	systemContext, cits := formatContext(res.Hits)
	retrieveMs := int(time.Since(retrieveStart).Milliseconds())
	if !send(ctx, out, StreamReply{Phase: PhaseRetrieve, Citations: cits}) {
		return
	}

	budget := promptBudget(provider)
	if hits, trimmed := trimHitsToBudget(res.Hits, int(float64(budget)*retrievalShare)); trimmed {
		res.Hits = hits
		systemContext, cits = formatContext(res.Hits)
		if !send(ctx, out, StreamReply{Phase: PhaseRetrieve, Citations: cits}) {
			return
		}
	}

	graphCtx := res.GraphContext
	if !cfg.UseGraph {
		graphCtx = ""
	}
	memoryCtx := s.loadMemoryContext(ctx, tenantID, userID, kbID)
	msgs, promptTrimmed := buildPrompt(systemContext, graphCtx, memoryCtx, nil, query, "", cfg.SystemPrompt, budget)
	if promptTrimmed {
		if !send(ctx, out, StreamReply{Phase: PhaseWarning, Warning: "Context trimmed to fit the model window."}) {
			return
		}
	}

	_, _, usage, generateMs, streamErr, cancelled := s.streamLLM(ctx, out, provider, cfg, msgs)
	if streamErr != "" || cancelled {
		return
	}

	stats := &ReplyStats{
		RetrieveMs: retrieveMs,
		GenerateMs: generateMs,
		TotalMs:    int(time.Since(totalStart).Milliseconds()),
	}
	if usage != nil {
		stats.PromptTokens = usage.PromptTokens
		stats.CompletionTokens = usage.CompletionTokens
		stats.TotalTokens = usage.TotalTokens
	}
	send(ctx, out, StreamReply{Phase: PhaseDone, Stats: stats})
}
