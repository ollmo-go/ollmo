package doc

import (
	"bufio"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"ollmo/ollmo/internal/middleware"
)

// DocEvent is pushed to subscribers when a document's status changes.
type DocEvent struct {
	TenantID string    `json:"tenant_id"`
	DocID    string    `json:"doc_id"`
	KbID     string    `json:"kb_id"`
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
	At       time.Time `json:"at"`
}

// EventBus is an in-memory pub/sub for document status changes. Subscribers
// get a buffered channel; events are dropped if the channel is full to avoid
// blocking publishers (SSE clients can reconnect to get current state).
type EventBus struct {
	mu          sync.Mutex
	subscribers map[string][]chan DocEvent // keyed by tenantID
}

func NewEventBus() *EventBus {
	return &EventBus{subscribers: make(map[string][]chan DocEvent)}
}

// Subscribe returns a channel that receives doc events for the given tenant.
// The channel has a buffer of 32; events are dropped when full. Callers must
// call Unsubscribe when done to avoid leaking goroutines.
func (b *EventBus) Subscribe(tenantID string) chan DocEvent {
	ch := make(chan DocEvent, 32)
	b.mu.Lock()
	b.subscribers[tenantID] = append(b.subscribers[tenantID], ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (b *EventBus) Unsubscribe(tenantID string, ch chan DocEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subscribers[tenantID]
	for i, sub := range subs {
		if sub == ch {
			b.subscribers[tenantID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}

// Publish broadcasts an event to all subscribers for the event's tenant.
func (b *EventBus) Publish(event DocEvent) {
	b.mu.Lock()
	subs := b.subscribers[event.TenantID]
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Channel full: drop the event so a slow consumer does not block
			// the publisher. SSE clients can reconnect for current state.
		}
	}
}

// SSEHandler returns a fiber.HandlerFunc that streams document status events
// to the client via SSE. It subscribes to the event bus on connect and
// unsubscribes on disconnect.
func SSEHandler(bus *EventBus) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID := middleware.TenantID(c)

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
		c.Context().SetContentType("text/event-stream; charset=utf-8")

		ch := bus.Subscribe(tenantID)

		c.Context().Response.SetBodyStreamWriter(func(w *bufio.Writer) {
			defer bus.Unsubscribe(tenantID, ch)

			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case event, ok := <-ch:
					if !ok {
						return
					}
					b, err := json.Marshal(event)
					if err != nil {
						log.Printf("[doc] sse marshal failed: %v", err)
						continue
					}
					_, _ = w.WriteString("data: ")
					_, _ = w.Write(b)
					_, _ = w.WriteString("\n\n")
					if err := w.Flush(); err != nil {
						// Client went away.
						return
					}
				case <-ticker.C:
					_, _ = w.WriteString(": heartbeat\n\n")
					if err := w.Flush(); err != nil {
						return
					}
				case <-c.Context().Done():
					return
				}
			}
		})
		return nil
	}
}
