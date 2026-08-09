package repository_test

import (
	"context"
	"testing"

	"github.com/RoamXAI/loomfeed/internal/database"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// Phase 2.3 — GetHistoryFiltered backs the deep-dive /u/{id}/reputation
// page. Two paths: no filter returns everything newest-first, and the
// event_type param trims to a single class so the chip filters on the
// page don't paginate through irrelevant rows.

func TestReputationRepo_GetHistoryFiltered(t *testing.T) {
	pool := database.TestPool(t)
	database.CleanupTables(t, pool, "reputation_events", "participants")

	pRepo := repository.NewParticipantRepo(pool)
	repo := repository.NewReputationRepo(pool)
	ctx := context.Background()

	owner := createTestOwner(t, pRepo, ctx, "rephist-1")

	// Three events of two different classes. Insert in known order
	// (oldest first) so we can verify the newest-first ordering of
	// the response.
	events := []string{
		repository.EventVoteReceived,
		repository.EventVoteReceived,
		repository.EventPostSupported,
	}
	for _, et := range events {
		if err := repo.RecordEvent(ctx, owner.ID, et, 0); err != nil {
			t.Fatalf("RecordEvent(%s): %v", et, err)
		}
	}

	// Unfiltered — all three, newest first.
	all, err := repo.GetHistoryFiltered(ctx, owner.ID, "", 50)
	if err != nil {
		t.Fatalf("GetHistoryFiltered (all): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 events, got %d", len(all))
	}
	if all[0].EventType != repository.EventPostSupported {
		t.Errorf("expected newest event first (post_supported), got %q", all[0].EventType)
	}

	// Filtered to vote_received only — two rows, no post_supported.
	filtered, err := repo.GetHistoryFiltered(ctx, owner.ID, repository.EventVoteReceived, 50)
	if err != nil {
		t.Fatalf("GetHistoryFiltered (vote_received): %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 vote_received events, got %d", len(filtered))
	}
	for _, e := range filtered {
		if e.EventType != repository.EventVoteReceived {
			t.Errorf("filter leaked event_type %q into vote_received result", e.EventType)
		}
	}

	// Filtered to a class with no rows — empty slice (not nil), so
	// the JSON encoder serializes [] not null.
	none, err := repo.GetHistoryFiltered(ctx, owner.ID, repository.EventFlagUpheld, 50)
	if err != nil {
		t.Fatalf("GetHistoryFiltered (flag_upheld): %v", err)
	}
	if none == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(none) != 0 {
		t.Errorf("expected 0 flag_upheld events, got %d", len(none))
	}

	// Limit is honored.
	limited, err := repo.GetHistoryFiltered(ctx, owner.ID, "", 2)
	if err != nil {
		t.Fatalf("GetHistoryFiltered (limit=2): %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected limit of 2 to return 2 events, got %d", len(limited))
	}
}
