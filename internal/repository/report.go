package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Report struct {
	ID          string     `json:"id"`
	ReporterID  string     `json:"reporter_id"`
	ContentID   string     `json:"content_id"`
	ContentType string     `json:"content_type"`
	Reason      string     `json:"reason"`
	Details     string     `json:"details,omitempty"`
	Status      string     `json:"status"`
	ResolvedBy  *string    `json:"resolved_by,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type ReportRepo struct {
	pool *pgxpool.Pool
}

func NewReportRepo(pool *pgxpool.Pool) *ReportRepo {
	return &ReportRepo{pool: pool}
}

func (r *ReportRepo) Create(ctx context.Context, reporterID, contentID, contentType, reason, details string) (*Report, error) {
	var report Report
	err := r.pool.QueryRow(ctx,
		`INSERT INTO reports (reporter_id, content_id, content_type, reason, details)
         VALUES ($1, $2, $3, $4, NULLIF($5, ''))
         RETURNING id, reporter_id, content_id, content_type, reason, COALESCE(details, ''), status, created_at`,
		reporterID, contentID, contentType, reason, details,
	).Scan(&report.ID, &report.ReporterID, &report.ContentID, &report.ContentType,
		&report.Reason, &report.Details, &report.Status, &report.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create report: %w", err)
	}
	return &report, nil
}

func (r *ReportRepo) ListPending(ctx context.Context, communityID string, limit, offset int) ([]Report, error) {
	rows, err := r.pool.Query(ctx, `
        SELECT r.id, r.reporter_id, r.content_id, r.content_type, r.reason,
               COALESCE(r.details, ''), r.status, r.created_at
        FROM reports r
        JOIN posts p ON (r.content_type = 'post' AND r.content_id = p.id AND p.community_id = $1)
        WHERE r.status = 'pending'
        ORDER BY r.created_at DESC
        LIMIT $2 OFFSET $3`, communityID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []Report
	for rows.Next() {
		var rep Report
		if err := rows.Scan(&rep.ID, &rep.ReporterID, &rep.ContentID, &rep.ContentType,
			&rep.Reason, &rep.Details, &rep.Status, &rep.CreatedAt); err != nil {
			return nil, err
		}
		reports = append(reports, rep)
	}
	return reports, rows.Err()
}

// GetByID fetches a single report. Used to resolve the report's target
// content → community for the moderator authorization check before a
// resolve/dismiss is permitted.
func (r *ReportRepo) GetByID(ctx context.Context, reportID string) (*Report, error) {
	var rep Report
	err := r.pool.QueryRow(ctx, `
        SELECT id, reporter_id, content_id, content_type, reason,
               COALESCE(details, ''), status, created_at
        FROM reports WHERE id = $1`, reportID).
		Scan(&rep.ID, &rep.ReporterID, &rep.ContentID, &rep.ContentType,
			&rep.Reason, &rep.Details, &rep.Status, &rep.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &rep, nil
}

func (r *ReportRepo) Resolve(ctx context.Context, reportID, resolverID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE reports SET status = $1, resolved_by = $2, resolved_at = NOW() WHERE id = $3`,
		status, resolverID, reportID)
	return err
}

// ResolveAllForContent closes every pending report against a single
// content row. Used when a moderator takes a direct action (remove or
// approve) on the target — the individual reports don't need further
// attention once the content itself has been addressed.
func (r *ReportRepo) ResolveAllForContent(ctx context.Context, contentID, contentType, resolverID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE reports SET status = $1, resolved_by = $2, resolved_at = NOW()
         WHERE content_id = $3 AND content_type = $4 AND status = 'pending'`,
		status, resolverID, contentID, contentType)
	return err
}
