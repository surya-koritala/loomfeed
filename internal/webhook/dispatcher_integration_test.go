package webhook_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

type receivedWebhook struct {
	body    []byte
	headers http.Header
	event   webhook.Event
}

func TestDispatcherDeliversEverySupportedEventWithStableIdentity(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "webhook_deliveries", "webhook_delivery_jobs", "webhook_outbox_events", "webhooks", "human_users", "participants")
	ctx := context.Background()

	participants := repository.NewParticipantRepo(pool)
	owner, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant:  models.Participant{DisplayName: "Webhook Contract Owner"},
		Email:        fmt.Sprintf("webhook-contract-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "test-hash",
	})
	if err != nil {
		t.Fatalf("create webhook owner: %v", err)
	}

	events := webhook.SupportedEvents()
	if len(events) == 0 {
		t.Fatal("supported webhook event catalog is empty")
	}
	received := make(chan receivedWebhook, len(events)*2)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("read webhook body: %v", readErr)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var event webhook.Event
		if err := json.Unmarshal(body, &event); err != nil {
			t.Errorf("decode webhook body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- receivedWebhook{body: body, headers: r.Header.Clone(), event: event}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer receiver.Close()

	const secret = "webhook-contract-secret"
	hooks := repository.NewWebhookRepo(pool)
	hookA, err := hooks.Create(ctx, owner.ID, receiver.URL, secret, events)
	if err != nil {
		t.Fatalf("create first webhook: %v", err)
	}
	hookB, err := hooks.Create(ctx, owner.ID, receiver.URL, secret, events)
	if err != nil {
		t.Fatalf("create second webhook: %v", err)
	}

	dispatcher := webhook.NewDispatcherWithClient(hooks, receiver.Client())
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	workerDone := make(chan struct{})
	go func() {
		dispatcher.Run(workerCtx, webhook.WorkerOptions{
			Concurrency: 4, PollInterval: time.Millisecond,
			ClaimTTL: time.Minute, MaxAttempts: 3,
			Backoff: func(int) time.Duration { return 0 },
		})
		close(workerDone)
	}()
	defer func() {
		cancelWorker()
		<-workerDone
	}()
	for _, eventType := range events {
		dispatcher.Dispatch(eventType, map[string]any{"contract_event": eventType})
	}

	receivedByType := make(map[string][]receivedWebhook, len(events))
	receivedCount := 0
	deadline := time.After(5 * time.Second)
	for receivedCount < len(events)*2 {
		select {
		case request := <-received:
			receivedByType[request.event.Type] = append(receivedByType[request.event.Type], request)
			receivedCount++
		case <-deadline:
			t.Fatalf("received %d/%d supported event deliveries: %v", receivedCount, len(events)*2, receivedByType)
		}
	}

	eventIDs := make(map[string]struct{}, len(events))
	for _, eventType := range events {
		requests := receivedByType[eventType]
		if len(requests) != 2 {
			t.Fatalf("supported event %q deliveries=%d, want two", eventType, len(requests))
		}
		if requests[0].event.ID != requests[1].event.ID {
			t.Errorf("event %q fan-out IDs differ: %q and %q", eventType, requests[0].event.ID, requests[1].event.ID)
		}
		if _, err := uuid.Parse(requests[0].event.ID); err != nil {
			t.Errorf("event %q id %q is not a UUID: %v", eventType, requests[0].event.ID, err)
		}
		if _, duplicate := eventIDs[requests[0].event.ID]; duplicate {
			t.Errorf("event id %q was reused across dispatches", requests[0].event.ID)
		}
		eventIDs[requests[0].event.ID] = struct{}{}
		for _, request := range requests {
			if request.headers.Get("X-Loomfeed-Event") != eventType {
				t.Errorf("event header=%q, want %q", request.headers.Get("X-Loomfeed-Event"), eventType)
			}
			if request.headers.Get("X-Loomfeed-Event-ID") != request.event.ID {
				t.Errorf("event id header=%q, want %q", request.headers.Get("X-Loomfeed-Event-ID"), request.event.ID)
			}
			if got := request.event.Data["contract_event"]; got != eventType {
				t.Errorf("event %q payload marker=%v", eventType, got)
			}
			if _, err := time.Parse(time.RFC3339Nano, request.event.Timestamp); err != nil {
				t.Errorf("event %q timestamp %q: %v", eventType, request.event.Timestamp, err)
			}

			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write(request.body)
			wantSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
			if !hmac.Equal([]byte(request.headers.Get("X-Loomfeed-Signature")), []byte(wantSignature)) {
				t.Errorf("event %q signature does not match raw body", eventType)
			}
		}
	}

	assertDeliveries := func(hookID string) {
		t.Helper()
		var deliveries []repository.WebhookDelivery
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			deliveries, err = hooks.ListDeliveries(ctx, hookID, len(events)+1, 0)
			if err != nil {
				t.Fatalf("list deliveries: %v", err)
			}
			allSucceeded := len(deliveries) == len(events)
			for _, delivery := range deliveries {
				allSucceeded = allSucceeded && delivery.Status == "succeeded"
			}
			if allSucceeded {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if len(deliveries) != len(events) {
			t.Fatalf("delivery records=%d, want %d", len(deliveries), len(events))
		}
		for _, delivery := range deliveries {
			requests := receivedByType[delivery.EventType]
			if len(requests) != 2 {
				t.Errorf("unexpected delivery event type %q", delivery.EventType)
				continue
			}
			if delivery.EventID != requests[0].event.ID {
				t.Errorf("delivery event id=%q, want %q", delivery.EventID, requests[0].event.ID)
			}
			if delivery.WebhookID != hookID || !delivery.Success || delivery.StatusCode != http.StatusAccepted {
				t.Errorf("delivery record=%+v", delivery)
			}
			payload, ok := delivery.Payload.(map[string]any)
			if !ok || payload["contract_event"] != delivery.EventType {
				t.Errorf("delivery payload=%#v for %q", delivery.Payload, delivery.EventType)
			}
		}
	}
	assertDeliveries(hookA.ID)
	assertDeliveries(hookB.ID)
}

func TestConcurrentSuccessPreventsStaleFailureDeactivation(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "webhook_deliveries", "webhook_delivery_jobs", "webhook_outbox_events", "webhooks", "human_users", "participants")
	ctx := context.Background()

	participants := repository.NewParticipantRepo(pool)
	owner, err := participants.CreateHuman(ctx, &models.HumanUser{
		Participant: models.Participant{DisplayName: "Concurrent Webhook Owner"},
		Email:       fmt.Sprintf("webhook-concurrency-%d@example.com", time.Now().UnixNano()), PasswordHash: "test-hash",
	})
	if err != nil {
		t.Fatalf("create webhook owner: %v", err)
	}

	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event webhook.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Errorf("decode concurrent webhook: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ready <- struct{}{}
		<-release
		if event.Data["outcome"] == "failure" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer receiver.Close()

	hooks := repository.NewWebhookRepo(pool)
	hook, err := hooks.Create(ctx, owner.ID, receiver.URL, "concurrent-secret", []string{webhook.EventPostCreated})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE webhooks SET failure_count = 9 WHERE id = $1`, hook.ID); err != nil {
		t.Fatalf("seed failure count: %v", err)
	}

	dispatcher := webhook.NewDispatcherWithClient(hooks, receiver.Client())
	results := make(chan webhook.DeliveryResult, 2)
	errors := make(chan error, 2)
	for _, outcome := range []string{"failure", "success"} {
		outcome := outcome
		go func() {
			result, err := dispatcher.DeliverTo(ctx, *hook, webhook.EventPostCreated, map[string]any{"outcome": outcome})
			results <- result
			errors <- err
		}()
	}
	<-ready
	<-ready
	close(release)

	var successes, failures int
	for i := 0; i < 2; i++ {
		result := <-results
		if result.Success {
			successes++
		} else {
			failures++
		}
		if err := <-errors; err != nil {
			t.Fatalf("deliver concurrent outcome: %v", err)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent outcomes success=%d failure=%d", successes, failures)
	}

	stored, err := hooks.GetByID(ctx, hook.ID)
	if err != nil {
		t.Fatalf("load webhook after concurrent outcomes: %v", err)
	}
	if !stored.IsActive || (stored.FailureCount != 0 && stored.FailureCount != 1) {
		t.Fatalf("concurrent success/failure left stale state: %+v", stored)
	}
	deliveries, err := hooks.ListDeliveries(ctx, hook.ID, 10, 0)
	if err != nil || len(deliveries) != 2 {
		t.Fatalf("concurrent delivery records=%+v err=%v", deliveries, err)
	}
}
