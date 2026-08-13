package webhook_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

func seedDurableWebhook(t *testing.T, receiverURL string) (*pgxpool.Pool, *repository.WebhookRepo, repository.Webhook) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool,
		"webhook_deliveries", "webhook_delivery_jobs", "webhook_outbox_events",
		"webhooks", "human_users", "participants",
	)
	ctx := context.Background()
	owner, err := repository.NewParticipantRepo(pool).CreateHuman(ctx, &models.HumanUser{
		Participant:  models.Participant{DisplayName: "Durable Webhook Owner"},
		Email:        fmt.Sprintf("durable-webhook-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "test-hash",
	})
	if err != nil {
		t.Fatalf("create webhook owner: %v", err)
	}
	repo := repository.NewWebhookRepo(pool)
	hook, err := repo.Create(ctx, owner.ID, receiverURL, "durable-secret", []string{webhook.EventPostCreated})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	return pool, repo, *hook
}

func immediateWorkerOptions() webhook.WorkerOptions {
	return webhook.WorkerOptions{
		Concurrency:  2,
		PollInterval: time.Millisecond,
		ClaimTTL:     time.Minute,
		MaxAttempts:  3,
		Backoff:      func(int) time.Duration { return 0 },
	}
}

func TestDurableWorkerRecoversQueuedEventsAndPreventsDuplicateDestinationClaims(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32
	var calls atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		current := active.Add(1)
		defer active.Add(-1)
		for {
			maximum := maxActive.Load()
			if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
				break
			}
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusAccepted)
	}))
	defer receiver.Close()

	pool, repo, hook := seedDurableWebhook(t, receiver.URL)
	ctx := context.Background()
	firstEventID, err := repo.Enqueue(ctx, webhook.EventPostCreated, map[string]any{"post_id": "first"})
	if err != nil {
		t.Fatalf("enqueue before worker startup: %v", err)
	}
	secondEventID, err := repo.Enqueue(ctx, webhook.EventPostCreated, map[string]any{"post_id": "second"})
	if err != nil {
		t.Fatalf("enqueue second event: %v", err)
	}
	if firstEventID == "" || secondEventID == "" || firstEventID == secondEventID {
		t.Fatalf("event IDs=%q and %q, want distinct durable IDs", firstEventID, secondEventID)
	}
	// Simulate a process dying after it claimed the first job but before any
	// network attempt or attempt record. A later worker must recover the lease.
	abandoned, err := repo.ClaimDelivery(ctx, time.Millisecond)
	if err != nil || abandoned == nil || abandoned.EventID != firstEventID {
		t.Fatalf("abandoned claim=%+v err=%v, want first queued event", abandoned, err)
	}
	time.Sleep(2 * time.Millisecond)

	// Constructing the workers only after the abandoned claim simulates restart.
	workerA := webhook.NewDispatcherWithClient(repo, receiver.Client())
	workerB := webhook.NewDispatcherWithClient(repo, receiver.Client())
	options := immediateWorkerOptions()
	firstResult := make(chan error, 1)
	go func() {
		processed, err := workerA.ProcessOne(ctx, options)
		if err == nil && !processed {
			err = fmt.Errorf("first worker did not claim queued delivery")
		}
		firstResult <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first worker did not reach receiver")
	}

	processed, err := workerB.ProcessOne(ctx, options)
	if err != nil {
		t.Fatalf("second worker claim: %v", err)
	}
	if processed {
		t.Fatal("second worker claimed another job for the same destination while its lease was active")
	}
	close(release)
	if err := <-firstResult; err != nil {
		t.Fatalf("first worker: %v", err)
	}

	processed, err = workerB.ProcessOne(ctx, options)
	if err != nil || !processed {
		t.Fatalf("second delivery after lease release processed=%t err=%v", processed, err)
	}
	if calls.Load() != 2 || maxActive.Load() != 1 {
		t.Fatalf("receiver calls=%d max_active=%d, want two serialized deliveries", calls.Load(), maxActive.Load())
	}

	deliveries, err := repo.ListDeliveries(ctx, hook.ID, 10, 0)
	if err != nil {
		t.Fatalf("list owner-visible delivery statuses: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("owner-visible deliveries=%d, want 2", len(deliveries))
	}
	for _, delivery := range deliveries {
		wantAttempts := 1
		if delivery.EventID == firstEventID {
			wantAttempts = 2
		}
		if delivery.Status != "succeeded" || delivery.AttemptCount != wantAttempts || delivery.Terminal || !delivery.Success {
			t.Errorf("completed delivery status=%+v", delivery)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin owner visibility transaction: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE app_user"); err != nil {
		t.Fatalf("assume app_user: %v", err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_user_id', $1, true)`, hook.ParticipantID); err != nil {
		t.Fatalf("set webhook owner context: %v", err)
	}
	var ownJobs, ownEvents int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_delivery_jobs`).Scan(&ownJobs); err != nil {
		t.Fatalf("owner query jobs: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_outbox_events`).Scan(&ownEvents); err != nil {
		t.Fatalf("owner query events: %v", err)
	}
	if ownJobs != 2 || ownEvents != 2 {
		t.Fatalf("owner visibility jobs=%d events=%d, want 2/2", ownJobs, ownEvents)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_user_id', $1, true)`, "00000000-0000-0000-0000-000000000001"); err != nil {
		t.Fatalf("set foreign participant context: %v", err)
	}
	var foreignJobs, foreignEvents int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_delivery_jobs`).Scan(&foreignJobs); err != nil {
		t.Fatalf("foreign query jobs: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_outbox_events`).Scan(&foreignEvents); err != nil {
		t.Fatalf("foreign query events: %v", err)
	}
	if foreignJobs != 0 || foreignEvents != 0 {
		t.Fatalf("foreign visibility jobs=%d events=%d, want 0/0", foreignJobs, foreignEvents)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_user_id', '', true)`); err != nil {
		t.Fatalf("clear request participant context: %v", err)
	}
	var anonymousJobs, anonymousEvents int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_delivery_jobs`).Scan(&anonymousJobs); err != nil {
		t.Fatalf("anonymous app_user query jobs: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_outbox_events`).Scan(&anonymousEvents); err != nil {
		t.Fatalf("anonymous app_user query events: %v", err)
	}
	if anonymousJobs != 0 || anonymousEvents != 0 {
		t.Fatalf("anonymous app_user visibility jobs=%d events=%d, want 0/0", anonymousJobs, anonymousEvents)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE app_service"); err != nil {
		t.Fatalf("assume webhook service role: %v", err)
	}
	var serviceJobs, serviceEvents int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_delivery_jobs`).Scan(&serviceJobs); err != nil {
		t.Fatalf("service query jobs: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM webhook_outbox_events`).Scan(&serviceEvents); err != nil {
		t.Fatalf("service query events: %v", err)
	}
	if serviceJobs != 2 || serviceEvents != 2 {
		t.Fatalf("service visibility jobs=%d events=%d, want 2/2", serviceJobs, serviceEvents)
	}
}

