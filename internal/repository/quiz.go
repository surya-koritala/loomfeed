package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type QuizAttempt struct {
	ID            string            `json:"id"`
	PostID        string            `json:"post_id"`
	ParticipantID string            `json:"participant_id"`
	Answers       map[string]string `json:"answers"`
	Score         int               `json:"score"`
	Total         int               `json:"total"`
	CompletedAt   time.Time         `json:"completed_at"`
}

type QuizStats struct {
	TotalAttempts     int         `json:"total_attempts"`
	AvgScore          float64     `json:"avg_score"`
	AvgPercentage     float64     `json:"avg_percentage"`
	ScoreDistribution map[int]int `json:"score_distribution"`
}

type QuizRepo struct {
	pool *pgxpool.Pool
}

func NewQuizRepo(pool *pgxpool.Pool) *QuizRepo {
	return &QuizRepo{pool: pool}
}

func (r *QuizRepo) SubmitAttempt(ctx context.Context, postID, participantID string, answers map[string]string, score, total int) (*QuizAttempt, error) {
	answersJSON, err := json.Marshal(answers)
	if err != nil {
		return nil, fmt.Errorf("marshal answers: %w", err)
	}

	var qa QuizAttempt
	var rawAnswers []byte
	err = r.pool.QueryRow(ctx, `
		INSERT INTO quiz_attempts (post_id, participant_id, answers, score, total)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, post_id, participant_id, answers, score, total, completed_at`,
		postID, participantID, answersJSON, score, total,
	).Scan(&qa.ID, &qa.PostID, &qa.ParticipantID, &rawAnswers, &qa.Score, &qa.Total, &qa.CompletedAt)
	if err != nil {
		return nil, fmt.Errorf("submit quiz attempt: %w", err)
	}
	_ = json.Unmarshal(rawAnswers, &qa.Answers)
	return &qa, nil
}

func (r *QuizRepo) GetUserAttempt(ctx context.Context, postID, participantID string) (*QuizAttempt, error) {
	var qa QuizAttempt
	var rawAnswers []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, post_id, participant_id, answers, score, total, completed_at
		FROM quiz_attempts
		WHERE post_id = $1 AND participant_id = $2
		ORDER BY completed_at DESC LIMIT 1`,
		postID, participantID,
	).Scan(&qa.ID, &qa.PostID, &qa.ParticipantID, &rawAnswers, &qa.Score, &qa.Total, &qa.CompletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user attempt: %w", err)
	}
	_ = json.Unmarshal(rawAnswers, &qa.Answers)
	return &qa, nil
}

func (r *QuizRepo) GetStats(ctx context.Context, postID string) (*QuizStats, error) {
	var stats QuizStats
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(AVG(score), 0), COALESCE(AVG(score::float / NULLIF(total, 0) * 100), 0)
		FROM quiz_attempts WHERE post_id = $1`,
		postID,
	).Scan(&stats.TotalAttempts, &stats.AvgScore, &stats.AvgPercentage)
	if err != nil {
		return nil, fmt.Errorf("get quiz stats: %w", err)
	}

	stats.ScoreDistribution = make(map[int]int)
	rows, err := r.pool.Query(ctx, `SELECT score, COUNT(*) FROM quiz_attempts WHERE post_id = $1 GROUP BY score ORDER BY score`, postID)
	if err != nil {
		return nil, fmt.Errorf("get score distribution: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var score, count int
		if err := rows.Scan(&score, &count); err != nil {
			return nil, fmt.Errorf("scan score distribution: %w", err)
		}
		stats.ScoreDistribution[score] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return &stats, nil
}
