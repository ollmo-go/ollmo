package search

import (
	"math"
	"testing"

	"ollmo/ollmo/pkg/vector"
)

// rrfK is the dampening constant used by the rrf function (see service.go).
const rrfK = 60.0

// hit is a test helper that builds a vector.SearchHit with the given id/docID
// and a monotonic score (the score itself is irrelevant to RRF, only the rank
// position matters).
func hit(id, docID string, score float32) vector.SearchHit {
	return vector.SearchHit{ID: id, DocID: docID, Content: "c-" + id, Score: score}
}

// ids extracts the ordered chunk IDs from fused hits so tests can assert on
// ranking without carrying floats.
func ids(fused []rrfFusedHit) []string {
	out := make([]string, 0, len(fused))
	for _, f := range fused {
		out = append(out, f.ID)
	}
	return out
}

// TestRRF_DenseOnly verifies that when only a dense list is provided, the
// fused output preserves the original dense ranking (no reordering).
func TestRRF_DenseOnly(t *testing.T) {
	dense := []vector.SearchHit{
		hit("d1", "docA", 0.9),
		hit("d2", "docA", 0.8),
		hit("d3", "docB", 0.7),
	}
	lists := []rrfList{{hits: dense, weight: 1.0}}

	fused := rrf(lists)

	if len(fused) != len(dense) {
		t.Fatalf("expected %d fused hits, got %d", len(dense), len(fused))
	}
	got := ids(fused)
	want := []string{"d1", "d2", "d3"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rank %d: want %s, got %s", i, want[i], got[i])
		}
	}
}

// TestRRF_SparseOnly verifies that when only a sparse list is provided, the
// fused output preserves the original sparse ranking.
func TestRRF_SparseOnly(t *testing.T) {
	sparse := []vector.SearchHit{
		hit("s1", "docA", 5.0),
		hit("s2", "docB", 4.0),
	}
	lists := []rrfList{{hits: sparse, weight: 1.0}}

	fused := rrf(lists)

	if len(fused) != len(sparse) {
		t.Fatalf("expected %d fused hits, got %d", len(sparse), len(fused))
	}
	got := ids(fused)
	want := []string{"s1", "s2"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rank %d: want %s, got %s", i, want[i], got[i])
		}
	}
}

// TestRRF_OverlappingDocs verifies that a chunk appearing in both dense and
// sparse lists accumulates RRF score and ranks above chunks from only one
// list.
func TestRRF_OverlappingDocs(t *testing.T) {
	// "shared" appears at rank 0 in dense and rank 1 in sparse, so it should
	// outrank "d_only" (rank 1 dense) and "s_only" (rank 0 sparse).
	dense := []vector.SearchHit{
		hit("shared", "docA", 0.95),
		hit("d_only", "docB", 0.80),
	}
	sparse := []vector.SearchHit{
		hit("s_only", "docC", 6.0),
		hit("shared", "docA", 5.0),
	}
	lists := []rrfList{
		{hits: dense, weight: 1.0},
		{hits: sparse, weight: 1.0},
	}

	fused := rrf(lists)

	if len(fused) != 3 {
		t.Fatalf("expected 3 fused hits, got %d", len(fused))
	}
	got := ids(fused)
	// "shared" accumulates 1/(60+1) + 1/(60+2) ≈ 0.03252, which must be the
	// highest. The other two each get a single contribution.
	if got[0] != "shared" {
		t.Errorf("expected shared to rank first, got order %v", got)
	}

	// Verify the shared hit's score is the sum of both legs.
	var sharedScore float64
	for _, f := range fused {
		if f.ID == "shared" {
			sharedScore = f.Score
		}
	}
	wantShared := 1.0/(rrfK+1) + 1.0/(rrfK+2)
	if math.Abs(sharedScore-wantShared) > 1e-9 {
		t.Errorf("shared score: want %.9f, got %.9f", wantShared, sharedScore)
	}
}

// TestRRF_Empty verifies that fusing no lists yields an empty result.
func TestRRF_Empty(t *testing.T) {
	fused := rrf(nil)
	if len(fused) != 0 {
		t.Errorf("expected empty fused result, got %d hits", len(fused))
	}

	fused = rrf([]rrfList{{hits: nil, weight: 1.0}})
	if len(fused) != 0 {
		t.Errorf("expected empty fused result for nil hits, got %d hits", len(fused))
	}
}

