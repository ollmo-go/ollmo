package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DiscoveredModel is one candidate from an OpenAI-compatible GET /models
// listing. ContextLength and MaxTokens are present only when the endpoint
// reports them (common gateway extensions such as context_window /
// max_output_tokens); 0 means unknown.
type DiscoveredModel struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	ContextLength int    `json:"context_length,omitempty"`
	MaxTokens     int    `json:"max_tokens,omitempty"`
}

// listingResponse is the OpenAI GET /models shape. Data is a pointer so a
// reply without a "data" array (an HTML login page in disguise, a different
// protocol) is distinguishable from a legitimately empty listing.
type listingResponse struct {
	Data *[]struct {
		ID              string      `json:"id"`
		Name            string      `json:"name"`
		DisplayName     string      `json:"display_name"`
		ContextWindow   json.Number `json:"context_window"`
		ContextLength   json.Number `json:"context_length"`
		MaxTokens       json.Number `json:"max_tokens"`
		MaxOutputTokens json.Number `json:"max_output_tokens"`
	} `json:"data"`
}

// positiveInt parses a listing field, returning 0 when absent or unusable.
func positiveInt(n json.Number) int {
	i, err := n.Int64()
	if err != nil || i <= 0 {
		return 0
	}
	return int(i)
}

// ListModels queries an OpenAI-compatible endpoint for the models it
// advertises, so the settings form can offer them as candidates instead of
// forcing hand entry. The reply is advisory only: nothing is persisted, and
// endpoints without a listing surface an error naming the URL that failed.
func ListModels(ctx context.Context, endpoint, apiKey string) ([]DiscoveredModel, error) {
	url := strings.TrimRight(endpoint, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		hint := ""
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			hint = "; check the API key"
		}
		return nil, fmt.Errorf("%s answered %d%s", url, resp.StatusCode, hint)
	}

	var listing listingResponse
	// A model listing is small; cap the read so a misrouted endpoint cannot
	// stream an unbounded body into memory.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&listing); err != nil {
		return nil, fmt.Errorf("%s did not answer with a model listing: %w", url, err)
	}
	if listing.Data == nil {
		return nil, fmt.Errorf("%s answered no \"data\" array; enter models manually", url)
	}
	models := make([]DiscoveredModel, 0, len(*listing.Data))
	for _, e := range *listing.Data {
		// Rows without a usable id are skipped rather than failing the whole
		// listing: one malformed entry should not hide a working endpoint's
		// other models.
		if id := strings.TrimSpace(e.ID); id != "" {
			models = append(models, DiscoveredModel{
				ID:            id,
				Name:          firstNonEmpty(e.Name, e.DisplayName),
				ContextLength: firstNonEmptyInt(positiveInt(e.ContextWindow), positiveInt(e.ContextLength)),
				MaxTokens:     firstNonEmptyInt(positiveInt(e.MaxOutputTokens), positiveInt(e.MaxTokens)),
			})
		}
	}
	return models, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmptyInt(vals ...int) int {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}
