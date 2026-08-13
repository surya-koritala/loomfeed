package repository

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/arenaevents"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// ArenaRepo handles database operations for the Agent Arena feature.
type ArenaRepo struct {
	pool *pgxpool.Pool
}

// NewArenaRepo creates a new ArenaRepo.
func NewArenaRepo(pool *pgxpool.Pool) *ArenaRepo {
	return &ArenaRepo{pool: pool}
}

func loadArenaBattleEvent(ctx context.Context, db database.DBTX, id string) (*models.ArenaBattle, error) {
	var b models.ArenaBattle
	err := db.QueryRow(ctx, `
		SELECT ab.id, ab.topic, COALESCE(ab.description, ''),
		       ab.agent_a_id, COALESCE(pa.display_name, ''),
		       ab.agent_b_id, COALESCE(pb.display_name, ''),
		       ab.format, ab.status, ab.total_rounds, ab.current_round,
		       ab.round_time_limit, ab.word_limit, COALESCE(ab.rules, ''),
		       ab.trust_stake, ab.settled_stake, ab.stake_settled_at,
		       ab.winner_id, ab.voter_count, ab.created_by, ab.created_at, ab.completed_at
		FROM arena_battles ab
		LEFT JOIN participants pa ON pa.id = ab.agent_a_id
		LEFT JOIN participants pb ON pb.id = ab.agent_b_id
		WHERE ab.id = $1`, id).Scan(
		&b.ID, &b.Topic, &b.Description,
		&b.AgentAID, &b.AgentAName, &b.AgentBID, &b.AgentBName,
		&b.Format, &b.Status, &b.TotalRounds, &b.CurrentRound,
		&b.RoundTimeLimit, &b.WordLimit, &b.Rules,
		&b.TrustStake, &b.SettledStake, &b.StakeSettledAt,
		&b.WinnerID, &b.VoterCount, &b.CreatedBy, &b.CreatedAt, &b.CompletedAt,
	)
	return &b, err
}

func loadArenaRoundEvent(ctx context.Context, db database.DBTX, battleID string, roundNumber int) (*models.ArenaRound, error) {
	var round models.ArenaRound
	err := db.QueryRow(ctx, `
		SELECT id, battle_id, round_number, round_type, deadline, created_at
		FROM arena_rounds WHERE battle_id = $1 AND round_number = $2`, battleID, roundNumber).Scan(
		&round.ID, &round.BattleID, &round.RoundNumber, &round.RoundType,
		&round.Deadline, &round.CreatedAt,
	)
	return &round, err
}

// CreateBattle inserts a new arena battle.
func (r *ArenaRepo) CreateBattle(ctx context.Context, battle *models.ArenaBattle) (*models.ArenaBattle, error) {
	return createArenaBattle(ctx, r.pool, battle)
}

func createArenaBattle(ctx context.Context, db database.DBTX, battle *models.ArenaBattle) (*models.ArenaBattle, error) {
	if battle.TrustStake < 0 || math.IsNaN(battle.TrustStake) || math.IsInf(battle.TrustStake, 0) {
		return nil, fmt.Errorf("create arena battle: trust stake must be a finite non-negative number")
	}
	var b models.ArenaBattle
	err := db.QueryRow(ctx, `
		INSERT INTO arena_battles (topic, description, agent_a_id, agent_b_id, format, status,
		                           total_rounds, current_round, round_time_limit, word_limit,
		                           rules, trust_stake, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, topic, description, agent_a_id, agent_b_id, format, status,
		          total_rounds, current_round, round_time_limit, word_limit,
		          rules, trust_stake, settled_stake, stake_settled_at,
		          winner_id, voter_count, created_by, created_at, completed_at`,
		battle.Topic, battle.Description, battle.AgentAID, battle.AgentBID,
		battle.Format, models.ArenaStatusPending,
		battle.TotalRounds, 0, battle.RoundTimeLimit, battle.WordLimit,
		battle.Rules, battle.TrustStake, battle.CreatedBy,
	).Scan(
		&b.ID, &b.Topic, &b.Description, &b.AgentAID, &b.AgentBID,
		&b.Format, &b.Status,
		&b.TotalRounds, &b.CurrentRound, &b.RoundTimeLimit, &b.WordLimit,
		&b.Rules, &b.TrustStake, &b.SettledStake, &b.StakeSettledAt, &b.WinnerID, &b.VoterCount,
		&b.CreatedBy, &b.CreatedAt, &b.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create arena battle: %w", err)
	}
	return &b, nil
}

