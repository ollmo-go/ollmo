package search

import (
	"sort"

	"ollmo/ollmo/pkg/vector"
)

// rrfList bundles a list of Milvus hits with its weight in the fusion.
type rrfList struct {
	hits   []vector.SearchHit
	weight float64
}

// rrfFusedHit is one chunk after fusion. Score is the RRF score, not the raw
// similarity, so it is comparable across retrieval legs.
type rrfFusedHit struct {
	ID      string
	DocID   string
	Content string
	Score   float64
}

// rrf applies Reciprocal Rank Fusion to N rank lists. The constant 60 is the
// standard RRF dampening factor; it prevents highly-ranked items in one list
// from dominating the merged order. Equal IDs across lists accumulate score,
// so a chunk retrieved by both dense and sparse ranks higher.
func rrf(lists []rrfList) []rrfFusedHit {
	const k = 60.0
	scores := make(map[string]float64)
	payload := make(map[string]rrfFusedHit)

	for _, list := range lists {
		for rank, h := range list.hits {
			score := list.weight / (k + float64(rank+1))
			scores[h.ID] += score
			if _, ok := payload[h.ID]; !ok {
				payload[h.ID] = rrfFusedHit{
					ID: h.ID, DocID: h.DocID, Content: h.Content,
				}
			}
		}
	}

	out := make([]rrfFusedHit, 0, len(payload))
	for id, p := range payload {
		p.Score = scores[id]
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
