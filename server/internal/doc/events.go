package doc

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"ollmo/ollmo/internal/middleware"
)

// EventChannel is the Redis pub/sub channel relaying document events from
// the worker process to the API process.
const EventChannel = "doc:events"

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
// With Redis attached, Publish feeds the EventChannel pub/sub instead so
// events cross process boundaries: the worker publishes, and the API's
// RelayRedis goroutine fans them out to local SSE subscribers.
type EventBus struct {
	mu          sync.Mutex
	subscribers map[string][]chan DocEvent // keyed by tenantID
	rdb         *redis.Client
}

func NewEventBus() *EventBus {
	return &EventBus{subscribers: make(map[string][]chan DocEvent)}
}

// WithRedis enables cross-process delivery via Redis pub/sub.
func (b *EventBus) WithRedis(rdb *redis.Client) *EventBus {
	b.rdb = rdb
	return b
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
// With Redis attached the event travels the pub/sub channel only and reaches
// subscribers through RelayRedis, so it is delivered exactly once even when
// publisher and SSE clients live in different processes.
func (b *EventBus) Publish(event DocEvent) {
	if b.rdb != nil {
		data, err := json.Marshal(event)
		if err != nil {
			log.Printf("[doc] event marshal failed: %v", err)
			return
		}
		if err := b.rdb.Publish(context.Background(), EventChannel, data).Err(); err != nil {
			log.Printf("[doc] event publish to redis failed: %v", err)
		}
		return
	}
	b.publishLocal(event)
}

// RelayRedis subscribes to the event channel and fans received events out to
// local subscribers. Blocks until ctx is cancelled; run in a goroutine in
// the API process.
func (b *EventBus) RelayRedis(ctx context.Context) {
	if b.rdb == nil {
		return
	}
	sub := b.rdb.Subscribe(ctx, EventChannel)
	defer sub.Close()
	for msg := range sub.Channel() {
		var ev DocEvent
		if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
			continue
		}
		b.publishLocal(ev)
	}
}

func (b *EventBus) publishLocal(event DocEvent) {
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
				}
			}
		})
		return nil
	}
}
