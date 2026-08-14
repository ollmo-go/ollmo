package chat

import (
	"context"
	"log"
	"strings"

	"github.com/google/uuid"

	"ollmo/ollmo/internal/agent"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/internal/search"
	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/errs"
)

// AgentConfigFetcher returns the agent execution config for a KB. When nil or
// returning nil, the chat service falls back to its hardcoded defaults so
// KBs without an agent definition keep working unchanged.
type AgentConfigFetcher func(ctx context.Context, tenantID, kbID string) (*agent.ExecutionConfig, error)

// AgentDefinitionFetcher returns the full agent definition for a KB, enabling
// graph-based execution (classifier/condition/message routing). When nil or
// returning nil, the chat service falls back to AgentConfigFetcher (flat config).
type AgentDefinitionFetcher func(ctx context.Context, tenantID, kbID string) (*agent.Definition, error)

// MemoryContextFetcher returns a user's past conversation summaries for a
// KB. When nil or returning empty, the chat service omits memory context
// from the prompt.
type MemoryContextFetcher func(ctx context.Context, tenantID, userID, kbID string) (string, error)

// AutoMemoryTrigger is invoked after each completed (persisted) chat turn.
// The implementation decides whether the conversation should be summarized
// (threshold checks, feature flags, enqueueing). Kept as a plain function so
// the chat package does not depend on the memory package (which already
// imports chat). Errors must be handled by the implementation.
type AutoMemoryTrigger func(tenantID, convID string)

// ConversationDeletedHook is invoked after a conversation is successfully
// deleted so dependent subsystems (e.g. memory) can clean up their rows.
type ConversationDeletedHook func(tenantID, convID string)

// MessageQuotaChecker enforces daily message limits at two levels: tenant
// total and per-user cap. Check returns the effective remaining (-1 for
// unlimited); Incr bumps both counters after a message is accepted;
// Usage returns (effectiveRemaining, userLimit) for display.
type MessageQuotaChecker interface {
	CheckMessageQuota(tenantID, userID string) (remaining int, err error)
	IncrMessageUsage(tenantID, userID string) error
	MessageUsage(tenantID, userID string) (remaining, userLimit int, err error)
}

// Service wires retrieval + LLM streaming. The chat flow is:
//  1. resolve conversation + LLM provider
//  2. load recent history for context
//  3. retrieve chunks from the KB (search.Search), customized by agent config
//  4. build the system+context+history prompt, enriched with memory summaries
//  5. stream the assistant reply via LLMClient.ChatStream
//  6. persist user + assistant messages and citations
type Service struct {
	repo      *Repo
	searchSvc *search.Service
	llmRepo   *llm.Repo
	llm       *clients.LLMClient
	history   int // messages of context to pass to the LLM
	agentCfg  AgentConfigFetcher
	agentDef  AgentDefinitionFetcher
	memCtx    MemoryContextFetcher
	autoMem   AutoMemoryTrigger
	onConvDel ConversationDeletedHook
	msgQuota  MessageQuotaChecker
}

func NewService(repo *Repo, searchSvc *search.Service, llmRepo *llm.Repo, llm *clients.LLMClient) *Service {
	return &Service{repo: repo, searchSvc: searchSvc, llmRepo: llmRepo, llm: llm, history: 10}
}

// WithAgentConfig wires the agent config fetcher. When set, each Stream call
// loads the KB's agent definition and uses it to customize retrieval and LLM
// parameters. KBs without a saved agent fall back to the hardcoded defaults.
func (s *Service) WithAgentConfig(f AgentConfigFetcher) *Service {
	s.agentCfg = f
	return s
}

// WithAgentDefinition wires the full agent definition fetcher. When set, the
// chat service runs the graph executor (agent.Execute) which supports
// classifier/condition/message routing. Falls back to flat config when nil.
func (s *Service) WithAgentDefinition(f AgentDefinitionFetcher) *Service {
	s.agentDef = f
	return s
}

