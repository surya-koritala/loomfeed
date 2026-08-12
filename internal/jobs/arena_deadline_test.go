package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/surya-koritala/loomfeed/internal/arenaevents"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

type fakeArenaDeadlineStore struct {
	transitions []repository.ArenaDeadlineTransition
	battle      *models.ArenaBattle
	round       *models.ArenaRound
}

func (f *fakeArenaDeadlineStore) ProcessExpiredRounds(context.Context, time.Time, int) ([]repository.ArenaDeadlineTransition, error) {
	return f.transitions, nil
}

func (f *fakeArenaDeadlineStore) GetBattle(context.Context, string) (*models.ArenaBattle, error) {
	return f.battle, nil
}

func (f *fakeArenaDeadlineStore) GetRoundByBattleAndNumber(context.Context, string, int) (*models.ArenaRound, error) {
	return f.round, nil
}

type fakeArenaDeadlineDispatcher struct {
	events []string
}

func (f *fakeArenaDeadlineDispatcher) Dispatch(eventType string, _ map[string]any) {
	f.events = append(f.events, eventType)
}

func TestArenaDeadlineWorkerDispatchesTransitionEvents(t *testing.T) {
	store := &fakeArenaDeadlineStore{
		transitions: []repository.ArenaDeadlineTransition{
			{BattleID: "battle-1", ClosedRound: 1, OpenedRound: 2},
			{BattleID: "battle-2", ClosedRound: 3, Completed: true},
		},
		battle: &models.ArenaBattle{ID: "battle-1", Topic: "test"},
		round:  &models.ArenaRound{ID: "round-2", BattleID: "battle-1", RoundNumber: 2},
	}
	dispatcher := &fakeArenaDeadlineDispatcher{}
	worker := newArenaDeadlineWorker(store, dispatcher)

	processed, err := worker.Sweep(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed=%d, want 2", processed)
	}
	if len(dispatcher.events) != 2 || dispatcher.events[0] != arenaevents.RoundOpened || dispatcher.events[1] != arenaevents.BattleCompleted {
		t.Fatalf("unexpected dispatched events: %#v", dispatcher.events)
	}
}
