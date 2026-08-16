package doc

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"ollmo/ollmo/pkg/clients"
)

// pageBlock is a byte range in the parsed markdown that is known to live on
// one source page (0-based page_idx from MinerU).
type pageBlock struct {
	Start int `json:"start"`
	End   int `json:"end"`
	Page  int `json:"page"`
}

// anchorPages locates each MinerU content item inside the parsed markdown and
// records its byte range together with its page index. MinerU renders content
// items into the markdown verbatim, so text, table HTML, and image paths can
// be found by sequential search. Items that cannot be located are skipped;
// page attribution then falls back to the neighboring anchors.
func anchorPages(markdown string, items []clients.ContentItem) []pageBlock {
	var blocks []pageBlock
	cursor := 0
	for _, it := range items {
		anchor := itemAnchorText(it)
		if anchor == "" {
			continue
		}
		pos := strings.Index(markdown[cursor:], anchor)
		if pos < 0 {
			continue
		}
		pos += cursor
		blocks = append(blocks, pageBlock{Start: pos, End: pos + len(anchor), Page: it.PageIdx})
		cursor = pos + 1
	}
	return blocks
}

// itemAnchorText returns the substring of the parsed markdown that a content
// item is expected to appear as.
func itemAnchorText(it clients.ContentItem) string {
	switch it.Type {
	case "table":
		return strings.TrimSpace(it.TableBody)
	case "image":
		return strings.TrimSpace(it.ImgPath)
	default: // text, equation, and any unknown type carrying text
		return strings.TrimSpace(it.Text)
	}
}

// chunkPageNumbers maps each chunk back to the pages its content spans and
// returns a display string ("1" or "1,2", 1-based). Chunks are located in the
// markdown by their first line; the chunk's byte range is then overlapped
// with the anchored page blocks. Chunks with no anchors or no match get an
// empty string, which the UI simply hides.
func chunkPageNumbers(markdown string, blocks []pageBlock, chunks []string) []string {
	out := make([]string, len(chunks))
	if len(blocks) == 0 {
		return out
	}
	cursor := 0
	for i, c := range chunks {
		first := firstLine(c)
		if first == "" {
			continue
		}
		pos := strings.Index(markdown[cursor:], first)
		if pos < 0 {
			// Header-based strategies may join lines differently than the
			// markdown; retry from the start so earlier text cannot strand
			// later chunks.
			pos = strings.Index(markdown, first)
			if pos < 0 {
				continue
			}
		} else {
			pos += cursor
		}
		end := pos + len(c)
		if end > len(markdown) {
			end = len(markdown)
		}
		pages := pagesInRange(blocks, pos, end)
		if len(pages) > 0 {
			out[i] = joinPages(pages)
		}
		cursor = pos + 1
	}
	return out
}

// firstLine returns the first non-empty, non-marker line of a chunk.
func firstLine(chunk string) string {
	for _, line := range strings.Split(chunk, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return line
		}
	}
	return ""
}

// pagesInRange returns the sorted, deduplicated 1-based page numbers of all
// blocks overlapping [start, end).
func pagesInRange(blocks []pageBlock, start, end int) []int {
	seen := map[int]bool{}
	var pages []int
	for _, b := range blocks {
		if b.Start < end && b.End > start && !seen[b.Page] {
			seen[b.Page] = true
			pages = append(pages, b.Page+1)
		}
	}
	sort.Ints(pages)
	return pages
}

// joinPages renders page numbers as "1,2" capped to the column width (64).
func joinPages(pages []int) string {
	var parts []string
	n := 0
	for _, p := range pages {
		s := strconv.Itoa(p)
		if n > 0 {
			s = "," + s
		}
		if n+len(s) > 64 {
			break
		}
		n += len(s)
		parts = append(parts, strconv.Itoa(p))
	}
	return strings.Join(parts, ",")
}

// encodePageBlocks serializes page anchors for the documents.page_anchors
// column. Empty input yields "" so legacy rows keep the column NULL.
func encodePageBlocks(blocks []pageBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	b, err := json.Marshal(blocks)
	if err != nil {
		return ""
	}
	return string(b)
}

// decodePageBlocks parses the persisted page_anchors column. Malformed or
// empty values yield nil — the viewer simply hides page navigation.
func decodePageBlocks(s string) []pageBlock {
	if s == "" {
		return nil
	}
	var blocks []pageBlock
	if err := json.Unmarshal([]byte(s), &blocks); err != nil {
		return nil
	}
	return blocks
}

// locateChunkOffset finds a chunk's byte offset in the parsed markdown using
// the same first-line matching as chunkPageNumbers, so viewer highlighting is
// consistent with page attribution. Returns -1 when the chunk cannot be
// located (e.g. content edited after indexing).
func locateChunkOffset(markdown, chunkContent string) int {
	first := firstLine(chunkContent)
	if first == "" {
		return -1
	}
	pos := strings.Index(markdown, first)
	if pos < 0 {
		return -1
	}
	return pos
}
