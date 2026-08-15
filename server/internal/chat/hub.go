package chat

import "sync"

// Hub fans out conversation stream events to every subscriber of that
// conversation (observer pattern). Stream publishes each reply to the
// conversation's subscribers so additional tabs/devices viewing the same
// chat receive the same live events as the sending client. In-memory only:
// subscribers connected to another server instance are not supported.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan StreamReply]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan StreamReply]struct{})}
}

// Subscribe registers a subscriber for a conversation and returns its
// receive channel. The channel is buffered; Publish never blocks on it.
func (h *Hub) Subscribe(convID string) chan StreamReply {
	ch := make(chan StreamReply, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[convID] == nil {
		h.subs[convID] = make(map[chan StreamReply]struct{})
	}
	h.subs[convID][ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscriber. The channel is deliberately not closed:
// a concurrent Publish may still hold a reference; once removed from the
// registry the channel is garbage as soon as the reader stops.
func (h *Hub) Unsubscribe(convID string, ch chan StreamReply) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.subs[convID]
	if !ok {
		return
	}
	delete(set, ch)
	if len(set) == 0 {
		delete(h.subs, convID)
	}
}

// Publish delivers r to every subscriber of convID. Non-blocking: a
// subscriber that has not drained its buffer misses that event rather than
// stalling the producer (live-tail semantics; clients refetch on done to
// resync with persisted state).
func (h *Hub) Publish(convID string, r StreamReply) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[convID] {
		select {
		case ch <- r:
		default:
		}
	}
}
