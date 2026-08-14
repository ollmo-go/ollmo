package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"ollmo/ollmo/internal/search"
	"ollmo/ollmo/pkg/clients"
)

// buildPrompt assembles the system message (with retrieved context), the
// recent conversation history, and the current user query into the OpenAI
// chat messages array. excludeMsgID is skipped from history (the freshly
// persisted user message is re-added as the final query). graphContext is
// appended to the system message so the LLM sees entity relationships.
// memoryContext is appended so the LLM has cross-session context.
// customSystem overrides the default system prompt when non-empty, letting
// the agent canvas customize the assistant's behavior per KB.
func buildPrompt(systemContext, graphContext, memoryContext string, history []*Message, query, excludeMsgID, customSystem string) []clients.ChatMessage {
	msgs := make([]clients.ChatMessage, 0, len(history)+2)

	sys := customSystem
	if sys == "" {
		sys = "You are a helpful assistant. Answer the user's question using the provided context. " +
			"If the context does not contain the answer, say you don't know. " +
			"Cite sources as [doc_name] when you use context."
	}

	// Reserved variable substitution. {context} places retrieved chunks at
	// the user-chosen position in the system prompt; {query} exposes the
	// current question. When {context} is absent, chunks are appended to the
	// system prompt (backward compatible). User-defined variables, when added
	// later, will extend this substitution.
	contextPlaced := strings.Contains(sys, "{context}")
	sys = strings.ReplaceAll(sys, "{query}", query)
	if contextPlaced {
		sys = strings.ReplaceAll(sys, "{context}", systemContext)
	}
	if !contextPlaced && systemContext != "" {
		sys += "\n\nContext:\n" + systemContext
	}
	if graphContext != "" {
		sys += "\n\nKnowledge graph:\n" + graphContext
	}
	if memoryContext != "" {
		sys += "\n\n" + memoryContext
	}
	msgs = append(msgs, clients.ChatMessage{Role: RoleSystem, Content: sys})

	for _, m := range history {
		if m.Role != RoleUser && m.Role != RoleAssistant {
			continue
		}
		if m.ID == excludeMsgID {
			continue
		}
		msgs = append(msgs, clients.ChatMessage{Role: m.Role, Content: m.Content})
	}
	msgs = append(msgs, clients.ChatMessage{Role: RoleUser, Content: query})
	return msgs
}

// formatContext turns search hits into a numbered context block for the LLM.
// The numbers map to citation entries returned to the frontend.
func formatContext(hits []search.SearchHit) (string, []Citation) {
	if len(hits) == 0 {
		return "", nil
	}
	var b strings.Builder
	cits := make([]Citation, 0, len(hits))
	for i, h := range hits {
		fmt.Fprintf(&b, "[%d] (from %s)\n%s\n\n", i+1, h.DocName, h.Content)
		cits = append(cits, Citation{
			ChunkID:     h.ChunkID,
			DocID:       h.DocID,
			DocName:     h.DocName,
			Score:       float64(h.Score),
			Content:     h.Content,
			PageNumbers: h.PageNumbers,
		})
	}
	return strings.TrimRight(b.String(), "\n"), cits
}

// encodeCitations serializes citations for DB storage as JSON.
func encodeCitations(cits []Citation) string {
	if len(cits) == 0 {
		return ""
	}
	b, _ := json.Marshal(cits)
	return string(b)
}
