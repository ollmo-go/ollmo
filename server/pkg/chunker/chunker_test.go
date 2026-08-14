package chunker

import "testing"

func TestSplitQA(t *testing.T) {
	csv := "question,answer\n什么是 RAG?,检索增强生成\n如何部署?,docker compose up\n"
	chunks := SplitQA(csv)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(chunks), chunks)
	}
	want := "question: 什么是 RAG?\nanswer: 检索增强生成"
	if chunks[0] != want {
		t.Errorf("chunk 0: got %q, want %q", chunks[0], want)
	}
}

func TestSplitQAChineseHeader(t *testing.T) {
	csv := "问题,答案\n一加一等于几?,等于二\n"
	chunks := SplitQA(csv)
	if len(chunks) != 1 {
		t.Fatalf("expected header to be dropped, got %d chunks", len(chunks))
	}
}

func TestSplitQANoHeader(t *testing.T) {
	csv := "一加一等于几?,等于二\n天空是什么颜色?,蓝色\n"
	chunks := SplitQA(csv)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks without header drop, got %d", len(chunks))
	}
}

func TestSplitQASkipsBrokenRows(t *testing.T) {
	csv := "question,answer\n只有一列\n,答案缺问题\n问题缺答案,\n有效问题,有效答案\n"
	chunks := SplitQA(csv)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 usable chunk, got %d: %v", len(chunks), chunks)
	}
}

func TestSplitQAQuotedFields(t *testing.T) {
	csv := "question,answer\n\"含逗号的问题,真的\",\"含\n换行的答案\"\n"
	chunks := SplitQA(csv)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != "question: 含逗号的问题,真的\nanswer: 含\n换行的答案" {
		t.Errorf("quoted fields mishandled: %q", chunks[0])
	}
}

func TestSplitQAInvalidCSV(t *testing.T) {
	// Unterminated quote → reader error → nil so the caller falls back.
	if got := SplitQA("question,answer\n\"unterminated\n\"still broken"); got != nil {
		t.Errorf("expected nil for invalid csv, got %v", got)
	}
}
