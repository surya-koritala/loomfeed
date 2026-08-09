package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// CreateBattle inserts a new arena battle.
func (r *ArenaRepo) CreateBattle(ctx context.Context, battle *models.ArenaBattle) (*models.ArenaBattle, error) {
	var b models.ArenaBattle
	err := r.pool.QueryRow(ctx, `
		INSERT INTO arena_battles (topic, description, agent_a_id, agent_b_id, format, status,
		                           total_rounds, current_round, round_time_limit, word_limit,
		                           rules, trust_stake, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, topic, description, agent_a_id, agent_b_id, format, status,
		          total_rounds, current_round, round_time_limit, word_limit,
		          rules, trust_stake, winner_id, voter_count, created_by, created_at, completed_at`,
		battle.Topic, battle.Description, battle.AgentAID, battle.AgentBID,
		battle.Format, models.ArenaStatusPending,
		battle.TotalRounds, 0, battle.RoundTimeLimit, battle.WordLimit,
		battle.Rules, battle.TrustStake, battle.CreatedBy,
	).Scan(
		&b.ID, &b.Topic, &b.Description, &b.AgentAID, &b.AgentBID,
		&b.Format, &b.Status,
		&b.TotalRounds, &b.CurrentRound, &b.RoundTimeLimit, &b.WordLimit,
		&b.Rules, &b.TrustStake, &b.WinnerID, &b.VoterCount,
		&b.CreatedBy, &b.CreatedAt, &b.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create arena battle: %w", err)
	}
	return &b, nil
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
		       ab.trust_stake, ab.winner_id, ab.voter_count,
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
		&b.TrustStake, &b.WinnerID, &b.VoterCount,
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
		       ab.trust_stake, ab.winner_id, ab.voter_count,
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
			&b.TrustStake, &b.WinnerID, &b.VoterCount,
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
	var err error
	if status == models.ArenaStatusCompleted {
		_, err = r.pool.Exec(ctx, `
			UPDATE arena_battles SET status = $1, winner_id = $2, completed_at = NOW()
			WHERE id = $3`,
			status, winnerID, id)
	} else {
		_, err = r.pool.Exec(ctx, `
			UPDATE arena_battles SET status = $1 WHERE id = $2`,
			status, id)
	}
	if err != nil {
		return fmt.Errorf("update arena battle status: %w", err)
	}
	return nil
}

// CreateRound inserts a new round for a battle.
func (r *ArenaRepo) CreateRound(ctx context.Context, round *models.ArenaRound) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO arena_rounds (battle_id, round_number, round_type, deadline)
		VALUES ($1, $2, $3, $4)`,
		round.BattleID, round.RoundNumber, round.RoundType, round.Deadline,
	)
	if err != nil {
		return fmt.Errorf("create arena round: %w", err)
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
		       round_winner, deadline, created_at
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
			&rd.RoundWinner, &rd.Deadline, &rd.CreatedAt,
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
func (r *ArenaRepo) SubmitArgument(ctx context.Context, battleID string, roundNumber int, agentID, argument string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
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
		return fmt.Errorf("get battle for submit: %w", err)
	}

	if battleStatus != models.ArenaStatusActive && battleStatus != models.ArenaStatusPending {
		return fmt.Errorf("battle is not active")
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
		return fmt.Errorf("agent is not a participant in this battle")
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
		return fmt.Errorf("submit argument: %w", err)
	}

	// If this is the first submission on round 1 for a pending battle, activate it
	if battleStatus == models.ArenaStatusPending {
		_, err = tx.Exec(ctx, `
			UPDATE arena_battles SET status = 'active', current_round = 1 WHERE id = $1`,
			battleID)
		if err != nil {
			return fmt.Errorf("activate battle: %w", err)
		}
	}

	// If both agents have now submitted, advance the current round counter
	if otherSubmittedAt != nil {
		var totalRounds int
		err = tx.QueryRow(ctx, `
			SELECT total_rounds FROM arena_battles WHERE id = $1`, battleID,
		).Scan(&totalRounds)
		if err != nil {
			return fmt.Errorf("get total rounds: %w", err)
		}

		if roundNumber < totalRounds {
			_, err = tx.Exec(ctx, `
				UPDATE arena_battles SET current_round = $1 WHERE id = $2`,
				roundNumber+1, battleID)
			if err != nil {
				return fmt.Errorf("advance round: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// CastVote records a human's vote on a round and updates round score totals.
func (r *ArenaRepo) CastVote(ctx context.Context, vote *models.ArenaVote) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

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

	// Get the battle to determine agent IDs
	var agentAID, agentBID string
	err = tx.QueryRow(ctx, `
		SELECT agent_a_id, agent_b_id FROM arena_battles WHERE id = $1`,
		vote.BattleID,
	).Scan(&agentAID, &agentBID)
	if err != nil {
		return fmt.Errorf("get battle agents: %w", err)
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
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
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
		       round_winner, deadline, created_at
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
		&rd.RoundWinner, &rd.Deadline, &rd.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get arena round: %w", err)
	}
	return &rd, nil
}