func TestDurableWorkerBoundsGlobalConcurrency(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			maximum := maxActive.Load()
			if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
				break
			}
		}
		started <- struct{}{}
		<-release
		w.WriteHeader(http.StatusAccepted)
	}))
	defer receiver.Close()

	pool, repo, hook := seedDurableWebhook(t, receiver.URL)
	for i := 0; i < 2; i++ {
		if _, err := repo.Create(context.Background(), hook.ParticipantID, receiver.URL,
			fmt.Sprintf("secret-%d", i), []string{webhook.EventPostCreated}); err != nil {
			t.Fatalf("create fanout hook %d: %v", i, err)
		}
	}
	if _, err := repo.Enqueue(context.Background(), webhook.EventPostCreated, map[string]any{"post_id": "fanout"}); err != nil {
		t.Fatalf("enqueue fanout event: %v", err)
	}

	worker := webhook.NewDispatcherWithClient(repo, receiver.Client())
	options := immediateWorkerOptions()
	options.Concurrency = 2
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(workerCtx, options)
		close(done)
	}()
	for i := 0; i < options.Concurrency; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatalf("worker started only %d/%d bounded requests", i, options.Concurrency)
		}
	}
	select {
	case <-started:
		t.Fatal("worker exceeded configured global concurrency")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	completed := 0
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(context.Background(), `
			SELECT COUNT(*) FROM webhook_delivery_jobs WHERE status = 'succeeded'`).Scan(&completed); err != nil {
			t.Fatalf("count bounded deliveries: %v", err)
		}
		if completed == 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed != 3 {
		t.Fatalf("completed bounded deliveries=%d, want 3", completed)
	}
	cancelWorker()
	<-done
	if maxActive.Load() != int32(options.Concurrency) {
		t.Fatalf("max active deliveries=%d, want %d", maxActive.Load(), options.Concurrency)
	}
}

