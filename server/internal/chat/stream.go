package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"ollmo/ollmo/internal/agent"
	"ollmo/ollmo/internal/annotation"
	"ollmo/ollmo/internal/bill"
	"ollmo/ollmo/internal/execution"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/internal/search"
	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/errs"
	"ollmo/ollmo/pkg/tokener"
)

// chatUsage estimates token counts from raw prompt/completion text for
// providers that omit the usage field. The CJK-aware estimator is
// deliberately conservative; cost figures stay estimates regardless.
func chatUsage(prompt, completion string) *clients.TokenUsage {
	p := tokener.Estimate(prompt)
	c := tokener.Estimate(completion)
	return &clients.TokenUsage{PromptTokens: p, CompletionTokens: c, TotalTokens: p + c}
}

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
// tenantID/userID/kbID/convID/query from the surrounding scope. ChargeLLM
// bills classifier/intermediate node calls back to the conversation owner.
func (s *Service) graphDeps(tenantID, userID, kbID, convID, query string, trackHits, charge bool) agent.ExecutionDeps {
	return agent.ExecutionDeps{
		Search: func(ctx context.Context, tid, kid, q string, topK int, rerank bool, rerankModelID string, useGraph bool) (string, int, float64, string, []any, error) {
			r, err := s.searchSvc.Search(ctx, tid, kid, search.SearchRequest{
				Query: q, TopK: topK, Rerank: &rerank, RerankModelID: rerankModelID, TrackHits: trackHits,
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
		ChargeLLM: func(modelID, source string, usage *clients.TokenUsage) {
			if modelID == "" {
				return
			}
			if !charge {
				return
			}
			m, err := s.llmRepo.FindByID(tenantID, modelID)
			if err != nil {
				return
			}
			s.recordUsage(tenantID, userID, convID, kbID, source, m, usage)
		},
	}
}

// DebugAgentNode runs one agent node in isolation for the canvas "test this
// node" action. Nothing is persisted.
func (s *Service) DebugAgentNode(ctx context.Context, tenantID, kbID string, node agent.Node, query string) (*agent.NodeDebugResult, error) {
	deps := s.graphDeps(tenantID, "", kbID, "", query, false, false)
	return agent.DebugNode(ctx, deps, tenantID, kbID, node, query)
}

// send delivers a reply to the client channel, respecting ctx cancellation.
// Returns false when ctx is cancelled (client disconnected) so the caller can
// stop sending SSE events while the LLM continues producing output in the
// background.
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
//   - cancelled: the client disconnected mid-stream but the LLM call
//     continued to completion via an independent context, so content holds
//     the full generated answer.
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

	// Independent context so the LLM call survives client disconnects: the
	// stream keeps running even if the user closes the browser, and output
	// is still saved so history stays consistent with what was generated.
	// The context derives from the server-lifetime context (not the request)
	// so a server shutdown still cancels these background streams. The
	// timeout matches the LLM client's hard cap exactly.
	base := s.bgCtx
	if base == nil {
		base = context.Background()
	}
	llmCtx, cancelLLM := context.WithTimeout(base, clients.StreamHardTimeout)
	defer cancelLLM()

	var sb, rb strings.Builder
	generateStart := time.Now()
	clientGone := false

	for delta := range s.llm.ChatStream(llmCtx, provider.Endpoint, provider.APIKey, req) {
		if delta.Err != nil {
			if !clientGone {
				send(ctx, out, StreamReply{Phase: PhaseError, Error: delta.Err.Error()})
			}
			return sb.String(), rb.String(), usage, int(time.Since(generateStart).Milliseconds()), delta.Err.Error(), clientGone
		}
		if delta.Done {
			break
		}
		if delta.Usage != nil {
			usage = delta.Usage
		}
		if delta.Reasoning != "" {
			rb.WriteString(delta.Reasoning)
			if !clientGone {
				if !send(ctx, out, StreamReply{Phase: PhaseThinking, Token: delta.Reasoning}) {
					clientGone = true
				}
			}
		}
		if delta.Content != "" {
			sb.WriteString(delta.Content)
			if !clientGone {
				if !send(ctx, out, StreamReply{Phase: PhaseGenerate, Token: delta.Content}) {
					clientGone = true
				}
			}
		}
	}
	return sb.String(), rb.String(), usage, int(time.Since(generateStart).Milliseconds()), "", clientGone
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
			Query: query, TopK: topK, Rerank: &rerank, RerankModelID: cfg.RerankModelID, TrackHits: true,
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
		s.saveAssistant(tenantID, conv.ID, "（连接中断，请重试）", cits, nil, "", false)
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
			s.saveAssistant(tenantID, conv.ID, "（连接中断，请重试）", cits, nil, "", false)
			return
		}
	}

	content, reasoning, usage, generateMs, streamErr, cancelled := s.streamLLM(ctx, out, provider, cfg, msgs)
	totalMs := int(time.Since(totalStart).Milliseconds())

	if cancelled {
		// Client disconnected mid-stream; the LLM call kept running on an
		// independent context and finished in the background (cleanly, or
		// with an error the client never saw). Save the best available
		// result — full content, stats and billing — so the conversation
		// record matches what was actually generated.
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
		s.recordUsage(tenantID, conv.OwnerID, conv.ID, conv.KbID, bill.SourceChat, provider, usage)
		s.saveAssistant(tenantID, conv.ID, content, cits, stats, reasoning, false)
		s.triggerAutoMemory(tenantID, conv.ID)
		if err := s.repo.TouchConv(tenantID, conv.ID); err != nil {
			log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, conv.ID, err)
		}
		return
	}
	if streamErr != "" {
		s.saveAssistant(tenantID, conv.ID, content, cits, nil, reasoning, false)
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
	s.recordUsage(tenantID, conv.OwnerID, conv.ID, conv.KbID, bill.SourceChat, provider, usage)
	assistantID := s.saveAssistant(tenantID, conv.ID, content, cits, stats, reasoning, false)
	s.triggerAutoMemory(tenantID, conv.ID)
	send(ctx, out, StreamReply{Phase: PhaseDone, MessageID: assistantID, Stats: stats})
	s.emitFollowUps(ctx, provider, tenantID, conv.OwnerID, conv.ID, conv.KbID, assistantID, query, content, out)
	if err := s.repo.TouchConv(tenantID, conv.ID); err != nil {
		log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, conv.ID, err)
	}
}