// WithMemoryContext wires the memory context fetcher. When set, each Stream
// call loads past conversation summaries and appends them to the system prompt
// so the LLM has cross-session context without loading full histories.
func (s *Service) WithMemoryContext(f MemoryContextFetcher) *Service {
	s.memCtx = f
	return s
}

// WithAutoMemory wires the auto-summarization trigger. When set, it is called
// after each persisted chat turn so the memory subsystem can decide whether to
// enqueue an async summary. Not called for TestStream (nothing is persisted).
func (s *Service) WithAutoMemory(f AutoMemoryTrigger) *Service {
	s.autoMem = f
	return s
}

// triggerAutoMemory invokes the auto-memory hook if wired. Runs in a goroutine
// so the (cheap) threshold check never delays the SSE done event.
func (s *Service) triggerAutoMemory(tenantID, convID string) {
	if s.autoMem == nil {
		return
	}
	go s.autoMem(tenantID, convID)
}

// WithConversationDeleted wires a hook invoked after a conversation is
// removed, letting dependent subsystems (e.g. memory) clean up their rows.
func (s *Service) WithConversationDeleted(f ConversationDeletedHook) *Service {
	s.onConvDel = f
	return s
}

// WithMessageQuota wires the daily message quota checker. When set, Stream
// and TestStream reject requests once the tenant exceeds its daily limit.
func (s *Service) WithMessageQuota(qc MessageQuotaChecker) *Service {
	s.msgQuota = qc
	return s
}

// MessageUsage returns (effectiveRemaining, userLimit) for the current day.
// userLimit=-1 means unlimited. Returns (0, 0, nil) when no quota checker is
// wired.
func (s *Service) MessageUsage(tenantID, userID string) (int, int, error) {
	if s.msgQuota == nil {
		return 0, 0, nil
	}
	return s.msgQuota.MessageUsage(tenantID, userID)
}

// CreateInput is the request body for starting a conversation.
type CreateInput struct {
	Title string `json:"title"`
}

func (s *Service) Create(ctx context.Context, tenantID, ownerID, kbID string, in CreateInput) (*Conversation, error) {
	if in.Title == "" {
		in.Title = "New Conversation"
	}
	c := &Conversation{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		KbID:     kbID,
		Title:    in.Title,
		OwnerID:  ownerID,
	}
	if err := s.repo.CreateConv(c); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create conversation", err)
	}

	// If the KB's agent defines an opening message, save it as the first
	// assistant message so the user sees a greeting when the chat opens.
	cfg := s.loadAgentConfig(ctx, tenantID, kbID)
	if cfg.OpeningMessage != "" {
		if err := s.repo.CreateMsg(&Message{
			ID:             uuid.NewString(),
			TenantID:       tenantID,
			ConversationID: c.ID,
			Role:           "assistant",
			Content:        cfg.OpeningMessage,
		}); err != nil {
			log.Printf("[chat] save opening message failed tenant=%s conv=%s: %v", tenantID, c.ID, err)
		}
	}

	return c, nil
}

func (s *Service) Get(ctx context.Context, tenantID, userID, id string) (*Conversation, error) {
	return s.repo.FindConvOwned(tenantID, userID, id)
}

func (s *Service) List(ctx context.Context, tenantID, ownerID string, page, size int) ([]*Conversation, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return s.repo.ListConvs(tenantID, ownerID, page, size)
}

// Search returns conversations matching the query in title or message content.
func (s *Service) Search(ctx context.Context, tenantID, ownerID, query string, page, size int) ([]*Conversation, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return s.repo.SearchConvs(tenantID, ownerID, query, page, size)
}

// Rename updates a conversation's title. Only the owner can rename.
func (s *Service) Rename(ctx context.Context, tenantID, userID, id, title string) (*Conversation, error) {
	if title == "" {
		return nil, errs.BadRequest("title is required")
	}
	conv, err := s.repo.FindConvOwned(tenantID, userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateConvTitle(tenantID, id, title); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "rename conversation", err)
	}
	conv.Title = title
	return conv, nil
}

