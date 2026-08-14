package clients

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// LLMClient calls an OpenAI-compatible /v1/chat/completions endpoint. This
// covers OpenAI, DeepSeek, Ollama (with OpenAI adapter), vLLM, Xinference,
// and any server that mirrors the OpenAI chat schema. Provider-specific
// quirks (Anthropic's native API, Bedrock, Vertex) are out of scope; route
// those through an OpenAI-compatible gateway.
type LLMClient struct {
	http *http.Client
}

func NewLLM() *LLMClient {
	return &LLMClient{http: &http.Client{Timeout: 5 * time.Minute}}
}

// ChatMessage is a single role/content pair. ReasoningContent carries the
// model's chain-of-thought from thinking-mode models (DeepSeek-R1, V4-Flash
// with reasoning_effort, Qwen-QwQ). It is omitempty so standard OpenAI
// requests/responses are unaffected.
type ChatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// TokenUsage holds prompt/completion/total token counts from the LLM API.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatRequest mirrors the OpenAI chat completion payload. Stream is set by the
// caller depending on whether they want Chat or ChatStream. ReasoningEffort
// enables thinking mode on supported models ("high" / "max"). Thinking
// explicitly enables/disables thinking for models like DeepSeek-V4-Flash
// that default to thinking on.
type ChatRequest struct {
	Model           string        `json:"model"`
	Messages        []ChatMessage `json:"messages"`
	Temperature     float64       `json:"temperature,omitempty"`
	MaxTokens       int           `json:"max_tokens,omitempty"`
	TopP            float64       `json:"top_p,omitempty"`
	Stream          bool          `json:"stream,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	Thinking        *ThinkingOpt  `json:"thinking,omitempty"`
	StreamOptions   *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

// ThinkingOpt controls DeepSeek-style thinking mode. Type is "enabled" or "disabled".
type ThinkingOpt struct {
	Type string `json:"type"`
}

type chatChoice struct {
	Message      ChatMessage `json:"message"`
	Delta        ChatMessage `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   *TokenUsage  `json:"usage,omitempty"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Chat does a non-streaming completion. Used for connection tests and for
// non-interactive summaries.
func (c *LLMClient) Chat(ctx context.Context, endpoint, apiKey string, req ChatRequest) (string, error) {
	req.Stream = false
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("llm http %d: %s", resp.StatusCode, rawBody)
	}

	log.Printf("[llm] response from %s model=%s status=%d body=%s", endpoint, req.Model, resp.StatusCode, string(rawBody))

	var out chatResponse
	if err := json.Unmarshal(rawBody, &out); err != nil {
		return "", fmt.Errorf("decode llm response: %w (raw: %s)", err, string(rawBody))
	}
	if out.Error != nil {
		return "", fmt.Errorf("llm error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices (raw: %s)", string(rawBody))
	}
	return out.Choices[0].Message.Content, nil
}

// ChatStreamDelta is one token-sized chunk emitted by ChatStream. When Done
// is true the stream is complete and Content/Err are zero values; when Err is
// non-nil the stream failed. Usage carries token counts from the final chunk.
// Reasoning carries chain-of-thought tokens from thinking-mode models.
type ChatStreamDelta struct {
	Content   string
	Reasoning string
	Done      bool
	Err       error
	Usage     *TokenUsage
}

// ChatStream opens a streaming chat completion and emits token deltas on the
// returned channel. The channel is closed when the stream ends (success or
// error); callers must read until close. Cancel ctx to abort early.
func (c *LLMClient) ChatStream(ctx context.Context, endpoint, apiKey string, req ChatRequest) <-chan ChatStreamDelta {
	out := make(chan ChatStreamDelta)
	req.Stream = true
	req.StreamOptions = &struct {
		IncludeUsage bool `json:"include_usage"`
	}{IncludeUsage: true}

	go func() {
		defer close(out)
		body, _ := json.Marshal(req)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			out <- ChatStreamDelta{Err: err}
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := c.http.Do(httpReq)
		if err != nil {
			out <- ChatStreamDelta{Err: fmt.Errorf("llm stream: %w", err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			out <- ChatStreamDelta{Err: fmt.Errorf("llm stream http %d: %s", resp.StatusCode, b)}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		// SSE chunks can be larger than the default 64k token limit.
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				out <- ChatStreamDelta{Done: true}
				return
			}
			var chunk chatResponse
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				continue
			}
			if chunk.Error != nil {
				out <- ChatStreamDelta{Err: fmt.Errorf("llm stream error: %s", chunk.Error.Message)}
				return
			}
			if len(chunk.Choices) > 0 {
				out <- ChatStreamDelta{
					Content:   chunk.Choices[0].Delta.Content,
					Reasoning: chunk.Choices[0].Delta.ReasoningContent,
				}
			}
			if chunk.Usage != nil {
				out <- ChatStreamDelta{Usage: chunk.Usage}
			}
		}
		if err := scanner.Err(); err != nil {
			out <- ChatStreamDelta{Err: fmt.Errorf("llm stream read: %w", err)}
			return
		}
		out <- ChatStreamDelta{Done: true}
	}()
	return out
}
