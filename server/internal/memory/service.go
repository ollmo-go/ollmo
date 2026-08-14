package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"ollmo/ollmo/internal/chat"
	"ollmo/ollmo/internal/llm"
	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/errs"
)

// AutoSummarizeThreshold is the number of messages after which a conversation
// is summarized automatically, and the growth increment that triggers a
// re-summary. E.g. summarize at 10 messages, then again at 20, 30, ...
const AutoSummarizeThreshold = 10

// Service generates and stores conversation summaries. The chat service calls
// BuildContext to load past summaries as supplementary system-prompt context.
type Service struct {
	repo     *Repo
	convRepo *chat.Repo
	llmRepo  *llm.Repo
	llm      *clients.LLMClient
	asynq    *asynq.Client
}

func NewService(repo *Repo, convRepo *chat.Repo, llmRepo *llm.Repo, llm *clients.LLMClient) *Service {
	return &Service{repo: repo, convRepo: convRepo, llmRepo: llmRepo, llm: llm}
}

// WithAsynq wires the task client used to enqueue auto-summarization jobs.
func (s *Service) WithAsynq(c *asynq.Client) *Service {
	s.asynq = c
	return s
}

// MaybeEnqueueAutoSummary checks whether the conversation has crossed the
// auto-summarization threshold since its last summary and, if so, enqueues an
// async summarize task. Rules:
//   - no memory yet: trigger when message count >= threshold
//   - memory exists: re-trigger when count has grown by another threshold
//
// Errors are logged, never returned — auto-memory must not break the chat
// flow. Safe to call on every chat turn; the check is one cheap COUNT query.
func (s *Service) MaybeEnqueueAutoSummary(tenantID, convID string) {
	if s.asynq == nil {
		return
	}
	count, err := s.convRepo.CountMsgs(tenantID, convID)
	if err != nil {
		log.Printf("[memory] count msgs failed tenant=%s conv=%s: %v", tenantID, convID, err)
		return
	}
	if count < AutoSummarizeThreshold {
		return
	}
	if existing, err := s.repo.FindByConversation(tenantID, convID); err == nil && existing != nil {
		// Already summarized; only re-summarize after another full batch.
		if int(count)-existing.MessageCount < AutoSummarizeThreshold {
			return
		}
	}
	task, err := NewSummarizeTask(SummarizePayload{TenantID: tenantID, ConversationID: convID})
	if err != nil {
		log.Printf("[memory] build summarize task failed conv=%s: %v", convID, err)
		return
	}
	if _, err := s.asynq.Enqueue(task); err != nil {
		log.Printf("[memory] enqueue summarize task failed conv=%s: %v", convID, err)
		return
	}
	log.Printf("[memory] auto-summary enqueued tenant=%s conv=%s msgs=%d", tenantID, convID, count)
}

// SummarizeConversation asks the LLM to summarize a conversation and stores
// the result as a Memory row. If a memory already exists for the conversation,
// it is replaced so re-summarizing after more messages produces a fresh
// summary. The caller must own the conversation.
func (s *Service) SummarizeConversation(ctx context.Context, tenantID, userID, convID string) (*Memory, error) {
	conv, err := s.convRepo.FindConvOwned(tenantID, userID, convID)
	if err != nil {
		return nil, err
	}
	return s.summarize(ctx, tenantID, conv, SourceManual)
}

// SummarizeByID summarizes a conversation without an ownership check. Used by
// the async auto-summarization worker, which runs outside any user request
// context and operates on IDs from the task payload.
func (s *Service) SummarizeByID(ctx context.Context, tenantID, convID string) (*Memory, error) {
	conv, err := s.convRepo.FindConv(tenantID, convID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeNotFound, "conversation not found", err)
	}
	return s.summarize(ctx, tenantID, conv, SourceAuto)
}

