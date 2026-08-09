package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api/handlers"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/testutil"
)

func setupDraftTest(t *testing.T) (*handlers.PostDraftHandler, *repository.ParticipantRepo, *config.Config) {
	t.Helper()
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "post_drafts", "human_users", "participants")
	participants := repository.NewParticipantRepo(pool)
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-secret-key-for-testing",
			Expiry: time.Hour,
		},
	}
	return handlers.NewPostDraftHandler(repository.NewPostDraftRepo(pool)), participants, cfg
}

func TestPostDraftHandler_CreateAndList(t *testing.T) {
	handler, participants, cfg := setupDraftTest(t)
	_, token := registerTestUser(t, participants, cfg, "drafter@example.com", "Drafter")

	protected := middleware.Auth(cfg.JWT.Secret)

	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/me/drafts", token, map[string]any{
		"post_type": "text",
		"title":     "Working title",
		"body":      "First pass at the body.",
		"tags":      []string{"draft", "wip"},
	})
	createRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Create)).ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)

	var created repository.PostDraft
	testutil.DecodeResponse(t, createRec, &created)
	if created.ID == "" {
		t.Fatal("expected draft id")
	}
	if created.Title != "Working title" {
		t.Errorf("title not persisted, got %q", created.Title)
	}
	if len(created.Tags) != 2 || created.Tags[0] != "draft" {
		t.Errorf("tags not persisted, got %v", created.Tags)
	}

	listReq := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/me/drafts", token, nil)
	listRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.List)).ServeHTTP(listRec, listReq)
	testutil.AssertStatus(t, listRec, http.StatusOK)

	var listResp struct {
		Drafts []repository.PostDraft `json:"drafts"`
	}
	testutil.DecodeResponse(t, listRec, &listResp)
	if len(listResp.Drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(listResp.Drafts))
	}
	if listResp.Drafts[0].ID != created.ID {
		t.Errorf("list returned wrong id")
	}
}

func TestPostDraftHandler_UpdateAndDelete(t *testing.T) {
	handler, participants, cfg := setupDraftTest(t)
	_, token := registerTestUser(t, participants, cfg, "draft-edit@example.com", "DraftEdit")

	protected := middleware.Auth(cfg.JWT.Secret)

	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/me/drafts", token, map[string]any{
		"post_type": "text",
		"title":     "v1",
	})
	createRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Create)).ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)
	var created repository.PostDraft
	testutil.DecodeResponse(t, createRec, &created)

	updateReq := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/me/drafts/"+created.ID, token, map[string]any{
		"post_type": "link",
		"title":     "v2",
		"url":       "https://example.com",
	})
	updateReq.SetPathValue("id", created.ID)
	updateRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Update)).ServeHTTP(updateRec, updateReq)
	testutil.AssertStatus(t, updateRec, http.StatusOK)
	var updated repository.PostDraft
	testutil.DecodeResponse(t, updateRec, &updated)
	if updated.Title != "v2" || updated.PostType != "link" || updated.URL != "https://example.com" {
		t.Errorf("update did not persist: %+v", updated)
	}

	delReq := testutil.JSONRequestWithAuth(t, http.MethodDelete, "/api/v1/me/drafts/"+created.ID, token, nil)
	delReq.SetPathValue("id", created.ID)
	delRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Delete)).ServeHTTP(delRec, delReq)
	testutil.AssertStatus(t, delRec, http.StatusOK)

	listReq := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/me/drafts", token, nil)
	listRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.List)).ServeHTTP(listRec, listReq)
	testutil.AssertStatus(t, listRec, http.StatusOK)
	var listResp struct {
		Drafts []repository.PostDraft `json:"drafts"`
	}
	testutil.DecodeResponse(t, listRec, &listResp)
	if len(listResp.Drafts) != 0 {
		t.Errorf("expected 0 drafts after delete, got %d", len(listResp.Drafts))
	}
}

func TestPostDraftHandler_OwnershipEnforced(t *testing.T) {
	handler, participants, cfg := setupDraftTest(t)
	_, aToken := registerTestUser(t, participants, cfg, "owner@example.com", "Owner")
	_, bToken := registerTestUser(t, participants, cfg, "stranger@example.com", "Stranger")

	protected := middleware.Auth(cfg.JWT.Secret)

	createReq := testutil.JSONRequestWithAuth(t, http.MethodPost, "/api/v1/me/drafts", aToken, map[string]any{
		"title": "private",
	})
	createRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Create)).ServeHTTP(createRec, createReq)
	testutil.AssertStatus(t, createRec, http.StatusCreated)
	var created repository.PostDraft
	testutil.DecodeResponse(t, createRec, &created)

	// stranger gets 404 (NOT 403 — we don't acknowledge the draft exists)
	getReq := testutil.JSONRequestWithAuth(t, http.MethodGet, "/api/v1/me/drafts/"+created.ID, bToken, nil)
	getReq.SetPathValue("id", created.ID)
	getRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Get)).ServeHTTP(getRec, getReq)
	testutil.AssertStatus(t, getRec, http.StatusNotFound)

	// stranger update also 404
	updateReq := testutil.JSONRequestWithAuth(t, http.MethodPut, "/api/v1/me/drafts/"+created.ID, bToken, map[string]any{
		"title": "hijack",
	})
	updateReq.SetPathValue("id", created.ID)
	updateRec := httptest.NewRecorder()
	protected(http.HandlerFunc(handler.Update)).ServeHTTP(updateRec, updateReq)
	testutil.AssertStatus(t, updateRec, http.StatusNotFound)
}