func (s *Service) streamAnnotation(
	ctx context.Context,
	tenantID, userID string,
	conv *Conversation,
	in SendInput,
	query string,
	m *annotation.MatchResult,
) (<-chan StreamReply, error) {
	cfg := s.loadAgentConfig(ctx, tenantID, conv.KbID)
	provider, err := s.resolveProvider(ctx, tenantID, cfg.LLMModelID)
	if err != nil {
		return nil, err
	}

	userMsg := &Message{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		ConversationID: conv.ID,
		Role:           RoleUser,
		Content:        in.Message,
	}
	if err := s.repo.CreateMsg(userMsg); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "save user message", err)
	}
	if err := s.repo.TouchConv(tenantID, conv.ID); err != nil {
		log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, conv.ID, err)
	}

	out := make(chan StreamReply, 16)
	src := make(chan StreamReply, 16)
	go s.replyAnnotation(ctx, tenantID, conv, m, src, time.Now(), provider, query)

	go func() {
		defer close(out)
		s.hub.Publish(conv.ID, StreamReply{Phase: PhaseUser, MessageID: userMsg.ID, Token: in.Message})
		for r := range src {
			s.hub.Publish(conv.ID, r)
			if !send(ctx, out, r) {
				for range src {
				}
				return
			}
		}
	}()
	return out, nil
}

// replyAnnotation streams a matched annotation answer through the same SSE
// channel as LLM replies, split into token-sized chunks so the frontend
// renderer (onToken reducer) handles both paths identically.
func (s *Service) replyAnnotation(ctx context.Context, tenantID string, conv *Conversation, m *annotation.MatchResult, out chan<- StreamReply, start time.Time, provider *llm.LLMModel, query string) {
	answer := m.Annotation.Answer
	stats := &ReplyStats{TotalMs: int(time.Since(start).Milliseconds())}

	runes := []rune(answer)
	const chunkSize = 3
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		if !send(ctx, out, StreamReply{Phase: PhaseGenerate, Token: string(runes[i:end]), Annotation: true}) {
			s.saveAssistant(tenantID, conv.ID, answer, nil, nil, "", true)
			return
		}
	}

	assistantID := s.saveAssistant(tenantID, conv.ID, answer, nil, stats, "", true)
	send(ctx, out, StreamReply{Phase: PhaseDone, MessageID: assistantID, Stats: stats, Annotation: true})
	s.emitFollowUps(ctx, provider, tenantID, conv.OwnerID, conv.ID, conv.KbID, assistantID, query, answer, out)
	if err := s.repo.TouchConv(tenantID, conv.ID); err != nil {
		log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, conv.ID, err)
	}
}

// Follow-up chip generation knobs. Inputs are capped so the extra LLM call
// stays cheap regardless of answer length; the call has its own timeout so a
// slow model never hangs the SSE stream open.
const (
	followUpCount          = 3
	followUpTimeout        = 15 * time.Second
	followUpMaxQueryChars  = 500
	followUpMaxAnswerChars = 2000
)

