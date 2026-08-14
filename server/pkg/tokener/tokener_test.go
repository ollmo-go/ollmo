package tokener

import (
	"strings"
	"testing"
)

func TestEstimate_MixedContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		min  int // conservative lower bound
		max  int // upper bound (must not overshoot a sane BPE count by much)
	}{
		{"empty", "", 0, 0},
		{"ascii word", "hello", 1, 2},
		{"ascii sentence", "the quick brown fox jumps over lazy dogs", 8, 11},
		{"cjk", "知识库问答助手", 7, 8},
		{"cjk punctuation", "你好，世界。", 6, 7},
		{"mixed", "使用 knowledge base 进行检索 retrieval", 10, 16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Estimate(tc.in)
			if got < tc.min || got > tc.max {
				t.Errorf("Estimate(%q) = %d, want [%d, %d]", tc.in, got, tc.min, tc.max)
			}
		})
	}
}

// TestEstimate_CJKHeavierThanLatin verifies the core asymmetry the budget
// logic relies on: CJK text costs more tokens per rune than Latin text.
func TestEstimate_CJKHeavierThanLatin(t *testing.T) {
	cjk := strings.Repeat("知", 100)
	latin := strings.Repeat("a", 100)
	if Estimate(cjk) <= Estimate(latin) {
		t.Errorf("Estimate(cjk)=%d must exceed Estimate(latin)=%d", Estimate(cjk), Estimate(latin))
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("", 10); got != "" {
		t.Errorf("Truncate empty = %q", got)
	}
	if got := Truncate("hello world", 0); got != "" {
		t.Errorf("Truncate zero budget = %q, want empty", got)
	}
	s := strings.Repeat("知", 100) // 100 estimated tokens
	got := Truncate(s, 20)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated text must end with ellipsis, got %q…", got[:len(got)-1])
	}
	if n := Estimate(strings.TrimSuffix(got, "…")); n > 21 {
		t.Errorf("truncated text estimates %d tokens, exceeds budget 20", n)
	}
	// Within budget: unchanged.
	if got := Truncate("short", 100); got != "short" {
		t.Errorf("Truncate within budget = %q, want unchanged", got)
	}
}
