package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	redisEventChannel   = "loomfeed:sse:events:v1"
	brokerQueueCapacity = 256
	brokerPublishLimit  = time.Second
)

// Event represents a server-sent event.
type Event struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type brokerEvent struct {
	Origin    string `json:"origin"`
	Key       string `json:"key,omitempty"`
	Broadcast bool   `json:"broadcast,omitempty"`
	Event     Event  `json:"event"`
}

// Hub manages SSE subscriptions and event publishing.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Event
	closed      bool

	origin      string
	redis       redis.UniversalClient
	pubsub      *redis.PubSub
	brokerQueue chan brokerEvent
	brokerCtx   context.Context
	brokerStop  context.CancelFunc
	brokerWG    sync.WaitGroup
	closeOnce   sync.Once
	closeErr    error
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string][]chan Event),
	}
}

// NewRedisHub creates a Hub whose events fan out through Redis Pub/Sub while
// preserving immediate delivery to subscribers in the publishing process.
func NewRedisHub(ctx context.Context, client redis.UniversalClient) (*Hub, error) {
	if client == nil {
		return nil, fmt.Errorf("redis event hub requires a client")
	}
	brokerCtx, stop := context.WithCancel(ctx)
	pubsub := client.Subscribe(brokerCtx, redisEventChannel)
	if _, err := pubsub.Receive(brokerCtx); err != nil {
		stop()
		_ = pubsub.Close()
		return nil, fmt.Errorf("subscribe Redis event hub: %w", err)
	}

	h := &Hub{
		subscribers: make(map[string][]chan Event),
		origin:      uuid.NewString(),
		redis:       client,
		pubsub:      pubsub,
		brokerQueue: make(chan brokerEvent, brokerQueueCapacity),
		brokerCtx:   brokerCtx,
		brokerStop:  stop,
	}
	messages := pubsub.Channel(redis.WithChannelSize(brokerQueueCapacity))
	h.brokerWG.Add(2)
	go h.publishBrokerEvents()
	go h.receiveBrokerEvents(messages)
	return h, nil
}

// Subscribe creates a new channel for the given participant and registers it.
// The receive-only return keeps channel send/close ownership inside the Hub.
func (h *Hub) Subscribe(participantID string) <-chan Event {
	ch := make(chan Event, 16)
	h.mu.Lock()
	if h.closed {
		close(ch)
		h.mu.Unlock()
		return ch
	}
	h.subscribers[participantID] = append(h.subscribers[participantID], ch)
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a channel from the given participant's subscriptions.
func (h *Hub) Unsubscribe(participantID string, ch <-chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	channels := h.subscribers[participantID]
	var removed chan Event
	for index, c := range channels {
		if c == ch {
			removed = c
			copy(channels[index:], channels[index+1:])
			channels[len(channels)-1] = nil
			channels = channels[:len(channels)-1]
			break
		}
	}
	if len(channels) == 0 {
		delete(h.subscribers, participantID)
	} else {
		h.subscribers[participantID] = channels
	}
	if removed != nil {
		close(removed)
	}
}

// Publish sends an event to all channels of the given participant.
func (h *Hub) Publish(participantID string, event Event) {
	h.publishLocal(participantID, event)
	h.enqueueBroker(brokerEvent{Origin: h.origin, Key: participantID, Event: event})
}

// PublishLocal sends an event only to subscribers in this process. It is for
// internal worker signals that must not run once per API replica.
func (h *Hub) PublishLocal(participantID string, event Event) {
	h.publishLocal(participantID, event)
}

func (h *Hub) publishLocal(participantID string, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	channels := h.subscribers[participantID]
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
	h.broadcastLocal(event)
	h.enqueueBroker(brokerEvent{Origin: h.origin, Broadcast: true, Event: event})
}

func (h *Hub) broadcastLocal(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, channels := range h.subscribers {
		for _, ch := range channels {
			select {
			case ch <- event:
			default:
			}
		}
	}
}

func (h *Hub) enqueueBroker(event brokerEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed || h.brokerQueue == nil {
		return
	}
	select {
	case h.brokerQueue <- event:
	default:
		slog.Debug("dropping cross-replica SSE event because broker queue is full", "event", event.Event.Type)
	}
}

func (h *Hub) publishBrokerEvents() {
	defer h.brokerWG.Done()
	for {
		select {
		case event := <-h.brokerQueue:
			payload, err := json.Marshal(event)
			if err != nil {
				slog.Warn("encode cross-replica SSE event", "error", err)
				continue
			}
			ctx, cancel := context.WithTimeout(h.brokerCtx, brokerPublishLimit)
			err = h.redis.Publish(ctx, redisEventChannel, payload).Err()
			cancel()
			if err != nil && h.brokerCtx.Err() == nil {
				slog.Warn("publish cross-replica SSE event", "error", err)
			}
		case <-h.brokerCtx.Done():
			return
		}
	}
}

func (h *Hub) receiveBrokerEvents(messages <-chan *redis.Message) {
	defer h.brokerWG.Done()
	for {
		select {
		case message, ok := <-messages:
			if !ok {
				return
			}
			var event brokerEvent
			if err := json.Unmarshal([]byte(message.Payload), &event); err != nil {
				slog.Warn("decode cross-replica SSE event", "error", err)
				continue
			}
			if event.Origin == h.origin {
				continue
			}
			if event.Broadcast {
				h.broadcastLocal(event.Event)
			} else {
				h.publishLocal(event.Key, event.Event)
			}
		case <-h.brokerCtx.Done():
			return
		}
	}
}

// Close stops broker fan-out and closes every subscriber channel. Hub is the
// sole channel closer, serialized against sends by the same mutex.
func (h *Hub) Close() error {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		for participantID, channels := range h.subscribers {
			for _, ch := range channels {
				close(ch)
			}
			delete(h.subscribers, participantID)
		}
		h.mu.Unlock()

		if h.brokerStop != nil {
			h.brokerStop()
		}
		if h.pubsub != nil {
			h.closeErr = h.pubsub.Close()
		}
		h.brokerWG.Wait()
	})
	return h.closeErr
}