// truncateRunes cuts s to at most n runes, suffixing with an ellipsis when
// truncated.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// parseFollowUps extracts a JSON string array from an LLM reply, tolerating
// markdown code fences and stray prose around it. Returns nil when nothing
// parseable is found.
func parseFollowUps(raw string) []string {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(raw[start:end+1]), &items); err != nil {
		return nil
	}
	out := make([]string, 0, followUpCount)
	for _, q := range items {
		q = strings.TrimSpace(q)
		if q != "" {
			out = append(out, q)
		}
		if len(out) >= followUpCount {
			break
		}
	}
	return out
}

// generateFollowUps asks the LLM for short follow-up questions based on the
// just-completed turn. Best-effort: errors and timeouts return nil so the
// chat stream is never affected by chip generation failures. The follow-up
// call itself is billed when the usage recorder is wired.
func (s *Service) generateFollowUps(ctx context.Context, tenantID, userID, kbID, convID string, provider *llm.LLMModel, query, answer string) []string {
	ctx, cancel := context.WithTimeout(ctx, followUpTimeout)
	defer cancel()
	prompt := fmt.Sprintf(
		"Based on the Q&A below, generate %d short follow-up questions the user might ask next. "+
			"Reply with ONLY a JSON array of %d strings, in the same language as the Q&A. No numbering, no explanations.\n\nUser: %s\n\nAssistant: %s",
		followUpCount, followUpCount,
		truncateRunes(query, followUpMaxQueryChars),
		truncateRunes(answer, followUpMaxAnswerChars),
	)
	resp, usage, err := s.llm.Chat(ctx, provider.Endpoint, provider.APIKey, clients.ChatRequest{
		Model:       provider.Model,
		Messages:    []clients.ChatMessage{{Role: "user", Content: prompt}},
		Temperature: 0.5,
		MaxTokens:   200,
	})
	if usage != nil {
		s.recordUsage(tenantID, userID, convID, kbID, bill.SourceFollowUps, provider, usage)
	} else if err == nil {
		s.recordUsage(tenantID, userID, convID, kbID, bill.SourceFollowUps, provider, chatUsage(prompt, resp))
	}
	if err != nil {
		log.Printf("[chat] follow-up generation failed: %v", err)
		return nil
	}
	questions := parseFollowUps(resp)
	if len(questions) == 0 {
		log.Printf("[chat] follow-up parse empty, raw: %s", truncateRunes(resp, 300))
	}
	return questions
}

// emitFollowUps generates follow-up suggestions for the saved assistant
// message, persists them on the message row, and pushes a follow_ups event.
// Called after the done event so chip generation never delays the answer;
// failures are silent (chips are best-effort).
func (s *Service) emitFollowUps(ctx context.Context, provider *llm.LLMModel, tenantID, userID, convID, kbID, msgID, query, answer string, out chan<- StreamReply) {
	if msgID == "" || strings.TrimSpace(answer) == "" {
		return
	}
	questions := s.generateFollowUps(ctx, tenantID, userID, kbID, convID, provider, query, answer)
	if len(questions) == 0 {
		return
	}
	if b, err := json.Marshal(questions); err == nil {
		if err := s.repo.SetMsgFollowUps(tenantID, convID, msgID, string(b)); err != nil {
			log.Printf("[chat] save follow-ups failed tenant=%s conv=%s msg=%s: %v", tenantID, convID, msgID, err)
		}
	}
	send(ctx, out, StreamReply{Phase: PhaseFollowUp, Questions: questions})
}

// recordExecution persists one agent-graph run for the replay page. Failures
// are logged and swallowed: losing history must never break the chat stream.
func (s *Service) recordExecution(e *execution.Execution) {
	if s.execRepo == nil {
		return
	}
	if err := s.execRepo.Create(e); err != nil {
		log.Printf("[chat] save execution failed tenant=%s kb=%s: %v", e.TenantID, e.KbID, err)
	}
}