// SetPinned toggles the pinned flag on a conversation. Only the owner can pin.
func (s *Service) SetPinned(ctx context.Context, tenantID, userID, id string, pinned bool) (*Conversation, error) {
	conv, err := s.repo.FindConvOwned(tenantID, userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateConvPinned(tenantID, id, pinned); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "pin conversation", err)
	}
	conv.Pinned = pinned
	return conv, nil
}

// Export returns a conversation and its messages as a markdown document.
func (s *Service) Export(ctx context.Context, tenantID, userID, id string) (string, string, error) {
	conv, err := s.repo.FindConvOwned(tenantID, userID, id)
	if err != nil {
		return "", "", err
	}
	msgs, err := s.repo.ListMsgs(tenantID, id)
	if err != nil {
		return "", "", errs.Wrap(errs.CodeInternal, "list messages", err)
	}
	var b strings.Builder
	b.WriteString("# " + conv.Title + "\n\n")
	for _, m := range msgs {
		role := m.Role
		if role == RoleUser {
			role = "User"
		} else if role == RoleAssistant {
			role = "Assistant"
		}
		b.WriteString("## " + role + "\n\n")
		b.WriteString(m.Content + "\n\n")
	}
	return conv.Title, b.String(), nil
}

func (s *Service) ListMessages(ctx context.Context, tenantID, userID, convID string) ([]*Message, error) {
	// Verify the caller owns the conversation before listing messages.
	if _, err := s.repo.FindConvOwned(tenantID, userID, convID); err != nil {
		return nil, err
	}
	return s.repo.ListMsgs(tenantID, convID)
}

func (s *Service) Delete(ctx context.Context, tenantID, userID, id string) error {
	if err := s.repo.DeleteConv(tenantID, userID, id); err != nil {
		return err
	}
	if s.onConvDel != nil {
		s.onConvDel(tenantID, id)
	}
	return nil
}

// SendInput is the body for sending a user message and streaming the reply.
// Query is optional; when empty, the user's message is used as the query.
type SendInput struct {
	Message string `json:"message"`
	Query   string `json:"query"`
	TopK    int    `json:"top_k"`
}

// StreamReply is what each SSE event carries. Phase is one of: retrieve,
// generate, done, error, warning. Token is set during generate; Done closes
// the stream.
type StreamReply struct {
	Phase     string            `json:"phase"`
	Token     string            `json:"token,omitempty"`
	Citations []Citation        `json:"citations,omitempty"`
	MessageID string            `json:"message_id,omitempty"`
	Error     string            `json:"error,omitempty"`
	Warning   string            `json:"warning,omitempty"`
	Stats     *ReplyStats       `json:"stats,omitempty"`
	Trace     []agent.TraceStep `json:"trace,omitempty"`
}