func TestDurableWorkerRetriesWithIdenticalSignedEnvelopeAndExposesTerminalFailure(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	var signatures []string
	var eventIDs []string
	var responseStatus atomic.Int32
	responseStatus.Store(http.StatusServiceUnavailable)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read retry body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		mu.Lock()
		bodies = append(bodies, body)
		signatures = append(signatures, r.Header.Get("X-Loomfeed-Signature"))
		eventIDs = append(eventIDs, r.Header.Get("X-Loomfeed-Event-ID"))
		mu.Unlock()
		w.WriteHeader(int(responseStatus.Load()))
	}))
	defer receiver.Close()

	pool, repo, hook := seedDurableWebhook(t, receiver.URL)
	ctx := context.Background()
	eventID, err := repo.Enqueue(ctx, webhook.EventPostCreated, map[string]any{"post_id": "retry-me"})
	if err != nil {
		t.Fatalf("enqueue retry event: %v", err)
	}
	worker := webhook.NewDispatcherWithClient(repo, receiver.Client())
	options := immediateWorkerOptions()

	processed, err := worker.ProcessOne(ctx, options)
	if err != nil || !processed {
		t.Fatalf("first failed attempt processed=%t err=%v", processed, err)
	}
	responseStatus.Store(http.StatusNoContent)
	processed, err = worker.ProcessOne(ctx, options)
	if err != nil || !processed {
		t.Fatalf("retry attempt processed=%t err=%v", processed, err)
	}

	mu.Lock()
	if len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatalf("retry bodies=%q, want two byte-identical envelopes", bodies)
	}
	if signatures[0] != signatures[1] || eventIDs[0] != eventID || eventIDs[1] != eventID {
		t.Fatalf("retry signatures=%q event_ids=%q, want stable identity %q", signatures, eventIDs, eventID)
	}
	mac := hmac.New(sha256.New, []byte("durable-secret"))
	_, _ = mac.Write(bodies[0])
	wantSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if signatures[0] != wantSignature {
		t.Fatalf("signature=%q, want %q", signatures[0], wantSignature)
	}
	mu.Unlock()

	var attemptCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM webhook_deliveries WHERE event_id = $1`, eventID).Scan(&attemptCount); err != nil {
		t.Fatalf("count retry attempts: %v", err)
	}
	if attemptCount != 2 {
		t.Fatalf("retry attempt records=%d, want 2", attemptCount)
	}
	deliveries, err := repo.ListDeliveries(ctx, hook.ID, 10, 0)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("owner delivery status=%+v err=%v", deliveries, err)
	}
	if deliveries[0].Status != "succeeded" || deliveries[0].AttemptCount != 2 || deliveries[0].Terminal {
		t.Fatalf("retried delivery status=%+v", deliveries[0])
	}

	terminalEventID, err := repo.Enqueue(ctx, webhook.EventPostCreated, map[string]any{"post_id": "terminal"})
	if err != nil {
		t.Fatalf("enqueue terminal event: %v", err)
	}
	responseStatus.Store(http.StatusServiceUnavailable)
	options.MaxAttempts = 2
	for attempt := 0; attempt < options.MaxAttempts; attempt++ {
		processed, err = worker.ProcessOne(ctx, options)
		if err != nil || !processed {
			t.Fatalf("terminal attempt %d processed=%t err=%v", attempt+1, processed, err)
		}
	}
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM webhook_delivery_jobs WHERE event_id = $1`, terminalEventID).Scan(&status); err != nil {
		t.Fatalf("load terminal job: %v", err)
	}
	if status != "dead" {
		t.Fatalf("terminal job status=%q, want dead", status)
	}
	deliveries, err = repo.ListDeliveries(ctx, hook.ID, 10, 0)
	if err != nil || len(deliveries) != 2 {
		t.Fatalf("owner terminal statuses=%+v err=%v", deliveries, err)
	}
	if deliveries[0].EventID != terminalEventID || deliveries[0].Status != "dead" ||
		deliveries[0].AttemptCount != 2 || !deliveries[0].Terminal || deliveries[0].Success {
		t.Fatalf("terminal owner status=%+v", deliveries[0])
	}
}