// TestRRF_MathCorrect verifies the RRF formula against a hand-computed input.
// For a single list with two hits at ranks 0 and 1:
//
//	score(rank 0) = 1 / (60 + 1) = 1/61
//	score(rank 1) = 1 / (60 + 2) = 1/62
//
// The fused output must be sorted descending by score.
func TestRRF_MathCorrect(t *testing.T) {
	dense := []vector.SearchHit{
		hit("a", "docA", 0.9),
		hit("b", "docA", 0.8),
	}
	lists := []rrfList{{hits: dense, weight: 1.0}}

	fused := rrf(lists)

	if len(fused) != 2 {
		t.Fatalf("expected 2 fused hits, got %d", len(fused))
	}

	wantA := 1.0 / (rrfK + 1)
	wantB := 1.0 / (rrfK + 2)

	var scoreA, scoreB float64
	for _, f := range fused {
		switch f.ID {
		case "a":
			scoreA = f.Score
		case "b":
			scoreB = f.Score
		}
	}

	if math.Abs(scoreA-wantA) > 1e-12 {
		t.Errorf("score a: want %.12f, got %.12f", wantA, scoreA)
	}
	if math.Abs(scoreB-wantB) > 1e-12 {
		t.Errorf("score b: want %.12f, got %.12f", wantB, scoreB)
	}
	// a (rank 0) must rank above b (rank 1).
	if fused[0].ID != "a" || fused[1].ID != "b" {
		t.Errorf("expected order [a, b], got %v", ids(fused))
	}
	if !(fused[0].Score > fused[1].Score) {
		t.Errorf("expected fused[0].Score > fused[1].Score, got %v > %v", fused[0].Score, fused[1].Score)
	}
}

// TestRRF_WeightScaling verifies that a list with a higher weight contributes
// more to the fused score. With dense weight 2.0 and sparse weight 1.0, a
// chunk that appears only in dense should outrank a chunk that appears only in
// sparse at the same rank.
func TestRRF_WeightScaling(t *testing.T) {
	dense := []vector.SearchHit{hit("d", "docA", 0.9)}
	sparse := []vector.SearchHit{hit("s", "docB", 5.0)}
	lists := []rrfList{
		{hits: dense, weight: 2.0},
		{hits: sparse, weight: 1.0},
	}

	fused := rrf(lists)

	if len(fused) != 2 {
		t.Fatalf("expected 2 fused hits, got %d", len(fused))
	}
	// d: 2/(60+1) ≈ 0.03279  >  s: 1/(60+1) ≈ 0.01639
	if fused[0].ID != "d" {
		t.Errorf("expected weighted dense hit to rank first, got %v", ids(fused))
	}
}

func TestNormalizeVectorWeight(t *testing.T) {
	cases := []struct {
		name          string
		in            *float64
		dense, sparse float64
	}{
		{"nil keeps equal weights", nil, 1.0, 1.0},
		{"0.7 biases dense", ptr(0.7), 0.7, 0.3},
		{"0 is sparse-only", ptr(0.0), 0.0, 1.0},
		{"1 is dense-only", ptr(1.0), 1.0, 0.0},
		{"negative clamped", ptr(-3), 0.0, 1.0},
		{"overflow clamped", ptr(7), 1.0, 0.0},
	}
	for _, tc := range cases {
		d, s := normalizeVectorWeight(tc.in)
		if math.Abs(d-tc.dense) > 1e-9 || math.Abs(s-tc.sparse) > 1e-9 {
			t.Errorf("%s: got (%v,%v), want (%v,%v)", tc.name, d, s, tc.dense, tc.sparse)
		}
	}
}

func ptr(f float64) *float64 { return &f }

// TestRRF_Dedup verifies that the same chunk ID appearing twice in one list
// does not double-count (only the first occurrence contributes). The rrf
// implementation uses a payload map keyed by ID, so the second occurrence is
// effectively ignored for scoring.
func TestRRF_Dedup(t *testing.T) {
	dense := []vector.SearchHit{
		hit("dup", "docA", 0.9),
		hit("dup", "docA", 0.5), // same ID, lower rank
		hit("other", "docB", 0.8),
	}
	lists := []rrfList{{hits: dense, weight: 1.0}}

	fused := rrf(lists)

	if len(fused) != 2 {
		t.Fatalf("expected 2 deduped fused hits, got %d", len(fused))
	}
	// "dup" at rank 0 scores 1/61; "other" at rank 2 scores 1/63, so dup wins.
	if fused[0].ID != "dup" {
		t.Errorf("expected dup first, got %v", ids(fused))
	}
}
