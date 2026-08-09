package events

import "sync"

// Event represents a server-sent event.
type Event struct {
	Type string
	Data string
}

// Hub manages SSE subscriptions and event publishing.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Event
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string][]chan Event),
	}
}

// Subscribe creates a new channel for the given participant and registers it.
func (h *Hub) Subscribe(participantID string) chan Event {
	ch := make(chan Event, 16)
	h.mu.Lock()
	h.subscribers[participantID] = append(h.subscribers[participantID], ch)
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a channel from the given participant's subscriptions.
func (h *Hub) Unsubscribe(participantID string, ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	channels := h.subscribers[participantID]
	updated := channels[:0]
	for _, c := range channels {
		if c != ch {
			updated = append(updated, c)
		}
	}
	if len(updated) == 0 {
		delete(h.subscribers, participantID)
	} else {
		h.subscribers[participantID] = updated
	}
	close(ch)
}

// Publish sends an event to all channels of the given participant.
func (h *Hub) Publish(participantID string, event Event) {
	h.mu.RLock()
	channels := h.subscribers[participantID]
	h.mu.RUnlock()
	for _, ch := range channels {
		select {
		case ch <- event:
		default:
			// drop if channel is full (non-blocking)
		}
	}
}

// Broadcast sends an event to all connected participants.
func (h *Hub) Broadcast(event Event) {
	h.mu.RLock()
	allChannels := make([]chan Event, 0)
	for _, channels := range h.subscribers {
		allChannels = append(allChannels, channels...)
	}
	h.mu.RUnlock()
	for _, ch := range allChannels {
		select {
		case ch <- event:
		default:
		}
	}
}
