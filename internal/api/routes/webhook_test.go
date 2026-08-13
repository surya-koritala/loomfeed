package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/surya-koritala/loomfeed/internal/arenaevents"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

func TestWebhookTestRouteDeliversOnlyToSelectedOwnedHook(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "webhook_deliveries", "webhook_delivery_jobs", "webhook_outbox_events", "webhooks", "api_keys", "agent_identities", "human_users", "participants")
	ctx := context.Background()

	type capturedRequest struct {
		body  []byte
		event webhook.Event
	}
	ownerRequests := make(chan capturedRequest, 1)
	ownerReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var event webhook.Event
		if err := json.Unmarshal(body, &event); err != nil {
			t.Errorf("decode owner webhook: %v", err)
		}
		ownerRequests <- capturedRequest{body: body, event: event}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ownerReceiver.Close()
	foreignRequests := make(chan struct{}, 1)
	foreignReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		foreignRequests <- struct{}{}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer foreignReceiver.Close()
	failingRequests := make(chan struct{}, 1)
	failingReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		failingRequests <- struct{}{}
		http.Error(w, "receiver unavailable", http.StatusInternalServerError)
	}))
	defer failingReceiver.Close()
	redirectTargetRequests := make(chan struct{}, 1)
	redirectReceiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			redirectTargetRequests <- struct{}{}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.Redirect(w, r, "/target", http.StatusFound)
	}))
	defer redirectReceiver.Close()

	participants := repository.NewParticipantRepo(pool)
	createHuman := func(label string) *models.Participant {
		t.Helper()
		participant, err := participants.CreateHuman(ctx, &models.HumanUser{
			Participant:  models.Participant{DisplayName: "Webhook " + label},
			Email:        fmt.Sprintf("webhook-route-%s-%d@example.com", label, time.Now().UnixNano()),
			PasswordHash: "test-hash",
		})
		if err != nil {
			t.Fatalf("create %s: %v", label, err)
		}
		return participant
	}
	owner := createHuman("owner")
	foreignOwner := createHuman("foreign")
	agent, err := participants.CreateAgent(ctx, &models.AgentIdentity{
		Participant: models.Participant{DisplayName: "Read Only Webhook Agent"},
		OwnerID:     owner.ID, ModelProvider: "test", ModelName: "test",
		ProtocolType: models.ProtocolREST, MaxRPM: 60,
	})
	if err != nil {
		t.Fatalf("create read-only agent: %v", err)
	}

	hooks := repository.NewWebhookRepo(pool)
	ownerHook, err := hooks.Create(ctx, owner.ID, ownerReceiver.URL, "owner-secret", []string{webhook.EventPostCreated})
	if err != nil {
		t.Fatalf("create owner webhook: %v", err)
	}
	foreignHook, err := hooks.Create(ctx, foreignOwner.ID, foreignReceiver.URL, "foreign-secret", []string{webhook.EventCommentCreated})
	if err != nil {
		t.Fatalf("create foreign webhook: %v", err)
	}
	failingHook, err := hooks.Create(ctx, owner.ID, failingReceiver.URL, "failing-secret", []string{webhook.EventPostCreated})
	if err != nil {
		t.Fatalf("create failing webhook: %v", err)
	}
	redirectHook, err := hooks.Create(ctx, owner.ID, redirectReceiver.URL, "redirect-secret", []string{webhook.EventPostCreated})
	if err != nil {
		t.Fatalf("create redirect webhook: %v", err)
	}

	const jwtSecret = "webhook-route-test-secret"
	ownerToken, err := auth.GenerateToken(jwtSecret, time.Hour, owner.ID, string(owner.Type))
	if err != nil {
		t.Fatalf("generate owner token: %v", err)
	}
	mux := http.NewServeMux()
	Register(mux, pool, &config.Config{JWT: config.JWTConfig{Secret: jwtSecret}}, registerOptions{
		disableBackgroundWorkers: true,
		webhookClient:            ownerReceiver.Client(),
	})

	callTest := func(webhookID string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID+"/test", strings.NewReader("{}"))
		request.Header.Set("Authorization", "Bearer "+ownerToken)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	readOnlyKey, readOnlyHash, readOnlyPrefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("generate read-only API key: %v", err)
	}
	if _, err := repository.NewAPIKeyRepo(pool).Create(ctx, &models.APIKey{
		AgentID: agent.ID, KeyHash: readOnlyHash, KeyPrefix: readOnlyPrefix,
		Scopes: []string{"read"}, RateLimit: 60, ExpiresAt: time.Now().Add(time.Hour), IsActive: true,
	}); err != nil {
		t.Fatalf("store read-only API key: %v", err)
	}
	readOnlyHook, err := hooks.Create(ctx, agent.ID, foreignReceiver.URL, "read-only-secret", []string{webhook.EventPostCreated})
	if err != nil {
		t.Fatalf("create read-only agent webhook: %v", err)
	}
	readOnlyRequest := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+readOnlyHook.ID+"/test", strings.NewReader("{}"))
	readOnlyRequest.Header.Set("X-API-Key", readOnlyKey)
	readOnlyResult := httptest.NewRecorder()
	mux.ServeHTTP(readOnlyResult, readOnlyRequest)
	if readOnlyResult.Code != http.StatusForbidden {
		t.Fatalf("read-only API key test status=%d body=%s", readOnlyResult.Code, readOnlyResult.Body.String())
	}

	unsupportedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(`{
		"url":"https://example.com/loomfeed-hook",
		"secret":"test-secret",
		"events":["webhook.test"]
	}`))
	unsupportedRequest.Header.Set("Authorization", "Bearer "+ownerToken)
	unsupportedRequest.Header.Set("Content-Type", "application/json")
	unsupportedResult := httptest.NewRecorder()
	mux.ServeHTTP(unsupportedResult, unsupportedRequest)
	if unsupportedResult.Code != http.StatusBadRequest {
		t.Fatalf("endpoint-only event registration status=%d body=%s", unsupportedResult.Code, unsupportedResult.Body.String())
	}

	ownerResult := callTest(ownerHook.ID)
	if ownerResult.Code != http.StatusOK {
		t.Fatalf("owned test status=%d body=%s", ownerResult.Code, ownerResult.Body.String())
	}
	var response struct {
		Status     string `json:"status"`
		EventID    string `json:"event_id"`
		StatusCode int    `json:"status_code"`
	}
	if err := json.Unmarshal(ownerResult.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	if response.Status != "delivered" || response.StatusCode != http.StatusAccepted {
		t.Fatalf("test response=%+v", response)
	}
	if _, err := uuid.Parse(response.EventID); err != nil {
		t.Fatalf("response event id=%q: %v", response.EventID, err)
	}

	select {
	case request := <-ownerRequests:
		if request.event.ID != response.EventID || request.event.Type != webhook.EventWebhookTest {
			t.Fatalf("delivered test event=%+v, response=%+v", request.event, response)
		}
		if request.event.Data["webhook_id"] != ownerHook.ID {
			t.Fatalf("test payload=%#v", request.event.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("owned webhook receiver was not contacted")
	}

	deliveries, err := hooks.ListDeliveries(ctx, ownerHook.ID, 10, 0)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("owned deliveries=%+v err=%v", deliveries, err)
	}
	if deliveries[0].EventID != response.EventID || deliveries[0].EventType != webhook.EventWebhookTest || !deliveries[0].Success {
		t.Fatalf("owned delivery=%+v", deliveries[0])
	}
	statusHook, err := hooks.Create(ctx, owner.ID, ownerReceiver.URL, "status-secret", []string{arenaevents.ChallengeCreated})
	if err != nil {
		t.Fatalf("create status webhook: %v", err)
	}
	queuedEventID, err := hooks.Enqueue(ctx, arenaevents.ChallengeCreated, map[string]any{"battle_id": "queued-status"})
	if err != nil {
		t.Fatalf("enqueue owner-visible status: %v", err)
	}
	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+statusHook.ID+"/deliveries", nil)
	statusRequest.Header.Set("Authorization", "Bearer "+ownerToken)
	statusResult := httptest.NewRecorder()
	mux.ServeHTTP(statusResult, statusRequest)
	if statusResult.Code != http.StatusOK {
		t.Fatalf("delivery status endpoint=%d body=%s", statusResult.Code, statusResult.Body.String())
	}
	var visibleStatuses []repository.WebhookDelivery
	if err := json.Unmarshal(statusResult.Body.Bytes(), &visibleStatuses); err != nil {
		t.Fatalf("decode owner-visible statuses: %v", err)
	}
	var queuedStatus *repository.WebhookDelivery
	for i := range visibleStatuses {
		if visibleStatuses[i].EventID == queuedEventID {
			queuedStatus = &visibleStatuses[i]
			break
		}
	}
	if queuedStatus == nil || queuedStatus.Status != "pending" || queuedStatus.AttemptCount != 0 || queuedStatus.Terminal {
		t.Fatalf("queued owner-visible status=%+v", queuedStatus)
	}

	foreignResult := callTest(foreignHook.ID)
	if foreignResult.Code != http.StatusNotFound {
		t.Fatalf("foreign test status=%d body=%s", foreignResult.Code, foreignResult.Body.String())
	}
	select {
	case <-foreignRequests:
		t.Fatal("foreign-owned webhook receiver was contacted")
	case <-time.After(100 * time.Millisecond):
	}
	foreignDeliveries, err := hooks.ListDeliveries(ctx, foreignHook.ID, 10, 0)
	if err != nil || len(foreignDeliveries) != 0 {
		t.Fatalf("foreign deliveries=%+v err=%v", foreignDeliveries, err)
	}

	failingResult := callTest(failingHook.ID)
	if failingResult.Code != http.StatusBadGateway {
		t.Fatalf("failed delivery status=%d body=%s", failingResult.Code, failingResult.Body.String())
	}
	var failureResponse struct {
		Status     string `json:"status"`
		EventID    string `json:"event_id"`
		StatusCode int    `json:"status_code"`
	}
	if err := json.Unmarshal(failingResult.Body.Bytes(), &failureResponse); err != nil {
		t.Fatalf("decode failure response: %v", err)
	}
	if failureResponse.Status != "failed" || failureResponse.StatusCode != http.StatusInternalServerError {
		t.Fatalf("failure response=%+v", failureResponse)
	}
	select {
	case <-failingRequests:
	case <-time.After(time.Second):
		t.Fatal("failing owned webhook receiver was not contacted")
	}
	failingDeliveries, err := hooks.ListDeliveries(ctx, failingHook.ID, 10, 0)
	if err != nil || len(failingDeliveries) != 1 {
		t.Fatalf("failing deliveries=%+v err=%v", failingDeliveries, err)
	}
	if failingDeliveries[0].EventID != failureResponse.EventID || failingDeliveries[0].Success || failingDeliveries[0].StatusCode != http.StatusInternalServerError {
		t.Fatalf("failing delivery=%+v", failingDeliveries[0])
	}

	redirectResult := callTest(redirectHook.ID)
	if redirectResult.Code != http.StatusBadGateway {
		t.Fatalf("redirect delivery status=%d body=%s", redirectResult.Code, redirectResult.Body.String())
	}
	select {
	case <-redirectTargetRequests:
		t.Fatal("webhook delivery followed a redirect and dropped its POST contract")
	case <-time.After(100 * time.Millisecond):
	}
	redirectDeliveries, err := hooks.ListDeliveries(ctx, redirectHook.ID, 10, 0)
	if err != nil || len(redirectDeliveries) != 1 || redirectDeliveries[0].Success || redirectDeliveries[0].StatusCode != http.StatusFound {
		t.Fatalf("redirect deliveries=%+v err=%v", redirectDeliveries, err)
	}

	for attempt := 2; attempt <= 10; attempt++ {
		result := callTest(failingHook.ID)
		if result.Code != http.StatusBadGateway {
			t.Fatalf("failed delivery attempt %d status=%d body=%s", attempt, result.Code, result.Body.String())
		}
		select {
		case <-failingRequests:
		case <-time.After(time.Second):
			t.Fatalf("failing receiver was not contacted for attempt %d", attempt)
		}
	}
	deactivated, err := hooks.GetByID(ctx, failingHook.ID)
	if err != nil || deactivated.IsActive || deactivated.FailureCount != 10 {
		t.Fatalf("deactivated webhook=%+v err=%v", deactivated, err)
	}
}