// summarize is the shared core for manual and auto summarization: load
// messages, call the LLM, and upsert the memory row.
func (s *Service) summarize(ctx context.Context, tenantID string, conv *chat.Conversation, source string) (*Memory, error) {
	msgs, err := s.convRepo.ListMsgs(tenantID, conv.ID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "list messages", err)
	}
	if len(msgs) < 2 {
		return nil, errs.BadRequest("conversation has too few messages to summarize")
	}

	provider, err := s.llmRepo.FindDefault(tenantID)
	if err != nil {
		return nil, errs.BadRequest("no default llm provider configured")
	}

	summary, keyPoints, err := s.callSummarizer(ctx, msgs, provider.Endpoint, provider.APIKey, provider.Model)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "summarize", err)
	}

	// Delete existing memory for this conversation so re-summarizing replaces.
	existing, _ := s.repo.FindByConversation(tenantID, conv.ID)
	if existing != nil {
		if err := s.repo.Delete(tenantID, conv.KbID, existing.ID); err != nil {
			log.Printf("[memory] delete existing failed tenant=%s conv=%s: %v", tenantID, conv.ID, err)
		}
	}

	kpJSON, _ := json.Marshal(keyPoints)
	m := &Memory{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		KbID:           conv.KbID,
		ConversationID: conv.ID,
		UserID:         conv.OwnerID,
		Type:           TypeEpisodic,
		Status:         StatusActive,
		Source:         source,
		MessageCount:   len(msgs),
		Title:          conv.Title,
		Summary:        summary,
		KeyPoints:      string(kpJSON),
	}
	if err := s.repo.Create(m); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "create memory", err)
	}
	return m, nil
}

// BuildContext returns a newline-separated summary block from the user's
// recent ACTIVE memories in a KB. The chat service appends this to the
// system prompt so the LLM has access to cross-session context without
// loading full message histories. Memories are per user: on shared KBs,
// other members' summaries are never injected. Disabled/forgotten memories
// are excluded.
func (s *Service) BuildContext(ctx context.Context, tenantID, userID, kbID string, limit int) (string, error) {
	if limit <= 0 {
		limit = 5
	}
	items, err := s.repo.ListActiveByKB(tenantID, userID, kbID, limit)
	if err != nil || len(items) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("Past conversation summaries (for context):\n")
	for _, m := range items {
		fmt.Fprintf(&b, "- %s: %s\n", m.Title, m.Summary)
	}
	return b.String(), nil
}

func (s *Service) List(tenantID, userID, kbID string, page, size int) ([]*Memory, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	return s.repo.ListByKB(tenantID, userID, kbID, page, size)
}

func (s *Service) Delete(tenantID, kbID, id string) error {
	return s.repo.Delete(tenantID, kbID, id)
}

// DeleteByConversation removes all memories belonging to a conversation.
// Wired as the chat service's conversation-deleted hook so deleting a
// conversation also removes its auto/manual summaries. Errors are logged,
// never returned — cleanup must not break the delete flow.
func (s *Service) DeleteByConversation(tenantID, convID string) {
	if err := s.repo.DeleteByConversation(tenantID, convID); err != nil {
		log.Printf("[memory] delete by conversation failed tenant=%s conv=%s: %v", tenantID, convID, err)
	}
}

// callSummarizer asks the LLM to produce a short summary and a list of key
// points from the conversation messages. Returns (summary, keyPoints, error).
func (s *Service) callSummarizer(ctx context.Context, msgs []*chat.Message, endpoint, apiKey, model string) (string, []string, error) {
	var b strings.Builder
	b.WriteString("Summarize the following conversation in 2-3 sentences. ")
	b.WriteString("Also list 3-5 key points as a JSON array of strings. ")
	b.WriteString("Return ONLY valid JSON: {\"summary\":\"...\",\"key_points\":[\"...\"]}\n\n")
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", m.Role, truncate(m.Content, 500))
	}

	req := clients.ChatRequest{
		Model: model,
		Messages: []clients.ChatMessage{
			{Role: "system", Content: "You are a summarization assistant that outputs only JSON."},
			{Role: "user", Content: b.String()},
		},
		Temperature: 0,
		MaxTokens:   512,
	}
	raw, err := s.llm.Chat(ctx, endpoint, apiKey, req)
	if err != nil {
		return "", nil, fmt.Errorf("llm summarize: %w", err)
	}

	var result struct {
		Summary   string   `json:"summary"`
		KeyPoints []string `json:"key_points"`
	}
	cleaned := stripFences(raw)
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		// Fallback: use the raw text as summary if JSON parsing fails.
		return truncate(raw, 500), nil, nil
	}
	if result.Summary == "" {
		result.Summary = truncate(raw, 500)
	}
	return result.Summary, result.KeyPoints, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func stripFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
