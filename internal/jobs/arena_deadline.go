package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/arenaevents"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

type arenaDeadlineStore interface {
	ProcessExpiredRounds(context.Context, time.Time, int) ([]repository.ArenaDeadlineTransition, error)
	GetBattle(context.Context, string) (*models.ArenaBattle, error)
	GetRoundByBattleAndNumber(context.Context, string, int) (*models.ArenaRound, error)
}

type arenaDeadlineDispatcher interface {
	Dispatch(string, map[string]any)
}

// ArenaDeadlineWorker advances battles even when participants or voters leave.
// Database row locks make it safe to run on every API replica.
type ArenaDeadlineWorker struct {
	store      arenaDeadlineStore
	dispatcher arenaDeadlineDispatcher
}

func NewArenaDeadlineWorker(pool *pgxpool.Pool) *ArenaDeadlineWorker {
	return newArenaDeadlineWorker(
		repository.NewArenaRepo(pool),
		webhook.NewDispatcher(repository.NewWebhookRepo(pool)),
	)
}

func newArenaDeadlineWorker(store arenaDeadlineStore, dispatcher arenaDeadlineDispatcher) *ArenaDeadlineWorker {
	return &ArenaDeadlineWorker{store: store, dispatcher: dispatcher}
}

// Sweep processes one earliest expired round per battle and emits the same
// lifecycle events as request-driven transitions.
func (w *ArenaDeadlineWorker) Sweep(ctx context.Context, now time.Time) (int, error) {
	transitions, err := w.store.ProcessExpiredRounds(ctx, now, 100)
	if err != nil {
		return 0, err
	}
	for _, transition := range transitions {
		if transition.OpenedRound > 0 {
			battle, battleErr := w.store.GetBattle(ctx, transition.BattleID)
			round, roundErr := w.store.GetRoundByBattleAndNumber(ctx, transition.BattleID, transition.OpenedRound)
			if battleErr != nil || roundErr != nil {
				slog.Warn("arena deadline: load opened round payload failed",
					"battle_id", transition.BattleID, "round", transition.OpenedRound,
					"battle_error", battleErr, "round_error", roundErr)
			} else {
				w.dispatcher.Dispatch(arenaevents.RoundOpened, arenaevents.RoundPayload(battle, round))
			}
		}
		if transition.Completed {
			battle, battleErr := w.store.GetBattle(ctx, transition.BattleID)
			if battleErr != nil {
				slog.Warn("arena deadline: load completed payload failed", "battle_id", transition.BattleID, "error", battleErr)
			} else {
				w.dispatcher.Dispatch(arenaevents.BattleCompleted, arenaevents.CompletedPayload(battle))
			}
		}
	}
	return len(transitions), nil
}

// Run performs an immediate sweep, then repeats until shutdown.
func (w *ArenaDeadlineWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	run := func() {
		processed, err := w.Sweep(ctx, time.Now().UTC())
		if err != nil {
			slog.Error("arena deadline sweep failed", "error", err)
		} else if processed > 0 {
			slog.Info("arena deadline sweep completed", "rounds_closed", processed)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
