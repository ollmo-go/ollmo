package doc

import (
	"strings"
	"testing"

	"ollmo/ollmo/pkg/chunker"
	"ollmo/ollmo/pkg/clients"
)

// markdown fixture mimicking MinerU output: three text items spread over two
// pages plus one table on the second page.
var pageFixture = struct {
	markdown string
	items    []clients.ContentItem
}{
	markdown: "# Intro\n\nFirst paragraph on page one.\n\nSecond paragraph on page one.\n\n<table><tr><td>row</td></tr></table>\n\nParagraph on page two.",
	items: []clients.ContentItem{
		{Type: "text", Text: "First paragraph on page one.", PageIdx: 0},
		{Type: "text", Text: "Second paragraph on page one.", PageIdx: 0},
		{Type: "table", TableBody: "<table><tr><td>row</td></tr></table>", PageIdx: 1},
		{Type: "text", Text: "Paragraph on page two.", PageIdx: 1},
	},
}

func TestAnchorPages(t *testing.T) {
	blocks := anchorPages(pageFixture.markdown, pageFixture.items)
	if len(blocks) != 4 {
		t.Fatalf("expected 4 anchored blocks, got %d", len(blocks))
	}
	for i, b := range blocks {
		if b.Page != pageFixture.items[i].PageIdx {
			t.Errorf("block %d: page=%d, want %d", i, b.Page, pageFixture.items[i].PageIdx)
		}
		if !strings.Contains(pageFixture.markdown[b.Start:b.End], itemAnchorText(pageFixture.items[i])) {
			t.Errorf("block %d range does not cover its anchor text", i)
		}
	}
}

func TestAnchorPagesSkipsUnmatchableItems(t *testing.T) {
	items := append(pageFixture.items, clients.ContentItem{Type: "text", Text: "not in markdown", PageIdx: 9})
	blocks := anchorPages(pageFixture.markdown, items)
	if len(blocks) != 4 {
		t.Fatalf("expected unmatchable item to be skipped, got %d blocks", len(blocks))
	}
}

func TestChunkPageNumbersParagraphStrategy(t *testing.T) {
	blocks := anchorPages(pageFixture.markdown, pageFixture.items)
	chunks := chunker.SplitMarkdown(pageFixture.markdown, chunker.StrategyParagraph, 80, 0)
	pages := chunkPageNumbers(pageFixture.markdown, blocks, chunks)

	if len(pages) != len(chunks) {
		t.Fatalf("got %d page strings for %d chunks", len(pages), len(chunks))
	}
	// With size=80 the fixture packs into two chunks: the first holds the
	// page-one intro, the second holds the table + page-two paragraph.
	found := map[string]bool{}
	for _, p := range pages {
		found[p] = true
	}
	if !found["1"] || !found["2"] {
		t.Errorf("expected page sets {1} and {2}, got %v", pages)
	}
}

func TestChunkPageNumbersSingleChunkSpansPages(t *testing.T) {
	blocks := anchorPages(pageFixture.markdown, pageFixture.items)
	chunks := chunker.SplitMarkdown(pageFixture.markdown, chunker.StrategyParagraph, 5000, 0)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	pages := chunkPageNumbers(pageFixture.markdown, blocks, chunks)
	if pages[0] != "1,2" {
		t.Errorf("chunk spanning both pages: got %q, want %q", pages[0], "1,2")
	}
}

func TestChunkPageNumbersNoBlocks(t *testing.T) {
	chunks := chunker.SplitMarkdown(pageFixture.markdown, chunker.StrategyParagraph, 200, 0)
	pages := chunkPageNumbers(pageFixture.markdown, nil, chunks)
	for i, p := range pages {
		if p != "" {
			t.Errorf("chunk %d: expected empty page string without blocks, got %q", i, p)
		}
	}
}

func TestJoinPagesCap(t *testing.T) {
	pages := make([]int, 50)
	for i := range pages {
		pages[i] = i + 1
	}
	got := joinPages(pages)
	if len(got) > 64 {
		t.Errorf("joinPages exceeded column width: len=%d", len(got))
	}
}

func TestLocateChunkOffset(t *testing.T) {
	md := pageFixture.markdown
	off := locateChunkOffset(md, "Second paragraph on page one.\n")
	if off < 0 || !strings.HasPrefix(md[off:], "Second paragraph on page one.") {
		t.Errorf("locateChunkOffset failed: got %d", off)
	}
	if locateChunkOffset(md, "not present anywhere") != -1 {
		t.Error("expected -1 for unmatchable chunk")
	}
}
