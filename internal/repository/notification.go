package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Notification represents a notification for a participant.
type Notification struct {
	ID          string    `json:"id"`
	RecipientID string    `json:"recipient_id"`
	Type        string    `json:"type"`
	ActorID     *string   `json:"actor_id,omitempty"`
	PostID      *string   `json:"post_id,omitempty"`
	CommentID   *string   `json:"comment_id,omitempty"`
	Message     string    `json:"message"`
	IsRead      bool      `json:"is_read"`
	CreatedAt   time.Time `json:"created_at"`
	// Joined fields
	ActorName string `json:"actor_name,omitempty"`
	ActorType string `json:"actor_type,omitempty"`
}

// NotificationRepo handles database operations for notifications.
type NotificationRepo struct {
	pool       *pgxpool.Pool
	pushFanout PushFanout
}

// NewNotificationRepo creates a new NotificationRepo.
func NewNotificationRepo(pool *pgxpool.Pool) *NotificationRepo {
	return &NotificationRepo{pool: pool}
}

// PushFanout is the minimal interface the notification repo needs to
// fire browser push notifications after a Create. Satisfied by a small
// helper wired in routes.go — NotificationRepo stays decoupled from
// the push package that way (which in turn prevents an import cycle).
type PushFanout interface {
	Notify(ctx context.Context, recipientID, notifType, message string, postID *string)
}

// WithPushFanout registers a fanout hook. Safe to leave unset — when
// nil, Create just inserts the row without emitting pushes.
func (r *NotificationRepo) WithPushFanout(fn PushFanout) {
	r.pushFanout = fn
}

// Create inserts a new notification. If a push fanout hook is wired
// via WithPushFanout, fires a browser push on a background goroutine
// (best-effort; never blocks the caller).
func (r *NotificationRepo) Create(ctx context.Context, recipientID, notifType string, actorID, postID, commentID *string, message string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO notifications (recipient_id, type, actor_id, post_id, comment_id, message)
         VALUES ($1, $2, $3, $4, $5, $6)`,
		recipientID, notifType, actorID, postID, commentID, message)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	if r.pushFanout != nil {
		// Detach from the request context so push delivery isn't
		// cancelled when the HTTP handler returns. We still honor the
		// process lifetime via context.Background.
		go r.pushFanout.Notify(context.Background(), recipientID, notifType, message, postID)
	}
	return nil
}

// ListByRecipient returns paginated notifications for a recipient, newest first.
func (r *NotificationRepo) ListByRecipient(ctx context.Context, recipientID string, limit, offset int) ([]Notification, int, error) {
	var total int
	_ = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1`, recipientID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
        SELECT n.id, n.recipient_id, n.type, n.actor_id, n.post_id, n.comment_id,
               n.message, n.is_read, n.created_at,
               COALESCE(p.display_name, '') as actor_name,
               COALESCE(p.type::text, '') as actor_type
        FROM notifications n
        LEFT JOIN participants p ON p.id = n.actor_id
        WHERE n.recipient_id = $1
        ORDER BY n.created_at DESC
        LIMIT $2 OFFSET $3`,
		recipientID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var notifs []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.RecipientID, &n.Type, &n.ActorID, &n.PostID, &n.CommentID,
			&n.Message, &n.IsRead, &n.CreatedAt, &n.ActorName, &n.ActorType); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}
		notifs = append(notifs, n)
	}
	return notifs, total, rows.Err()
}

// MarkRead marks a specific notification as read for the given recipient.
func (r *NotificationRepo) MarkRead(ctx context.Context, recipientID, notifID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET is_read = TRUE WHERE id = $1 AND recipient_id = $2`,
		notifID, recipientID)
	return err
}

// MarkAllRead marks all unread notifications as read for the given recipient.
func (r *NotificationRepo) MarkAllRead(ctx context.Context, recipientID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET is_read = TRUE WHERE recipient_id = $1 AND is_read = FALSE`,
		recipientID)
	return err
}

// UnreadCount returns the number of unread notifications for a recipient.
func (r *NotificationRepo) UnreadCount(ctx context.Context, recipientID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND is_read = FALSE`,
		recipientID).Scan(&count)
	return count, err
}
