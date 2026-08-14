package chat

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// runSSE mounts writeSSE on a test app, runs produce against the reply
// channel, and returns the aggregated response body.
func runSSE(t *testing.T, produce func(ch chan<- StreamReply)) string {
	t.Helper()
	app := fiber.New()
	ch := make(chan StreamReply, 16)
	app.Get("/sse", func(c *fiber.Ctx) error {
		writeSSE(c, ch)
		return nil
	})
	go produce(ch)
	resp, err := app.Test(httptest.NewRequest("GET", "/sse", nil), 5000)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	return string(body)
}

// TestWriteSSE_DataEventsAndClose verifies each reply is written as one
// `data: <json>` SSE event and a closed channel terminates the stream.
func TestWriteSSE_DataEventsAndClose(t *testing.T) {
	body := runSSE(t, func(ch chan<- StreamReply) {
		ch <- StreamReply{Phase: PhaseRetrieve, Citations: []Citation{{DocID: "d1", DocName: "doc.md"}}}
		ch <- StreamReply{Phase: PhaseGenerate, Token: "hello"}
		close(ch)
	})
	if got := strings.Count(body, "data: "); got != 2 {
		t.Fatalf("got %d data events, want 2; body=%q", got, body)
	}
	if !strings.Contains(body, `"token":"hello"`) {
		t.Errorf("token event missing from body: %q", body)
	}
	if !strings.Contains(body, `"doc_name":"doc.md"`) {
		t.Errorf("citation missing from body: %q", body)
	}
	if strings.Contains(body, ": ping") {
		t.Errorf("no heartbeat expected when replies arrive instantly: %q", body)
	}
}

// TestWriteSSE_HeartbeatDuringSilence verifies comment heartbeats are sent
// while no reply is available, so intermediaries see connection activity
// during long silent gaps (e.g. waiting for the first LLM token).
func TestWriteSSE_HeartbeatDuringSilence(t *testing.T) {
	orig := sseHeartbeat
	sseHeartbeat = 50 * time.Millisecond
	t.Cleanup(func() { sseHeartbeat = orig })

	body := runSSE(t, func(ch chan<- StreamReply) {
		time.Sleep(200 * time.Millisecond) // silence: no first token yet
		ch <- StreamReply{Phase: PhaseGenerate, Token: "finally"}
		close(ch)
	})
	if n := strings.Count(body, ": ping"); n == 0 {
		t.Errorf("expected at least one heartbeat during silence, body=%q", body)
	}
	if !strings.Contains(body, `"token":"finally"`) {
		t.Errorf("reply after silence missing: %q", body)
	}
}

// TestWriteSSE_EmptyClose verifies an immediately closed channel ends the
// stream cleanly with an empty body.
func TestWriteSSE_EmptyClose(t *testing.T) {
	body := runSSE(t, func(ch chan<- StreamReply) { close(ch) })
	if body != "" {
		t.Errorf("expected empty body, got %q", body)
	}
}
