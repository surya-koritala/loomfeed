package scorecard

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/events"
)

type Debouncer struct {
	mu     sync.Mutex
	window time.Duration
	last   map[string]time.Time
}

func NewDebouncer(window time.Duration) *Debouncer {
	return &Debouncer{window: window, last: make(map[string]time.Time)}
}

func (d *Debouncer) ShouldCompute(participantID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.last[participantID]; ok && time.Since(t) < d.window {
		return false
	}
	d.last[participantID] = time.Now()
	return true
}

type scorecardEvent struct {
	ParticipantID string `json:"participant_id"`
}

type Worker struct {
	pool      *pgxpool.Pool
	hub       *events.Hub
	debouncer *Debouncer
}

func NewWorker(pool *pgxpool.Pool, hub *events.Hub) *Worker {
	return &Worker{pool: pool, hub: hub, debouncer: NewDebouncer(60 * time.Second)}
}

// Run subscribes to scorecard trigger events and recomputes scores.
// Call in a goroutine.
func (w *Worker) Run(ctx context.Context) {
	ch := w.hub.Subscribe("__scorecard_worker__")
	defer w.hub.Unsubscribe("__scorecard_worker__", ch)
	slog.Info("scorecard worker started")

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			w.handleEvent(ctx, event)
		case <-ctx.Done():
			return
		}
	}
}

func (w *Worker) handleEvent(ctx context.Context, event events.Event) {
	if event.Type != "scorecard.trigger" {
		return
	}
	var payload scorecardEvent
	if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
		slog.Error("scorecard: invalid event payload", "error", err)
		return
	}
	if payload.ParticipantID == "" {
		return
	}
	if !w.debouncer.ShouldCompute(payload.ParticipantID) {
		return
	}
	sc, err := Compute(ctx, w.pool, payload.ParticipantID)
	if err != nil {
		slog.Error("scorecard: compute failed", "participant", payload.ParticipantID, "error", err)
		return
	}
	if err := Save(ctx, w.pool, sc); err != nil {
		slog.Error("scorecard: save failed", "participant", payload.ParticipantID, "error", err)
		return
	}
	slog.Info("scorecard: updated", "participant", payload.ParticipantID, "score", sc.CompositeScore, "tier", sc.Tier)
}

// TriggerCompute publishes a scorecard trigger event. Call from handlers after votes, etc.
func TriggerCompute(hub *events.Hub, participantID string) {
	data, _ := json.Marshal(scorecardEvent{ParticipantID: participantID})
	hub.Publish("__scorecard_worker__", events.Event{
		Type: "scorecard.trigger",
		Data: string(data),
	})
}
