package chat

import (
	"strings"
	"testing"

	"ollmo/ollmo/internal/search"
	"ollmo/ollmo/pkg/clients"
	"ollmo/ollmo/pkg/tokener"
)

// mkHistory builds n chronological user/assistant messages of the given
// estimated token size each.
func mkHistory(n, tokensEach int) []*Message {
	out := make([]*Message, 0, n)
	for i := 0; i < n; i++ {
		role := RoleUser
		if i%2 == 1 {
			role = RoleAssistant
		}
		content := strings.Repeat("知", tokensEach)
		out = append(out, &Message{ID: string(rune('A' + i)), Role: role, Content: content})
	}
	return out
}

// TestBuildPrompt_NoBudgetKeepsEverything verifies budget <= 0 disables
// trimming (legacy behavior for defensive callers).
func TestBuildPrompt_NoBudgetKeepsEverything(t *testing.T) {
	hist := mkHistory(10, 50)
	msgs, trimmed := buildPrompt("", "", "", hist, "q", "", "", 0)
	if trimmed {
		t.Error("budget 0 must not report trimming")
	}
	if len(msgs) != len(hist)+2 { // system + history + query
		t.Errorf("got %d messages, want %d", len(msgs), len(hist)+2)
	}
}

// TestBuildPrompt_HistoryTrimmedNewestFirst verifies a tight budget keeps the
// newest messages and drops the oldest, preserving chronological order and
// conversational continuity (no middle gaps).
func TestBuildPrompt_HistoryTrimmedNewestFirst(t *testing.T) {
	hist := mkHistory(10, 50) // ~500 tokens total
	// Budget leaves room for system+query+~3 history messages.
	budget := tokener.Estimate(hist[9].Content)*3 + 64
	msgs, trimmed := buildPrompt("", "", "", hist, "q", "", "", budget)
	if !trimmed {
		t.Fatal("expected trimming to be reported")
	}
	kept := msgs[1 : len(msgs)-1] // strip system and final query
	if len(kept) < 1 || len(kept) > 4 {
		t.Fatalf("kept %d history messages, want 1-4", len(kept))
	}
	// Kept messages must be the NEWEST tail, in chronological order.
	wantFirst := hist[len(hist)-len(kept)]
	if kept[0].Content != wantFirst.Content {
		t.Errorf("oldest kept = %q, want the newest tail to survive", "msg")
	}
	// Total prompt must fit the budget (estimator-level check).
	if total := promptTokens(msgs); total > budget+32 {
		t.Errorf("prompt estimates %d tokens, exceeds budget %d", total, budget)
	}
}

// TestBuildPrompt_GraphMemoryTruncated verifies auxiliary context sections
// are cut to their shares when oversized.
func TestBuildPrompt_GraphMemoryTruncated(t *testing.T) {
	graph := strings.Repeat("实", 5000)  // ~5000 tokens
	memory := strings.Repeat("忆", 5000) // ~5000 tokens
	budget := 8192
	msgs, trimmed := buildPrompt("", graph, memory, nil, "q", "", "", budget)
	if !trimmed {
		t.Fatal("expected trimming of graph/memory")
	}
	sys := msgs[0].Content
	if n := tokener.Estimate(sys); n > budget/2 { // graph+memory shares are 10% each
		t.Errorf("system message estimates %d tokens, graph/memory shares exceeded", n)
	}
	if !strings.HasSuffix(sys, "…") && !strings.Contains(sys, "…") {
		t.Errorf("expected ellipsis marker after truncation")
	}
}

// TestBuildPrompt_ExcludeMsgID verifies the freshly persisted user message is
// skipped from history and re-added as the final query.
func TestBuildPrompt_ExcludeMsgID(t *testing.T) {
	hist := mkHistory(4, 10)
	msgs, _ := buildPrompt("", "", "", hist, "q", hist[3].ID, "", 0)
	if len(msgs) != 4+2-1 { // system + 3 history + query
		t.Errorf("got %d messages, want 5", len(msgs))
	}
	if msgs[len(msgs)-1].Role != RoleUser || msgs[len(msgs)-1].Content != "q" {
		t.Errorf("final message must be the query, got %+v", msgs[len(msgs)-1])
	}
}

// TestTrimHitsToBudget verifies lowest-ranked hits are dropped and the
// leading (highest-score) hits survive.
func TestTrimHitsToBudget(t *testing.T) {
	hits := make([]search.SearchHit, 5)
	for i := range hits {
		hits[i] = search.SearchHit{ChunkID: string(rune('a' + i)), DocName: "doc", Content: strings.Repeat("知", 100)}
	}
	got, trimmed := trimHitsToBudget(hits, 250) // each hit ~108 tokens
	if !trimmed {
		t.Fatal("expected trimming")
	}
	if len(got) != 2 {
		t.Errorf("kept %d hits, want 2 (highest-ranked)", len(got))
	}
	if got[0].ChunkID != "a" || got[1].ChunkID != "b" {
		t.Errorf("kept %s,%s; want a,b — ranking must be preserved", got[0].ChunkID, got[1].ChunkID)
	}
	// Budget <= 0 or empty input: passthrough.
	if h, tr := trimHitsToBudget(hits, 0); tr || len(h) != 5 {
		t.Errorf("zero budget must be a no-op")
	}
	if h, tr := trimHitsToBudget(nil, 100); tr || len(h) != 0 {
		t.Errorf("empty hits must be a no-op")
	}
}

// promptTokens sums the estimated tokens of a message list including the
// per-message overhead, mirroring what buildPrompt charges.
func promptTokens(msgs []clients.ChatMessage) int {
	total := 0
	for _, m := range msgs {
		total += tokener.Estimate(m.Content) + msgOverhead
	}
	return total
}
