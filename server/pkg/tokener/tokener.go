// Package tokener provides a dependency-free token estimator for prompt
// budgeting. It deliberately overestimates (never underestimates beyond the
// documented margin) so callers can reserve a window with a small safety
// gap instead of exact counts.
//
// Accuracy: modern BPE encoders (cl100k/o200k) tokenize CJK text at roughly
// 0.6-1 token per character and Latin text at roughly 4 characters per token.
// Counting every CJK rune as 1 token and Latin/other as 1 token per 4 chars
// lands within ~20% on typical zh/en mixed RAG content, which is well inside
// the safety margin callers apply.
package tokener

import (
	"strings"
	"unicode"
)

// Estimate returns a conservative token count for s.
func Estimate(s string) int {
	if s == "" {
		return 0
	}
	tokens := 0
	latinChars := 0
	for _, r := range s {
		switch {
		case isCJK(r):
			tokens++
		case unicode.IsSpace(r):
			// Whitespace is consumed by the adjacent token in BPE.
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r):
			latinChars++
		default:
			latinChars++
		}
	}
	tokens += (latinChars + 3) / 4 // ceil(latinChars / 4)
	return tokens
}

// isCJK reports whether r is a CJK ideograph, CJK punctuation, fullwidth
// form, or kana — the codepoint ranges that tokenize near 1 token per rune.
func isCJK(r rune) bool {
	switch {
	case r >= 0x2E80 && r <= 0x9FFF: // CJK radicals, kana, CJK ideographs
		return true
	case r >= 0x3000 && r <= 0x303F: // CJK punctuation
		return true
	case r >= 0xF900 && r <= 0xFAFF: // CJK compatibility ideographs
		return true
	case r >= 0xFF00 && r <= 0xFFEF: // fullwidth forms
		return true
	case r >= 0x20000 && r <= 0x2FA1F: // CJK extensions
		return true
	}
	return false
}

// Truncate cuts s to at most maxTokens estimated tokens, appending an
// ellipsis when truncation occurs. maxTokens <= 0 yields an empty string.
func Truncate(s string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	runes := []rune(s)
	tokens := 0.0
	for i, r := range runes {
		if isCJK(r) {
			tokens++
		} else if !unicode.IsSpace(r) {
			tokens += 0.25
		}
		if tokens > float64(maxTokens) {
			return strings.TrimRight(string(runes[:i]), " \n\t") + "…"
		}
	}
	return s
}
