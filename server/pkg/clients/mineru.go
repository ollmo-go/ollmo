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

// MinerUClient calls MinerU's REST API to parse documents. MinerU 3.x exposes
// an async task API at /api/v1/extract/task. This client is the only place
// that knows MinerU's wire format; if MinerU's API changes, only this file
// needs updating.
type MinerUClient struct {
	endpoint string
	http     *http.Client
}

func NewMinerU(endpoint string) *MinerUClient {
	return &MinerUClient{
		endpoint: endpoint,
		http:     &http.Client{Timeout: 10 * time.Minute},
	}
}

// ParseRequest is the payload sent to MinerU to start a parse task.
type ParseRequest struct {
	FileURL       string `json:"file_url"`
	Language      string `json:"language,omitempty"`
	EnableFormula bool   `json:"enable_formula"`
	EnableTable   bool   `json:"enable_table"`
	EnableOCR     bool   `json:"enableOCR"`
}

// ImageRef points to a parsed image stored by MinerU (page screenshot,
// figure, or table crop).
type ImageRef struct {
	Path string `json:"path"`
	Page int    `json:"page"`
	Type string `json:"type"`
}

// ParseResult is the parsed markdown content plus extracted images.
type ParseResult struct {
	Markdown string     `json:"markdown"`
	Images   []ImageRef `json:"images"`
}

type taskStatusResponse struct {
	TaskID   string      `json:"task_id"`
	State    string      `json:"state"`
	Progress int         `json:"progress"`
	Result   ParseResult `json:"result,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// SubmitParse starts an async parse task. Returns the task ID for polling.
func (c *MinerUClient) SubmitParse(ctx context.Context, req ParseRequest) (string, error) {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/extract/task", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("mineru submit %d: %s", resp.StatusCode, b)
	}

	var out struct {
		TaskID string `json:"task_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode mineru submit response: %w", err)
	}
	if out.TaskID == "" {
		return "", fmt.Errorf("mineru returned empty task_id")
	}
	return out.TaskID, nil
}

// GetTask fetches the current status of a parse task.
func (c *MinerUClient) GetTask(ctx context.Context, taskID string) (*taskStatusResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.endpoint+"/api/v1/extract/task/"+taskID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mineru status %d: %s", resp.StatusCode, b)
	}

	var out taskStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode mineru status: %w", err)
	}
	return &out, nil
}

// Wait blocks until the task reaches a terminal state (success|failed) or
// ctx is cancelled. poll defaults to 2s if non-positive.
func (c *MinerUClient) Wait(ctx context.Context, taskID string, poll time.Duration) (*ParseResult, error) {
	if poll <= 0 {
		poll = 2 * time.Second
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			status, err := c.GetTask(ctx, taskID)
			if err != nil {
				return nil, err
			}
			switch status.State {
			case "success":
				return &status.Result, nil
			case "failed":
				return nil, fmt.Errorf("mineru task %s failed: %s", taskID, status.Error)
			}
		}
	}
}
