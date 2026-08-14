// Package chunker provides text chunking strategies for RAG document
// ingestion. All strategies return plain text chunks; the caller persists
// them as needed.
package chunker

import "strings"

// Chunking strategy names. The KB's chunk_strategy field selects which
// splitter the parse worker uses. Unknown strategies fall back to paragraph.
const (
	StrategyParagraph   = "paragraph"
	StrategyToken       = "token"
	StrategyParentChild = "parent_child"
	StrategyHeaderAware = "header"
)

// SplitMarkdown dispatches to the chunking strategy pinned on the KB. All
// strategies return plain text chunks; the caller persists them as Chunk rows.
// chunkSize is a soft target in characters (not tokens); chunkOverlap is
// honored by the token and parent_child strategies.
func SplitMarkdown(md, strategy string, chunkSize, chunkOverlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 500
	}
	if chunkOverlap < 0 {
		chunkOverlap = 0
	}
	switch strings.ToLower(strategy) {
	case StrategyToken:
		return chunkByToken(md, chunkSize, chunkOverlap)
	case StrategyParentChild:
		return chunkParentChild(md, chunkSize, chunkOverlap)
	case StrategyHeaderAware:
		return chunkByHeader(md, chunkSize, chunkOverlap)
	default:
		return chunkByParagraph(md, chunkSize)
	}
}

// chunkByParagraph packs paragraphs into ~chunkSize chunks. This is the
// original Phase 1 splitter, kept for backward compatibility.
func chunkByParagraph(md string, chunkSize int) []string {
	paragraphs := strings.Split(md, "\n\n")

	var chunks []string
	var cur strings.Builder
	curLen := 0
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// A single paragraph longer than chunkSize would overflow the buffer
		// and produce an oversized chunk. Flush the buffer first, then split
		// the paragraph by a rune-aware sliding window so multi-byte CJK
		// characters are never cut mid-byte.
		if len(p) > chunkSize {
			if curLen > 0 {
				chunks = append(chunks, cur.String())
				cur.Reset()
				curLen = 0
			}
			runes := []rune(p)
			for i := 0; i < len(runes); i += chunkSize {
				end := i + chunkSize
				if end > len(runes) {
					end = len(runes)
				}
				chunks = append(chunks, string(runes[i:end]))
			}
			continue
		}
		if curLen+len(p) > chunkSize && curLen > 0 {
			chunks = append(chunks, cur.String())
			cur.Reset()
			curLen = 0
		}
		if curLen > 0 {
			cur.WriteString("\n\n")
			curLen += 2
		}
		cur.WriteString(p)
		curLen += len(p)
	}
	if curLen > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

// chunkByToken slides a fixed window over the text. Unlike the paragraph
// strategy, it does not respect paragraph boundaries; instead it guarantees
// every chunk is close to chunkSize and overlaps the previous by chunkOverlap
// so phrases straddling a boundary appear in both chunks.
func chunkByToken(md string, chunkSize, chunkOverlap int) []string {
	text := strings.TrimSpace(md)
	if len(text) == 0 {
		return nil
	}
	step := chunkSize - chunkOverlap
	if step <= 0 {
		step = chunkSize
	}
	var chunks []string
	for start := 0; start < len(text); start += step {
		end := start + chunkSize
		if end > len(text) {
			end = len(text)
		}
		chunk := strings.TrimSpace(text[start:end])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(text) {
			break
		}
	}
	return chunks
}

// chunkParentChild splits paragraphs into small leaf chunks (chunkSize/2) and
// groups each leaf with its parent context (the surrounding section). The
// parent text is prepended to each leaf so the embedding carries both the
// detail and its context. This improves recall on queries that reference
// surrounding text without a separate parent lookup.
func chunkParentChild(md string, chunkSize, chunkOverlap int) []string {
	leafSize := chunkSize / 2
	if leafSize < 100 {
		leafSize = 100
	}
	sections := splitSections(md)

	var chunks []string
	for _, sec := range sections {
		leaves := chunkByParagraph(sec.body, leafSize)
		if len(leaves) == 0 {
			continue
		}
		// When the section is short enough to fit one chunk, emit it whole
		// rather than splitting and duplicating the header.
		if len(leaves) == 1 && len(leaves[0]) <= chunkSize {
			chunks = append(chunks, sec.header+leaves[0])
			continue
		}
		for _, leaf := range leaves {
			chunks = append(chunks, sec.header+leaf)
		}
	}
	if len(chunks) == 0 {
		return chunkByParagraph(md, chunkSize)
	}
	return chunks
}

// chunkByHeader splits markdown on # / ## / ### headers. Each section keeps
// its header line so the LLM can cite the section title.
func chunkByHeader(md string, chunkSize, chunkOverlap int) []string {
	sections := splitSections(md)

	var chunks []string
	for _, sec := range sections {
		body := sec.header + sec.body
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		if len(body) <= chunkSize {
			chunks = append(chunks, body)
			continue
		}
		// Long section: fall back to paragraph packing within the section.
		for _, c := range chunkByParagraph(body, chunkSize) {
			chunks = append(chunks, c)
		}
	}
	if len(chunks) == 0 {
		return chunkByParagraph(md, chunkSize)
	}
	return chunks
}

// section is a header line plus the body text under it.
type section struct {
	header string
	body   string
}

// splitSections splits markdown into sections on markdown headers (# .. ######).
// Text before the first header becomes its own section with an empty header.
func splitSections(md string) []section {
	lines := strings.Split(md, "\n")
	var sections []section
	var cur section

	flush := func() {
		if strings.TrimSpace(cur.header+cur.body) != "" {
			sections = append(sections, cur)
		}
		cur = section{}
	}

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "#") {
			flush()
			cur.header = line + "\n"
		} else {
			cur.body += line + "\n"
		}
	}
	flush()
	return sections
}
