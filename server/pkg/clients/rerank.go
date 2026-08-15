package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RerankClient calls a Cohere-style rerank endpoint. Works with
// SiliconFlow, Cohere, Jina, and local text-embeddings-inference servers
// that expose the same API. The client returns documents reordered by
// relevance score, which is a cross-encoder signal denser than ANN cosine.
// The endpoint includes its version prefix (e.g. .../v1), matching the LLM
// client's convention.
type RerankClient struct {
	endpoint string
	apiKey   string
	http     *http.Client
}

func NewRerank(endpoint, apiKey string) *RerankClient {
	return &RerankClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

type rerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n,omitempty"`
	ReturnDocuments bool     `json:"return_documents"`
}

type rerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type rerankResponse struct {
	Results []rerankResult `json:"results"`
}

// RerankInput pairs a document's text with its original position in the
// caller's list so the caller can map scores back to domain objects.
type RerankInput struct {
	DocID   string
	Content string
}

// RerankOutput is one entry after reranking. Score is the cross-encoder
// relevance; higher is better.
type RerankOutput struct {
	DocID string
	Score float64
}

// Rerank returns documents sorted by cross-encoder relevance to the query.
// If the endpoint is empty or the call fails, the caller should fall back
// to the input order (handled by the search service).
func (c *RerankClient) Rerank(ctx context.Context, model, query string, docs []RerankInput, topN int) ([]RerankOutput, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	if topN <= 0 || topN > len(docs) {
		topN = len(docs)
	}

	docTexts := make([]string, len(docs))
	idByIndex := make([]string, len(docs))
	for i, d := range docs {
		docTexts[i] = d.Content
		idByIndex[i] = d.DocID
	}

	body, _ := json.Marshal(rerankRequest{
		Model:           model,
		Query:           query,
		Documents:       docTexts,
		TopN:            topN,
		ReturnDocuments: false,
	})
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/rerank", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rerank http %d: %s", resp.StatusCode, Snippet(b))
	}

	var out rerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w", err)
	}

	results := make([]RerankOutput, 0, len(out.Results))
	for _, r := range out.Results {
		if r.Index < 0 || r.Index >= len(idByIndex) {
			continue
		}
		results = append(results, RerankOutput{
			DocID: idByIndex[r.Index],
			Score: r.RelevanceScore,
		})
	}
	return results, nil
}
