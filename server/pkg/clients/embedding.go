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

// EmbeddingClient calls an OpenAI-compatible /v1/embeddings endpoint. This
// works with OpenAI, Infinity, Xinference, Ollama (with OpenAI adapter), and
// local vLLM/text-embeddings-inference servers. Only the wire format matters;
// model routing lives in the server config.
type EmbeddingClient struct {
	endpoint string
	apiKey   string
	http     *http.Client
}

func NewEmbedding(endpoint, apiKey string) *EmbeddingClient {
	return &EmbeddingClient{
		endpoint: endpoint,
		apiKey:   apiKey,
		http:     &http.Client{Timeout: 60 * time.Second},
	}
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed returns one vector per input text, preserving order. Batch size is
// controlled by the caller; this method does not chunk further.
func (c *EmbeddingClient) Embed(ctx context.Context, model string, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(embeddingRequest{Model: model, Input: inputs})
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding http %d: %s", resp.StatusCode, Snippet(b))
	}

	var out embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(out.Data) != len(inputs) {
		return nil, fmt.Errorf("embedding count mismatch: want %d got %d", len(inputs), len(out.Data))
	}

	// Server may return embeddings out of order; sort by index.
	vectors := make([][]float32, len(out.Data))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vectors) {
			return nil, fmt.Errorf("embedding index %d out of range", d.Index)
		}
		vectors[d.Index] = d.Embedding
	}
	return vectors, nil
}
