// Package events provides an in-process broadcast hub for StackHost events.
package events

import (
	"sync"
	"time"
)

const DefaultBufferSize = 32

// Event is the payload delivered to every active subscriber.
type Event struct {
	ID    uint64    `json:"id"`
	Event string    `json:"event"`
	Data  any       `json:"data,omitempty"`
	At    time.Time `json:"at"`
}

// Publisher is the small interface consumers such as the deployment engine need.
type Publisher interface {
	Publish(event string, data any)
}

// PublisherFunc adapts a callback to Publisher.
type PublisherFunc func(event string, data any)

func (publish PublisherFunc) Publish(event string, data any) {
	if publish != nil {
		publish(event, data)
	}
}

// Hub broadcasts events to independent buffered subscriptions.
type Hub struct {
	mu          sync.Mutex
	bufferSize  int
	nextEventID uint64
	subscribers map[<-chan Event]chan Event
	closed      bool
}

// New creates a hub whose subscriptions each have the requested buffer size.
func New(bufferSize int) *Hub {
	if bufferSize < 1 {
		bufferSize = DefaultBufferSize
	}
	return &Hub{
		bufferSize:  bufferSize,
		subscribers: make(map[<-chan Event]chan Event),
	}
}

// Publish broadcasts one event without waiting for slow subscribers. When a
// subscription's buffer is full, only that subscription misses the new event.
func (h *Hub) Publish(name string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}

	h.nextEventID++
	event := Event{
		ID:    h.nextEventID,
		Event: name,
		Data:  data,
		At:    time.Now().UTC(),
	}
	for _, subscriber := range h.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

// Subscribe returns a new independent stream. A stream created after Close is
// already closed.
func (h *Hub) Subscribe() <-chan Event {
	h.mu.Lock()
	defer h.mu.Unlock()

	subscriber := make(chan Event, h.bufferSize)
	if h.closed {
		close(subscriber)
		return subscriber
	}
	h.subscribers[subscriber] = subscriber
	return subscriber
}

// Unsubscribe removes and closes a stream. It is safe to call more than once.
func (h *Hub) Unsubscribe(subscriber <-chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	stream, ok := h.subscribers[subscriber]
	if !ok {
		return
	}
	delete(h.subscribers, subscriber)
	close(stream)
}

// Close closes all subscriptions and rejects future publications. It is safe
// to call more than once.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}

	h.closed = true
	for subscriber, stream := range h.subscribers {
		delete(h.subscribers, subscriber)
		close(stream)
	}
}