// newExecution builds an execution record for one graph run with its final
// outcome. Called via defer from runGraph so every exit path is recorded.
func newExecution(tenantID, userID, kbID, convID, query, source, status, answer, messageID, terminalType string, trace []agent.TraceStep, totalMs int) *execution.Execution {
	traceJSON, err := json.Marshal(trace)
	if err != nil {
		traceJSON = nil
	}
	return &execution.Execution{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		KbID:           kbID,
		ConversationID: convID,
		MessageID:      messageID,
		UserID:         userID,
		Source:         source,
		Status:         status,
		Query:          query,
		Answer:         answer,
		TerminalType:   terminalType,
		Trace:          string(traceJSON),
		TotalMs:        totalMs,
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

	// Execution recording: one row per graph run with its full trace so the
	// replay page can re-highlight the walk. status/answer/messageID mutate
	// through the branches below; the deferred closure reads final values.
	src := execution.SourceTest
	if persist {
		src = execution.SourceChat
	}
	var terminal agent.Terminal
	status := execution.StatusCancelled
	var answer, messageID string
	defer func() {
		s.recordExecution(newExecution(tenantID, userID, kbID, convID, query, src, status, answer, messageID, terminal.Type, terminal.Trace, int(time.Since(totalStart).Milliseconds())))
	}()

	deps := s.graphDeps(tenantID, userID, kbID, convID, query, persist, persist)

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

	terminal = agent.Execute(ctx, def, deps, tenantID, kbID, ec, func(ev agent.ExecutionEvent) {
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
		if persist {
			s.saveAssistant(tenantID, convID, "（连接中断，请重试）", cits, nil, "", false)
		}
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
		answer = text
		if !send(ctx, out, StreamReply{Phase: PhaseGenerate, Token: text}) {
			if persist {
				s.saveAssistant(tenantID, convID, text, cits, &ReplyStats{
					RetrieveMs: retrieveMs,
					TotalMs:    int(time.Since(totalStart).Milliseconds()),
				}, "", false)
			}
			if strings.TrimSpace(text) != "" {
				status = execution.StatusSuccess
			}
			return
		}
		var assistantID string
		if persist {
			assistantID = s.saveAssistant(tenantID, convID, text, cits, &ReplyStats{
				RetrieveMs: retrieveMs,
				TotalMs:    int(time.Since(totalStart).Milliseconds()),
			}, "", false)
			s.triggerAutoMemory(tenantID, convID)
			if err := s.repo.TouchConv(tenantID, convID); err != nil {
				log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, convID, err)
			}
		}
		messageID = assistantID
		status = execution.StatusSuccess
		send(ctx, out, StreamReply{Phase: PhaseDone, MessageID: assistantID, Stats: &ReplyStats{
			RetrieveMs: retrieveMs,
			TotalMs:    int(time.Since(totalStart).Milliseconds()),
		}, Trace: terminal.Trace})
		if persist {
			s.emitFollowUps(ctx, provider, tenantID, userID, convID, kbID, assistantID, query, text, out)
		}

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
				if persist {
					s.saveAssistant(tenantID, convID, "（连接中断，请重试）", cits, nil, "", false)
				}
				return
			}
		}
		content, reasoning, usage, generateMs, streamErr, cancelled := s.streamLLM(ctx, out, llmProvider, *cfg, msgs)
		totalMs := int(time.Since(totalStart).Milliseconds())

		answer = content
		if cancelled {
			// Client disconnected mid-stream; the LLM call kept running on
			// an independent context until it ended (cleanly, or with an
			// error the client never saw). Save everything — billing,
			// message row, conv touch — so the conversation record matches
			// what was actually generated.
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
			if persist {
				s.recordUsage(tenantID, userID, convID, kbID, bill.SourceChat, llmProvider, usage)
				assistantID := s.saveAssistant(tenantID, convID, content, cits, stats, reasoning, false)
				messageID = assistantID
				s.triggerAutoMemory(tenantID, convID)
				if err := s.repo.TouchConv(tenantID, convID); err != nil {
					log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, convID, err)
				}
			}
			if strings.TrimSpace(content) != "" {
				status = execution.StatusSuccess
			}
			return
		}
		if streamErr != "" {
			if persist {
				s.saveAssistant(tenantID, convID, content, cits, nil, reasoning, false)
			}
			status = execution.StatusError
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
			// Charge the terminal LLM node to the conversation owner. Test
			// mode (persist=false) persists nothing, so it is not billed.
			s.recordUsage(tenantID, userID, convID, kbID, bill.SourceChat, llmProvider, usage)
			assistantID = s.saveAssistant(tenantID, convID, content, cits, stats, reasoning, false)
			s.triggerAutoMemory(tenantID, convID)
			if err := s.repo.TouchConv(tenantID, convID); err != nil {
				log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, convID, err)
			}
		}
		messageID = assistantID
		status = execution.StatusSuccess
		send(ctx, out, StreamReply{Phase: PhaseDone, MessageID: assistantID, Stats: stats, Trace: terminal.Trace})
		if persist {
			s.emitFollowUps(ctx, llmProvider, tenantID, userID, convID, kbID, assistantID, query, content, out)
		}

	default:
		status = execution.StatusError
		send(ctx, out, StreamReply{Phase: PhaseError, Error: "agent graph ended without a terminal node"})
		if persist {
			s.saveAssistant(tenantID, convID, "（Agent 执行异常，请重试）", cits, nil, "", false)
		}
	}
}

// saveAssistant writes the assistant message row and returns its id. Errors
// are swallowed because the user already saw the streamed reply; losing the
// persisted row is recoverable (user can re-ask).
func (s *Service) saveAssistant(tenantID, convID, content string, cits []Citation, stats *ReplyStats, reasoning string, annotation bool) string {
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
		Annotation:     annotation,
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