// ReplyStats carries timing and token usage for a completed chat turn.
type ReplyStats struct {
	RetrieveMs       int `json:"retrieve_ms"`
	GenerateMs       int `json:"generate_ms"`
	TotalMs          int `json:"total_ms"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

const (
	PhaseRetrieve = "retrieve"
	PhaseThinking = "thinking"
	PhaseGenerate = "generate"
	PhaseDone     = "done"
	PhaseError    = "error"
	PhaseWarning  = "warning"
)

// Stream sends a user message, retrieves context, streams the assistant reply
// over the returned channel, and persists messages when done. The caller
// (handler) reads the channel and writes SSE events. Cancelling ctx aborts
// the LLM stream; the partial assistant message is still saved.
func (s *Service) Stream(ctx context.Context, tenantID, userID, convID string, in SendInput) (<-chan StreamReply, error) {
	conv, err := s.repo.FindConvOwned(tenantID, userID, convID)
	if err != nil {
		return nil, err
	}
	if in.Message == "" {
		return nil, errs.BadRequest("message is required")
	}

	// Enforce daily message quota before touching the LLM.
	if s.msgQuota != nil {
		if _, err := s.msgQuota.CheckMessageQuota(tenantID, userID); err != nil {
			return nil, err
		}
	}

	// Load the agent config to resolve the LLM provider for this KB.
	cfg := s.loadAgentConfig(ctx, tenantID, conv.KbID)

	// Resolve LLM provider: agent config's, else tenant default.
	provider, err := s.resolveProvider(ctx, tenantID, cfg.LLMModelID)
	if err != nil {
		return nil, err
	}

	// Persist the user message before streaming so the assistant reply can
	// reference it even if the stream is cancelled.
	userMsg := &Message{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		ConversationID: convID,
		Role:           RoleUser,
		Content:        in.Message,
	}
	if err := s.repo.CreateMsg(userMsg); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "save user message", err)
	}
	if err := s.repo.TouchConv(tenantID, convID); err != nil {
		log.Printf("[chat] touch conv failed tenant=%s conv=%s: %v", tenantID, convID, err)
	}

	// Bump the daily message counter after the message is accepted.
	if s.msgQuota != nil {
		if err := s.msgQuota.IncrMessageUsage(tenantID, userID); err != nil {
			log.Printf("[chat] incr msg quota failed tenant=%s: %v", tenantID, err)
		}
	}

	out := make(chan StreamReply, 16)
	go s.runStream(ctx, tenantID, conv, in, provider, userMsg, out, cfg)
	return out, nil
}

// TestStream runs the agent pipeline (retrieval + LLM streaming) without
// persisting anything. Used by the agent config test drawer so users can
// try different prompts/settings without creating throwaway conversations.
func (s *Service) TestStream(ctx context.Context, tenantID, userID, kbID, query string) (<-chan StreamReply, error) {
	if s.msgQuota != nil {
		if _, err := s.msgQuota.CheckMessageQuota(tenantID, userID); err != nil {
			return nil, err
		}
	}
	cfg := s.loadAgentConfig(ctx, tenantID, kbID)
	provider, err := s.resolveProvider(ctx, tenantID, cfg.LLMModelID)
	if err != nil {
		return nil, err
	}
	if s.msgQuota != nil {
		if err := s.msgQuota.IncrMessageUsage(tenantID, userID); err != nil {
			log.Printf("[chat] incr msg quota failed tenant=%s: %v", tenantID, err)
		}
	}
	out := make(chan StreamReply, 16)
	go s.testStream(ctx, tenantID, userID, kbID, query, provider, out, cfg)
	return out, nil
}

// loadAgentConfig returns the execution config for a KB. When no agent config
// fetcher is wired or the fetcher returns nil, DefaultExecutionConfig is used.
func (s *Service) loadAgentConfig(ctx context.Context, tenantID, kbID string) agent.ExecutionConfig {
	if s.agentCfg == nil {
		return agent.DefaultExecutionConfig()
	}
	cfg, err := s.agentCfg(ctx, tenantID, kbID)
	if err != nil || cfg == nil {
		return agent.DefaultExecutionConfig()
	}
	return *cfg
}

// loadAgentDefinition returns the full agent definition for a KB, or nil when
// no definition fetcher is wired or the fetcher returns nil/empty. The graph
// executor uses this to walk classifier/condition/message nodes.
func (s *Service) loadAgentDefinition(ctx context.Context, tenantID, kbID string) *agent.Definition {
	if s.agentDef == nil {
		return nil
	}
	def, err := s.agentDef(ctx, tenantID, kbID)
	if err != nil || def == nil || len(def.Nodes) == 0 {
		return nil
	}
	return def
}

// loadMemoryContext returns past conversation summaries for a KB. When no
// memory fetcher is wired or it errors, an empty string is returned so the
// chat flow proceeds without memory context.
func (s *Service) loadMemoryContext(ctx context.Context, tenantID, userID, kbID string) string {
	if s.memCtx == nil {
		return ""
	}
	memCtx, err := s.memCtx(ctx, tenantID, userID, kbID)
	if err != nil {
		return ""
	}
	return memCtx
}
