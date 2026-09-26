// Package live replays the dataset hour by hour on a simulated clock, re-runs the
// batch engine over the growing window and streams readings and alert changes.
//
// It lives beside the batch pipeline and does not touch it: the REST analysis keeps
// its own timings, and the stream only reads the same store.
package live

import "sync"

// Message is one server-sent event. ID grows monotonically so a client can resume
// with Last-Event-ID.
type Message struct {
	ID   uint64 `json:"id"`
	Kind string `json:"kind"` // tick | alert | control
	Data any    `json:"data"`
}

// Hub fans messages out to subscribers and keeps the last ones for resumption.
type Hub struct {
	mu     sync.Mutex
	subs   map[chan Message]struct{}
	ring   []Message
	size   int
	next   uint64
	closed bool
}

// NewHub keeps the last `size` messages for clients that reconnect.
func NewHub(size int) *Hub { return NewHubFrom(size, 0) }

// NewHubFrom numbers messages after `first`. Starting each process at its
// start time (in ms × 1000) makes ids of a previous process always older than
// the current buffer, so a client coming back after a restart gets a fresh
// snapshot instead of "resuming" from an unrelated id.
func NewHubFrom(size int, first uint64) *Hub {
	if size <= 0 {
		size = 512
	}
	return &Hub{subs: map[chan Message]struct{}{}, size: size, next: first}
}

// Close ends every subscription (their channels close, so SSE handlers return)
// and refuses new ones: call it when the server shuts down, so open streams do
// not hold the shutdown for its full timeout.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}

// Publish assigns the next id and delivers the message. A subscriber whose buffer is
// full is dropped (its channel closed): it reconnects and resumes from its last id.
func (h *Hub) Publish(kind string, data any) Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	m := Message{ID: h.next, Kind: kind, Data: data}
	h.ring = append(h.ring, m)
	if len(h.ring) > h.size {
		h.ring = h.ring[len(h.ring)-h.size:]
	}
	for ch := range h.subs {
		select {
		case ch <- m:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
	return m
}

// LastID is the id of the last published message (0 if none).
func (h *Hub) LastID() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.next
}

// Subscribe registers a subscriber. With lastID > 0 it also returns the messages
// published after it; resumed is false when they are no longer in the buffer (or
// lastID is 0), and the caller must then send a fresh snapshot instead.
func (h *Hub) Subscribe(lastID uint64) (ch chan Message, missed []Message, resumed bool, cancel func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch = make(chan Message, 256)
	if h.closed {
		close(ch)
		return ch, nil, true, func() {}
	}
	h.subs[ch] = struct{}{}
	oldest := h.next + 1 // with an empty buffer only the current id can resume
	if len(h.ring) > 0 {
		oldest = h.ring[0].ID
	}
	if lastID > 0 && lastID <= h.next && oldest <= lastID+1 {
		resumed = true
		for _, m := range h.ring {
			if m.ID > lastID {
				missed = append(missed, m)
			}
		}
	}
	cancel = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
	}
	return ch, missed, resumed, cancel
}

// Subscribers is the number of connected clients.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