// CreateBattleWithRounds persists the complete challenge and its initial
// lifecycle events atomically, so no worker can observe a partial battle.
func (r *ArenaRepo) CreateBattleWithRounds(
	ctx context.Context,
	battle *models.ArenaBattle,
	rounds []models.ArenaRound,
) (*models.ArenaBattle, error) {
	if len(rounds) == 0 {
		return nil, fmt.Errorf("create arena challenge: at least one round is required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create arena challenge: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	created, err := createArenaBattle(ctx, tx, battle)
	if err != nil {
		return nil, err
	}
	for i := range rounds {
		rounds[i].BattleID = created.ID
		if err := tx.QueryRow(ctx, `
			INSERT INTO arena_rounds (battle_id, round_number, round_type, deadline)
			VALUES ($1, $2, $3, $4)
			RETURNING id, created_at`,
			rounds[i].BattleID, rounds[i].RoundNumber, rounds[i].RoundType, rounds[i].Deadline,
		).Scan(&rounds[i].ID, &rounds[i].CreatedAt); err != nil {
			return nil, fmt.Errorf("create arena round %d: %w", rounds[i].RoundNumber, err)
		}
	}
	fullBattle, err := loadArenaBattleEvent(ctx, tx, created.ID)
	if err != nil {
		return nil, fmt.Errorf("load arena challenge event: %w", err)
	}
	fullBattle.Rounds = rounds
	if _, err := enqueueWebhookEvent(ctx, tx, arenaevents.ChallengeCreated, arenaevents.ChallengePayload(fullBattle)); err != nil {
		return nil, fmt.Errorf("enqueue arena challenge: %w", err)
	}
	firstRound, err := loadArenaRoundEvent(ctx, tx, created.ID, 1)
	if err != nil {
		return nil, fmt.Errorf("load first arena round event: %w", err)
	}
	if _, err := enqueueWebhookEvent(ctx, tx, arenaevents.RoundOpened, arenaevents.RoundPayload(fullBattle, firstRound)); err != nil {
		return nil, fmt.Errorf("enqueue first arena round: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create arena challenge: %w", err)
	}
	return fullBattle, nil
}

// GetBattle returns a battle by ID with joined agent and creator names.
func (r *ArenaRepo) GetBattle(ctx context.Context, id string) (*models.ArenaBattle, error) {
	var b models.ArenaBattle
	err := r.pool.QueryRow(ctx, `
		SELECT ab.id, ab.topic, COALESCE(ab.description, '') as description,
		       ab.agent_a_id, COALESCE(pa.display_name, '') as agent_a_name,
		       ab.agent_b_id, COALESCE(pb.display_name, '') as agent_b_name,
		       ab.format, ab.status, ab.total_rounds, ab.current_round,
		       ab.round_time_limit, ab.word_limit, COALESCE(ab.rules, '') as rules,
		       ab.trust_stake, ab.settled_stake, ab.stake_settled_at,
		       ab.winner_id, ab.voter_count,
		       ab.created_by, COALESCE(pc.display_name, '') as created_by_name,
		       ab.created_at, ab.completed_at
		FROM arena_battles ab
		LEFT JOIN participants pa ON pa.id = ab.agent_a_id
		LEFT JOIN participants pb ON pb.id = ab.agent_b_id
		LEFT JOIN participants pc ON pc.id = ab.created_by
		WHERE ab.id = $1`,
		id,
	).Scan(
		&b.ID, &b.Topic, &b.Description,
		&b.AgentAID, &b.AgentAName,
		&b.AgentBID, &b.AgentBName,
		&b.Format, &b.Status, &b.TotalRounds, &b.CurrentRound,
		&b.RoundTimeLimit, &b.WordLimit, &b.Rules,
		&b.TrustStake, &b.SettledStake, &b.StakeSettledAt, &b.WinnerID, &b.VoterCount,
		&b.CreatedBy, &b.CreatedByName,
		&b.CreatedAt, &b.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get arena battle: %w", err)
	}
	return &b, nil
}

// ListBattles returns battles filtered by status, ordered by creation time descending.
func (r *ArenaRepo) ListBattles(ctx context.Context, status string, limit, offset int) ([]models.ArenaBattle, int, error) {
	// Build count query
	countQuery := `SELECT COUNT(*) FROM arena_battles`
	args := []any{}
	argIdx := 1

	if status != "" {
		countQuery += fmt.Sprintf(" WHERE status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count arena battles: %w", err)
	}

	// Build list query
	query := `
		SELECT ab.id, ab.topic, COALESCE(ab.description, '') as description,
		       ab.agent_a_id, COALESCE(pa.display_name, '') as agent_a_name,
		       ab.agent_b_id, COALESCE(pb.display_name, '') as agent_b_name,
		       ab.format, ab.status, ab.total_rounds, ab.current_round,
		       ab.round_time_limit, ab.word_limit, COALESCE(ab.rules, '') as rules,
		       ab.trust_stake, ab.settled_stake, ab.stake_settled_at,
		       ab.winner_id, ab.voter_count,
		       ab.created_by, COALESCE(pc.display_name, '') as created_by_name,
		       ab.created_at, ab.completed_at
		FROM arena_battles ab
		LEFT JOIN participants pa ON pa.id = ab.agent_a_id
		LEFT JOIN participants pb ON pb.id = ab.agent_b_id
		LEFT JOIN participants pc ON pc.id = ab.created_by`

	listArgs := []any{}
	listArgIdx := 1

	if status != "" {
		query += fmt.Sprintf(" WHERE ab.status = $%d", listArgIdx)
		listArgs = append(listArgs, status)
		listArgIdx++
	}

	query += fmt.Sprintf(" ORDER BY ab.created_at DESC LIMIT $%d OFFSET $%d", listArgIdx, listArgIdx+1)
	listArgs = append(listArgs, limit, offset)

	rows, err := r.pool.Query(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list arena battles: %w", err)
	}
	defer rows.Close()

	var battles []models.ArenaBattle
	for rows.Next() {
		var b models.ArenaBattle
		if err := rows.Scan(
			&b.ID, &b.Topic, &b.Description,
			&b.AgentAID, &b.AgentAName,
			&b.AgentBID, &b.AgentBName,
			&b.Format, &b.Status, &b.TotalRounds, &b.CurrentRound,
			&b.RoundTimeLimit, &b.WordLimit, &b.Rules,
			&b.TrustStake, &b.SettledStake, &b.StakeSettledAt, &b.WinnerID, &b.VoterCount,
			&b.CreatedBy, &b.CreatedByName,
			&b.CreatedAt, &b.CompletedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan arena battle: %w", err)
		}
		battles = append(battles, b)
	}
	return battles, total, rows.Err()
}

// UpdateBattleStatus updates the status and optionally the winner of a battle.
func (r *ArenaRepo) UpdateBattleStatus(ctx context.Context, id string, status models.ArenaStatus, winnerID *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin arena status update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var previousStatus models.ArenaStatus
	if err := tx.QueryRow(ctx, `SELECT status FROM arena_battles WHERE id = $1 FOR UPDATE`, id).Scan(&previousStatus); err != nil {
		return fmt.Errorf("lock arena status update: %w", err)
	}

	if status == models.ArenaStatusCompleted {
		result, updateErr := tx.Exec(ctx, `
			UPDATE arena_battles SET status = $1, winner_id = $2, completed_at = NOW()
			WHERE id = $3`,
			status, winnerID, id)
		if updateErr != nil {
			return fmt.Errorf("update arena battle status: %w", updateErr)
		}
		if result.RowsAffected() == 0 {
			return fmt.Errorf("update arena battle status: battle not found")
		}
		if err := settleArenaStakeTx(ctx, tx, id, winnerID); err != nil {
			return fmt.Errorf("settle arena battle stake: %w", err)
		}
		if previousStatus != models.ArenaStatusCompleted {
			battle, err := loadArenaBattleEvent(ctx, tx, id)
			if err != nil {
				return fmt.Errorf("load status-completed battle payload: %w", err)
			}
			if _, err := enqueueWebhookEvent(ctx, tx, arenaevents.BattleCompleted, arenaevents.CompletedPayload(battle)); err != nil {
				return fmt.Errorf("enqueue status-completed battle: %w", err)
			}
		}
	} else {
		result, updateErr := tx.Exec(ctx, `
			UPDATE arena_battles SET status = $1 WHERE id = $2`,
			status, id)
		if updateErr != nil {
			return fmt.Errorf("update arena battle status: %w", updateErr)
		}
		if result.RowsAffected() == 0 {
			return fmt.Errorf("update arena battle status: battle not found")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit arena status update: %w", err)
	}
	return nil
}

// settleArenaStakeTx applies exactly one zero-sum reputation transfer for a
// completed battle. The battle marker and both reputation events share the
// caller's transaction, so retries either observe a completed settlement or
// replay none of it.
func settleArenaStakeTx(ctx context.Context, tx pgx.Tx, battleID string, winnerID *string) error {
	var agentAID, agentBID string
	var requestedStake float64
	var settledAt *time.Time
	if err := tx.QueryRow(ctx, `
		SELECT agent_a_id, agent_b_id, trust_stake, stake_settled_at
		FROM arena_battles WHERE id = $1 FOR UPDATE`, battleID,
	).Scan(&agentAID, &agentBID, &requestedStake, &settledAt); err != nil {
		return fmt.Errorf("lock arena stake: %w", err)
	}
	if settledAt != nil {
		return nil
	}
	if requestedStake < 0 || math.IsNaN(requestedStake) || math.IsInf(requestedStake, 0) {
		return fmt.Errorf("invalid trust stake %v", requestedStake)
	}
	if winnerID != nil && *winnerID != agentAID && *winnerID != agentBID {
		return fmt.Errorf("winner %s is not a battle participant", *winnerID)
	}

	// Lock both balances in a stable order before writing either event. This
	// prevents opposing simultaneous battles from acquiring participant locks
	// in opposite orders.
	firstID, secondID := agentAID, agentBID
	if secondID < firstID {
		firstID, secondID = secondID, firstID
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM participants
		WHERE id IN ($1, $2)
		ORDER BY id
		FOR UPDATE`, firstID, secondID)
	if err != nil {
		return fmt.Errorf("lock arena participant balances: %w", err)
	}
	locked := 0
	for rows.Next() {
		var participantID string
		if err := rows.Scan(&participantID); err != nil {
			rows.Close()
			return fmt.Errorf("scan arena participant lock: %w", err)
		}
		locked++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate arena participant locks: %w", err)
	}
	rows.Close()
	if locked != 2 {
		return fmt.Errorf("lock arena participant balances: expected 2 participants, got %d", locked)
	}

	settledStake := 0.0
	if requestedStake > 0 {
		if winnerID == nil {
			if _, _, err := ApplyExactReputationDeltaTx(ctx, tx, agentAID, EventArenaStakeReturn, 0); err != nil {
				return fmt.Errorf("return agent A arena stake: %w", err)
			}
			if _, _, err := ApplyExactReputationDeltaTx(ctx, tx, agentBID, EventArenaStakeReturn, 0); err != nil {
				return fmt.Errorf("return agent B arena stake: %w", err)
			}
		} else {
			winner := *winnerID
			loser := ""
			switch winner {
			case agentAID:
				loser = agentBID
			case agentBID:
				loser = agentAID
			}

			var loserBalance float64
			if err := tx.QueryRow(ctx, `SELECT COALESCE(reputation_score, 0) FROM participants WHERE id = $1`, loser).Scan(&loserBalance); err != nil {
				return fmt.Errorf("read arena loser balance: %w", err)
			}
			settledStake = math.Min(requestedStake, math.Max(0, loserBalance))
			if _, _, err := ApplyExactReputationDeltaTx(ctx, tx, winner, EventArenaStakeWon, settledStake); err != nil {
				return fmt.Errorf("credit arena stake winner: %w", err)
			}
			if _, applied, err := ApplyExactReputationDeltaTx(ctx, tx, loser, EventArenaStakeLost, -settledStake); err != nil {
				return fmt.Errorf("debit arena stake loser: %w", err)
			} else if applied != -settledStake {
				return fmt.Errorf("arena stake changed while locked: debited %v, expected %v", applied, -settledStake)
			}
		}
	}

	result, err := tx.Exec(ctx, `
		UPDATE arena_battles
		SET settled_stake = $2, stake_settled_at = NOW()
		WHERE id = $1 AND stake_settled_at IS NULL`, battleID, settledStake)
	if err != nil {
		return fmt.Errorf("mark arena stake settled: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("mark arena stake settled: settlement marker changed concurrently")
	}
	return nil
}

// CreateRound inserts a new round for a battle.
func (r *ArenaRepo) CreateRound(ctx context.Context, round *models.ArenaRound) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create arena round: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	err = tx.QueryRow(ctx, `
		INSERT INTO arena_rounds (battle_id, round_number, round_type, deadline)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		round.BattleID, round.RoundNumber, round.RoundType, round.Deadline,
	).Scan(&round.ID, &round.CreatedAt)
	if err != nil {
		return fmt.Errorf("create arena round: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create arena round: %w", err)
	}
	return nil
}

// GetRounds returns all rounds for a battle ordered by round number.
func (r *ArenaRepo) GetRounds(ctx context.Context, battleID string) ([]models.ArenaRound, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, battle_id, round_number, round_type,
		       agent_a_argument, agent_a_submitted_at,
		       agent_b_argument, agent_b_submitted_at,
		       agent_a_argument_score, agent_b_argument_score,
		       agent_a_source_score, agent_b_source_score,
		       agent_a_clarity_score, agent_b_clarity_score,
		       agent_a_total_votes, agent_b_total_votes,
		       round_winner, deadline, closed_at, COALESCE(closure_reason, ''), created_at
		FROM arena_rounds
		WHERE battle_id = $1
		ORDER BY round_number ASC`,
		battleID,
	)
	if err != nil {
		return nil, fmt.Errorf("get arena rounds: %w", err)
	}
	defer rows.Close()

	var rounds []models.ArenaRound
	for rows.Next() {
		var rd models.ArenaRound
		if err := rows.Scan(
			&rd.ID, &rd.BattleID, &rd.RoundNumber, &rd.RoundType,
			&rd.AgentAArgument, &rd.AgentASubmittedAt,
			&rd.AgentBArgument, &rd.AgentBSubmittedAt,
			&rd.AgentAArgumentScore, &rd.AgentBArgumentScore,
			&rd.AgentASourceScore, &rd.AgentBSourceScore,
			&rd.AgentAClarityScore, &rd.AgentBClarityScore,
			&rd.AgentATotalVotes, &rd.AgentBTotalVotes,
			&rd.RoundWinner, &rd.Deadline, &rd.ClosedAt, &rd.ClosureReason, &rd.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan arena round: %w", err)
		}
		rounds = append(rounds, rd)
	}
	return rounds, rows.Err()
}

// SubmitArgument records an agent's argument for a specific round.
// It updates the appropriate column based on which agent is submitting.
// If both agents have submitted, it advances the battle's current_round.
func (r *ArenaRepo) SubmitArgument(ctx context.Context, battleID string, roundNumber int, agentID, argument string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Get the battle to determine which agent this is
	var agentAID, agentBID string
	var battleStatus models.ArenaStatus
	err = tx.QueryRow(ctx, `
		SELECT agent_a_id, agent_b_id, status FROM arena_battles WHERE id = $1 FOR UPDATE`,
		battleID,
	).Scan(&agentAID, &agentBID, &battleStatus)
	if err != nil {
		return 0, fmt.Errorf("get battle for submit: %w", err)
	}

	if battleStatus != models.ArenaStatusActive && battleStatus != models.ArenaStatusPending {
		return 0, fmt.Errorf("battle is not active")
	}

	// Determine which column to update
	var updateCol, submittedAtCol, otherSubmittedAtCol string
	if agentID == agentAID {
		updateCol = "agent_a_argument"
		submittedAtCol = "agent_a_submitted_at"
		otherSubmittedAtCol = "agent_b_submitted_at"
	} else if agentID == agentBID {
		updateCol = "agent_b_argument"
		submittedAtCol = "agent_b_submitted_at"
		otherSubmittedAtCol = "agent_a_submitted_at"
	} else {
		return 0, fmt.Errorf("agent is not a participant in this battle")
	}

	// Update the argument
	now := time.Now()
	var otherSubmittedAt *time.Time
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE arena_rounds
		SET %s = $1, %s = $2
		WHERE battle_id = $3 AND round_number = $4
		RETURNING %s`, updateCol, submittedAtCol, otherSubmittedAtCol),
		argument, now, battleID, roundNumber,
	).Scan(&otherSubmittedAt)
	if err != nil {
		return 0, fmt.Errorf("submit argument: %w", err)
	}

	// If this is the first submission on round 1 for a pending battle, activate it
	if battleStatus == models.ArenaStatusPending {
		_, err = tx.Exec(ctx, `
			UPDATE arena_battles SET status = 'active', current_round = 1 WHERE id = $1`,
			battleID)
		if err != nil {
			return 0, fmt.Errorf("activate battle: %w", err)
		}
	}

	// If both agents have now submitted, advance the current round counter
	openedRound := 0
	if otherSubmittedAt != nil {
		var totalRounds int
		err = tx.QueryRow(ctx, `
			SELECT total_rounds FROM arena_battles WHERE id = $1`, battleID,
		).Scan(&totalRounds)
		if err != nil {
			return 0, fmt.Errorf("get total rounds: %w", err)
		}

		if roundNumber < totalRounds {
			_, err = tx.Exec(ctx, `
				UPDATE arena_battles SET current_round = $1 WHERE id = $2`,
				roundNumber+1, battleID)
			if err != nil {
				return 0, fmt.Errorf("advance round: %w", err)
			}
			openedRound = roundNumber + 1
		}
	}
	if openedRound > 0 {
		battle := &models.ArenaBattle{ID: battleID, AgentAID: agentAID, AgentBID: agentBID, CurrentRound: openedRound}
		if err := tx.QueryRow(ctx, `
			SELECT topic, word_limit, COALESCE(rules, '')
			FROM arena_battles WHERE id = $1`, battleID).Scan(
			&battle.Topic, &battle.WordLimit, &battle.Rules,
		); err != nil {
			return 0, fmt.Errorf("load opened arena round battle: %w", err)
		}
		var opened models.ArenaRound
		if err := tx.QueryRow(ctx, `
			SELECT id, battle_id, round_number, round_type, deadline, created_at
			FROM arena_rounds WHERE battle_id = $1 AND round_number = $2`, battleID, openedRound).Scan(
			&opened.ID, &opened.BattleID, &opened.RoundNumber, &opened.RoundType,
			&opened.Deadline, &opened.CreatedAt,
		); err != nil {
			return 0, fmt.Errorf("load opened arena round: %w", err)
		}
		if _, err := enqueueWebhookEvent(ctx, tx, arenaevents.RoundOpened, arenaevents.RoundPayload(battle, &opened)); err != nil {
			return 0, fmt.Errorf("enqueue opened arena round: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return openedRound, nil
}

// CastVote records a human's vote on a round and updates round score totals.
func (r *ArenaRepo) CastVote(ctx context.Context, vote *models.ArenaVote) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize vote-driven completion on the battle row. A request that was
	// admitted just before another vote completed the battle must not revise
	// the winner after the stake has already been settled.
	var agentAID, agentBID string
	var battleStatus models.ArenaStatus
	err = tx.QueryRow(ctx, `
		SELECT agent_a_id, agent_b_id, status
		FROM arena_battles WHERE id = $1 FOR UPDATE`,
		vote.BattleID,
	).Scan(&agentAID, &agentBID, &battleStatus)
	if err != nil {
		return fmt.Errorf("lock battle for vote: %w", err)
	}
	if battleStatus != models.ArenaStatusActive {
		return fmt.Errorf("battle is not active")
	}

	// Insert the vote
	_, err = tx.Exec(ctx, `
		INSERT INTO arena_votes (battle_id, round_id, voter_id, voted_for,
		                         argument_score, source_score, clarity_score)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		vote.BattleID, vote.RoundID, vote.VoterID, vote.VotedFor,
		vote.ArgumentScore, vote.SourceScore, vote.ClarityScore,
	)
	if err != nil {
		return fmt.Errorf("cast arena vote: %w", err)
	}

	// Recalculate round scores from all votes for this round
	_, err = tx.Exec(ctx, `
		UPDATE arena_rounds SET
			agent_a_argument_score = COALESCE((
				SELECT AVG(argument_score)::FLOAT FROM arena_votes
				WHERE round_id = $1 AND voted_for = $2
			), 0),
			agent_a_source_score = COALESCE((
				SELECT AVG(source_score)::FLOAT FROM arena_votes
				WHERE round_id = $1 AND voted_for = $2
			), 0),
			agent_a_clarity_score = COALESCE((
				SELECT AVG(clarity_score)::FLOAT FROM arena_votes
				WHERE round_id = $1 AND voted_for = $2
			), 0),
			agent_a_total_votes = (
				SELECT COUNT(*) FROM arena_votes
				WHERE round_id = $1 AND voted_for = $2
			),
			agent_b_argument_score = COALESCE((
				SELECT AVG(argument_score)::FLOAT FROM arena_votes
				WHERE round_id = $1 AND voted_for = $3
			), 0),
			agent_b_source_score = COALESCE((
				SELECT AVG(source_score)::FLOAT FROM arena_votes
				WHERE round_id = $1 AND voted_for = $3
			), 0),
			agent_b_clarity_score = COALESCE((
				SELECT AVG(clarity_score)::FLOAT FROM arena_votes
				WHERE round_id = $1 AND voted_for = $3
			), 0),
			agent_b_total_votes = (
				SELECT COUNT(*) FROM arena_votes
				WHERE round_id = $1 AND voted_for = $3
			)
		WHERE id = $1`,
		vote.RoundID, agentAID, agentBID,
	)
	if err != nil {
		return fmt.Errorf("update round scores: %w", err)
	}

	// Determine round winner if enough votes (at least 3 total)
	var agentAVotes, agentBVotes int
	err = tx.QueryRow(ctx, `
		SELECT agent_a_total_votes, agent_b_total_votes
		FROM arena_rounds WHERE id = $1`,
		vote.RoundID,
	).Scan(&agentAVotes, &agentBVotes)
	if err != nil {
		return fmt.Errorf("get round vote counts: %w", err)
	}

	totalVotes := agentAVotes + agentBVotes
	if totalVotes >= 1 {
		// Declare round winner based on total votes received
		var roundWinnerID *string
		if agentAVotes > agentBVotes {
			roundWinnerID = &agentAID
		} else if agentBVotes > agentAVotes {
			roundWinnerID = &agentBID
		}
		// If tied, leave as nil (draw)

		if roundWinnerID != nil {
			_, err = tx.Exec(ctx, `
				UPDATE arena_rounds SET round_winner = $1 WHERE id = $2`,
				roundWinnerID, vote.RoundID)
			if err != nil {
				return fmt.Errorf("set round winner: %w", err)
			}
		}
	}

	// Update voter_count on the battle
	_, err = tx.Exec(ctx, `
		UPDATE arena_battles SET voter_count = (
			SELECT COUNT(DISTINCT voter_id) FROM arena_votes WHERE battle_id = $1
		) WHERE id = $1`,
		vote.BattleID,
	)
	if err != nil {
		return fmt.Errorf("update voter count: %w", err)
	}

	// Check if all rounds are complete (both agents submitted in every round
	// and all rounds have been voted on with a winner declared)
	var totalRounds, completedRounds int
	err = tx.QueryRow(ctx, `
		SELECT
			(SELECT total_rounds FROM arena_battles WHERE id = $1),
			(SELECT COUNT(*) FROM arena_rounds
			 WHERE battle_id = $1
			   AND agent_a_submitted_at IS NOT NULL
			   AND agent_b_submitted_at IS NOT NULL
			   AND round_winner IS NOT NULL)`,
		vote.BattleID,
	).Scan(&totalRounds, &completedRounds)
	if err != nil {
		return fmt.Errorf("check battle completion: %w", err)
	}

	var completedBattle *models.ArenaBattle
	if completedRounds >= totalRounds {
		// Determine overall winner
		var agentAWins, agentBWins int
		err = tx.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE round_winner = $2),
				COUNT(*) FILTER (WHERE round_winner = $3)
			FROM arena_rounds WHERE battle_id = $1`,
			vote.BattleID, agentAID, agentBID,
		).Scan(&agentAWins, &agentBWins)
		if err != nil {
			return fmt.Errorf("count round wins: %w", err)
		}

		var overallWinner *string
		if agentAWins > agentBWins {
			overallWinner = &agentAID
		} else if agentBWins > agentAWins {
			overallWinner = &agentBID
		}
		// If tied, winner_id stays nil (draw)

		_, err = tx.Exec(ctx, `
			UPDATE arena_battles SET status = 'completed', winner_id = $1, completed_at = NOW()
			WHERE id = $2`,
			overallWinner, vote.BattleID)
		if err != nil {
			return fmt.Errorf("complete battle: %w", err)
		}
		if err := settleArenaStakeTx(ctx, tx, vote.BattleID, overallWinner); err != nil {
			return fmt.Errorf("settle completed battle stake: %w", err)
		}
		completedBattle = &models.ArenaBattle{ID: vote.BattleID}
		if err := tx.QueryRow(ctx, `
			SELECT topic, status, agent_a_id, agent_b_id, winner_id, voter_count,
			       total_rounds, completed_at, trust_stake, settled_stake, stake_settled_at
			FROM arena_battles WHERE id = $1`, vote.BattleID).Scan(
			&completedBattle.Topic, &completedBattle.Status,
			&completedBattle.AgentAID, &completedBattle.AgentBID,
			&completedBattle.WinnerID, &completedBattle.VoterCount,
			&completedBattle.TotalRounds, &completedBattle.CompletedAt,
			&completedBattle.TrustStake, &completedBattle.SettledStake,
			&completedBattle.StakeSettledAt,
		); err != nil {
			return fmt.Errorf("load completed arena battle payload: %w", err)
		}
		if _, err := enqueueWebhookEvent(ctx, tx, arenaevents.BattleCompleted, arenaevents.CompletedPayload(completedBattle)); err != nil {
			return fmt.Errorf("enqueue completed arena battle: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ArenaDeadlineTransition describes the durable state change produced by an
// expired round. OpenedRound is non-zero only when the sweep itself advanced
// the battle; Completed is true only for the sweep that finalized it.
type ArenaDeadlineTransition struct {
	BattleID    string
	ClosedRound int
	OpenedRound int
	Completed   bool
}

// ProcessExpiredRounds closes the earliest expired, still-open round for each
// active/pending battle. Rows are locked with SKIP LOCKED so multiple API
// replicas can run the same ticker without processing a deadline twice.
func (r *ArenaRepo) ProcessExpiredRounds(ctx context.Context, now time.Time, limit int) ([]ArenaDeadlineTransition, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin arena deadline sweep: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	type candidate struct {
		roundID     string
		battleID    string
		roundNumber int
	}
	rows, err := tx.Query(ctx, `
		SELECT ar.id, ar.battle_id, ar.round_number
		FROM arena_rounds ar
		JOIN arena_battles ab ON ab.id = ar.battle_id
		WHERE ar.closed_at IS NULL
		  AND ar.deadline IS NOT NULL
		  AND ar.deadline <= $1
		  AND ab.status IN ('pending', 'active')
		  AND NOT EXISTS (
			SELECT 1 FROM arena_rounds earlier
			WHERE earlier.battle_id = ar.battle_id
			  AND earlier.round_number < ar.round_number
			  AND earlier.closed_at IS NULL
		  )
		ORDER BY ar.deadline ASC, ar.battle_id, ar.round_number
		FOR UPDATE OF ar SKIP LOCKED
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("select expired arena rounds: %w", err)
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.roundID, &c.battleID, &c.roundNumber); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan expired arena round: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate expired arena rounds: %w", err)
	}
	rows.Close()

	transitions := make([]ArenaDeadlineTransition, 0, len(candidates))
	for _, c := range candidates {
		var status models.ArenaStatus
		var currentRound, totalRounds int
		var agentAID, agentBID string
		if err := tx.QueryRow(ctx, `
			SELECT status, current_round, total_rounds, agent_a_id, agent_b_id
			FROM arena_battles WHERE id = $1 FOR UPDATE`, c.battleID,
		).Scan(&status, &currentRound, &totalRounds, &agentAID, &agentBID); err != nil {
			return nil, fmt.Errorf("lock expired arena battle: %w", err)
		}
		if status == models.ArenaStatusCompleted || status == models.ArenaStatusCancelled {
			continue
		}

		var agentAVotes, agentBVotes int
		if err := tx.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE voted_for = $2),
				COUNT(*) FILTER (WHERE voted_for = $3)
			FROM arena_votes WHERE round_id = $1`, c.roundID, agentAID, agentBID,
		).Scan(&agentAVotes, &agentBVotes); err != nil {
			return nil, fmt.Errorf("count expired round votes: %w", err)
		}
		var roundWinner *string
		if agentAVotes > agentBVotes {
			roundWinner = &agentAID
		} else if agentBVotes > agentAVotes {
			roundWinner = &agentBID
		}

		result, err := tx.Exec(ctx, `
			UPDATE arena_rounds SET
				round_winner = $2,
				agent_a_argument_score = COALESCE((SELECT AVG(argument_score)::FLOAT FROM arena_votes WHERE round_id = $1 AND voted_for = $3), 0),
				agent_a_source_score = COALESCE((SELECT AVG(source_score)::FLOAT FROM arena_votes WHERE round_id = $1 AND voted_for = $3), 0),
				agent_a_clarity_score = COALESCE((SELECT AVG(clarity_score)::FLOAT FROM arena_votes WHERE round_id = $1 AND voted_for = $3), 0),
				agent_a_total_votes = $4,
				agent_b_argument_score = COALESCE((SELECT AVG(argument_score)::FLOAT FROM arena_votes WHERE round_id = $1 AND voted_for = $5), 0),
				agent_b_source_score = COALESCE((SELECT AVG(source_score)::FLOAT FROM arena_votes WHERE round_id = $1 AND voted_for = $5), 0),
				agent_b_clarity_score = COALESCE((SELECT AVG(clarity_score)::FLOAT FROM arena_votes WHERE round_id = $1 AND voted_for = $5), 0),
				agent_b_total_votes = $6,
				closed_at = $7,
				closure_reason = 'deadline'
			WHERE id = $1 AND closed_at IS NULL`,
			c.roundID, roundWinner, agentAID, agentAVotes, agentBID, agentBVotes, now)
		if err != nil {
			return nil, fmt.Errorf("close expired arena round: %w", err)
		}
		if result.RowsAffected() == 0 {
			continue
		}

		transition := ArenaDeadlineTransition{BattleID: c.battleID, ClosedRound: c.roundNumber}
		var closedRounds int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM arena_rounds WHERE battle_id = $1 AND closed_at IS NOT NULL`, c.battleID).Scan(&closedRounds); err != nil {
			return nil, fmt.Errorf("count closed arena rounds: %w", err)
		}
		if closedRounds >= totalRounds {
			var agentAWins, agentBWins int
			if err := tx.QueryRow(ctx, `
				SELECT
					COUNT(*) FILTER (WHERE round_winner = $2),
					COUNT(*) FILTER (WHERE round_winner = $3)
				FROM arena_rounds WHERE battle_id = $1`, c.battleID, agentAID, agentBID,
			).Scan(&agentAWins, &agentBWins); err != nil {
				return nil, fmt.Errorf("count expired battle wins: %w", err)
			}
			var overallWinner *string
			if agentAWins > agentBWins {
				overallWinner = &agentAID
			} else if agentBWins > agentAWins {
				overallWinner = &agentBID
			}
			if _, err := tx.Exec(ctx, `
				UPDATE arena_battles
				SET status = 'completed', winner_id = $2, completed_at = $3
				WHERE id = $1 AND status IN ('pending', 'active')`, c.battleID, overallWinner, now); err != nil {
				return nil, fmt.Errorf("complete expired arena battle: %w", err)
			}
			if err := settleArenaStakeTx(ctx, tx, c.battleID, overallWinner); err != nil {
				return nil, fmt.Errorf("settle expired arena battle stake: %w", err)
			}
			transition.Completed = true
		} else if c.roundNumber < totalRounds && currentRound < c.roundNumber+1 {
			transition.OpenedRound = c.roundNumber + 1
			if _, err := tx.Exec(ctx, `
				UPDATE arena_battles SET status = 'active', current_round = $2 WHERE id = $1`,
				c.battleID, transition.OpenedRound); err != nil {
				return nil, fmt.Errorf("advance expired arena battle: %w", err)
			}
		}
		if transition.OpenedRound > 0 {
			battle, err := loadArenaBattleEvent(ctx, tx, c.battleID)
			if err != nil {
				return nil, fmt.Errorf("load deadline-opened battle payload: %w", err)
			}
			opened, err := loadArenaRoundEvent(ctx, tx, c.battleID, transition.OpenedRound)
			if err != nil {
				return nil, fmt.Errorf("load deadline-opened round payload: %w", err)
			}
			if _, err := enqueueWebhookEvent(ctx, tx, arenaevents.RoundOpened, arenaevents.RoundPayload(battle, opened)); err != nil {
				return nil, fmt.Errorf("enqueue deadline-opened round: %w", err)
			}
		}
		if transition.Completed {
			battle, err := loadArenaBattleEvent(ctx, tx, c.battleID)
			if err != nil {
				return nil, fmt.Errorf("load deadline-completed battle payload: %w", err)
			}
			if _, err := enqueueWebhookEvent(ctx, tx, arenaevents.BattleCompleted, arenaevents.CompletedPayload(battle)); err != nil {
				return nil, fmt.Errorf("enqueue deadline-completed battle: %w", err)
			}
		}
		transitions = append(transitions, transition)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit arena deadline sweep: %w", err)
	}
	return transitions, nil
}

// GetVotes returns all votes for a specific round.
func (r *ArenaRepo) GetVotes(ctx context.Context, roundID string) ([]models.ArenaVote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT av.id, av.battle_id, av.round_id, av.voter_id,
		       COALESCE(p.display_name, '') as voter_name,
		       av.voted_for, av.argument_score, av.source_score, av.clarity_score,
		       av.created_at
		FROM arena_votes av
		LEFT JOIN participants p ON p.id = av.voter_id
		WHERE av.round_id = $1
		ORDER BY av.created_at ASC`,
		roundID,
	)
	if err != nil {
		return nil, fmt.Errorf("get arena votes: %w", err)
	}
	defer rows.Close()

	var votes []models.ArenaVote
	for rows.Next() {
		var v models.ArenaVote
		if err := rows.Scan(
			&v.ID, &v.BattleID, &v.RoundID, &v.VoterID, &v.VoterName,
			&v.VotedFor, &v.ArgumentScore, &v.SourceScore, &v.ClarityScore,
			&v.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan arena vote: %w", err)
		}
		votes = append(votes, v)
	}
	return votes, rows.Err()
}

// HasVoted checks whether a participant has already voted on a specific round.
func (r *ArenaRepo) HasVoted(ctx context.Context, roundID, voterID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM arena_votes WHERE round_id = $1 AND voter_id = $2)`,
		roundID, voterID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check arena vote exists: %w", err)
	}
	return exists, nil
}

// AddComment inserts a spectator comment on a battle.
func (r *ArenaRepo) AddComment(ctx context.Context, comment *models.ArenaComment) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO arena_comments (battle_id, author_id, body)
		VALUES ($1, $2, $3)`,
		comment.BattleID, comment.AuthorID, comment.Body,
	)
	if err != nil {
		return fmt.Errorf("add arena comment: %w", err)
	}
	return nil
}

// GetComments returns comments for a battle, ordered by creation time.
func (r *ArenaRepo) GetComments(ctx context.Context, battleID string, limit, offset int) ([]models.ArenaComment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ac.id, ac.battle_id, ac.author_id,
		       COALESCE(p.display_name, '') as author_name,
		       ac.body, ac.created_at
		FROM arena_comments ac
		LEFT JOIN participants p ON p.id = ac.author_id
		WHERE ac.battle_id = $1
		ORDER BY ac.created_at ASC
		LIMIT $2 OFFSET $3`,
		battleID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get arena comments: %w", err)
	}
	defer rows.Close()

	var comments []models.ArenaComment
	for rows.Next() {
		var c models.ArenaComment
		if err := rows.Scan(
			&c.ID, &c.BattleID, &c.AuthorID, &c.AuthorName,
			&c.Body, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan arena comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// GetLeaderboard returns the top agents by arena performance.
func (r *ArenaRepo) GetLeaderboard(ctx context.Context, limit int) ([]models.ArenaLeaderEntry, error) {
	rows, err := r.pool.Query(ctx, `
		WITH agent_battles AS (
			SELECT
				agent_id,
				COUNT(*) as total_battles,
				COUNT(*) FILTER (WHERE winner_id = agent_id) as wins,
				COUNT(*) FILTER (WHERE winner_id IS NOT NULL AND winner_id != agent_id) as losses,
				COUNT(*) FILTER (WHERE winner_id IS NULL AND status = 'completed') as draws
			FROM (
				SELECT agent_a_id as agent_id, winner_id, status FROM arena_battles WHERE status = 'completed'
				UNION ALL
				SELECT agent_b_id as agent_id, winner_id, status FROM arena_battles WHERE status = 'completed'
			) sub
			GROUP BY agent_id
		),
		agent_scores AS (
			SELECT
				voted_for as agent_id,
				AVG((argument_score + source_score + clarity_score)::FLOAT / 3.0) as avg_score
			FROM arena_votes
			GROUP BY voted_for
		)
		SELECT
			ab.agent_id,
			COALESCE(p.display_name, '') as agent_name,
			ab.wins, ab.losses, ab.draws, ab.total_battles,
			CASE WHEN ab.total_battles > 0
				THEN ab.wins::FLOAT / ab.total_battles
				ELSE 0 END as win_rate,
			COALESCE(asc2.avg_score, 0) as avg_score,
			COALESCE(p.trust_score, 0) as trust_score
		FROM agent_battles ab
		LEFT JOIN participants p ON p.id = ab.agent_id
		LEFT JOIN agent_scores asc2 ON asc2.agent_id = ab.agent_id
		ORDER BY ab.wins DESC, win_rate DESC, avg_score DESC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get arena leaderboard: %w", err)
	}
	defer rows.Close()

	var entries []models.ArenaLeaderEntry
	for rows.Next() {
		var e models.ArenaLeaderEntry
		if err := rows.Scan(
			&e.AgentID, &e.AgentName,
			&e.Wins, &e.Losses, &e.Draws, &e.TotalBattles,
			&e.WinRate, &e.AvgScore, &e.TrustScore,
		); err != nil {
			return nil, fmt.Errorf("scan arena leaderboard entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetAgentArenaStats returns an agent's arena win/loss/draw stats.
func (r *ArenaRepo) GetAgentArenaStats(ctx context.Context, agentID string) (*models.ArenaStats, error) {
	var s models.ArenaStats
	s.AgentID = agentID

	err := r.pool.QueryRow(ctx, `
		WITH agent_battles AS (
			SELECT winner_id, status FROM arena_battles
			WHERE (agent_a_id = $1 OR agent_b_id = $1) AND status = 'completed'
		)
		SELECT
			COUNT(*) as total_battles,
			COUNT(*) FILTER (WHERE winner_id = $1) as wins,
			COUNT(*) FILTER (WHERE winner_id IS NOT NULL AND winner_id != $1) as losses,
			COUNT(*) FILTER (WHERE winner_id IS NULL) as draws
		FROM agent_battles`,
		agentID,
	).Scan(&s.TotalBattles, &s.Wins, &s.Losses, &s.Draws)
	if err != nil {
		return nil, fmt.Errorf("get agent arena stats: %w", err)
	}

	if s.TotalBattles > 0 {
		s.WinRate = float64(s.Wins) / float64(s.TotalBattles)
	}

	// Get average score
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(AVG((argument_score + source_score + clarity_score)::FLOAT / 3.0), 0)
		FROM arena_votes WHERE voted_for = $1`,
		agentID,
	).Scan(&s.AvgScore)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("get agent arena avg score: %w", err)
	}

	return &s, nil
}

// GetRoundByBattleAndNumber returns a specific round by battle ID and round number.
func (r *ArenaRepo) GetRoundByBattleAndNumber(ctx context.Context, battleID string, roundNumber int) (*models.ArenaRound, error) {
	var rd models.ArenaRound
	err := r.pool.QueryRow(ctx, `
		SELECT id, battle_id, round_number, round_type,
		       agent_a_argument, agent_a_submitted_at,
		       agent_b_argument, agent_b_submitted_at,
		       agent_a_argument_score, agent_b_argument_score,
		       agent_a_source_score, agent_b_source_score,
		       agent_a_clarity_score, agent_b_clarity_score,
		       agent_a_total_votes, agent_b_total_votes,
		       round_winner, deadline, closed_at, COALESCE(closure_reason, ''), created_at
		FROM arena_rounds
		WHERE battle_id = $1 AND round_number = $2`,
		battleID, roundNumber,
	).Scan(
		&rd.ID, &rd.BattleID, &rd.RoundNumber, &rd.RoundType,
		&rd.AgentAArgument, &rd.AgentASubmittedAt,
		&rd.AgentBArgument, &rd.AgentBSubmittedAt,
		&rd.AgentAArgumentScore, &rd.AgentBArgumentScore,
		&rd.AgentASourceScore, &rd.AgentBSourceScore,
		&rd.AgentAClarityScore, &rd.AgentBClarityScore,
		&rd.AgentATotalVotes, &rd.AgentBTotalVotes,
		&rd.RoundWinner, &rd.Deadline, &rd.ClosedAt, &rd.ClosureReason, &rd.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get arena round: %w", err)
	}
	return &rd, nil
}
