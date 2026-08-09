package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Message represents a direct message.
type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	SenderID       string    `json:"sender_id"`
	SenderName     string    `json:"sender_name,omitempty"`
	SenderAvatar   string    `json:"sender_avatar,omitempty"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}

// ConversationPreview represents a conversation with its last message preview.
type ConversationPreview struct {
	ID               string     `json:"id"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LastMessageBody  string     `json:"last_message_body,omitempty"`
	LastMessageAt    *time.Time `json:"last_message_at,omitempty"`
	UnreadCount      int        `json:"unread_count"`
	OtherParticipant *OtherParticipantInfo `json:"other_participant,omitempty"`
}

// OtherParticipantInfo holds info about the other participant in a conversation.
type OtherParticipantInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Type        string `json:"type"`
}

// MessageRepo handles database operations for direct messages.
type MessageRepo struct {
	pool *pgxpool.Pool
}

// NewMessageRepo creates a new MessageRepo.
func NewMessageRepo(pool *pgxpool.Pool) *MessageRepo {
	return &MessageRepo{pool: pool}
}

// CreateConversation finds an existing conversation between two participants or creates a new one.
func (r *MessageRepo) CreateConversation(ctx context.Context, participantIDs []string) (string, error) {
	if len(participantIDs) != 2 {
		return "", fmt.Errorf("exactly 2 participants required")
	}

	// Find existing conversation between these two participants
	var existingID string
	err := r.pool.QueryRow(ctx, `
		SELECT cp1.conversation_id
		FROM conversation_participants cp1
		JOIN conversation_participants cp2 ON cp2.conversation_id = cp1.conversation_id
		WHERE cp1.participant_id = $1
		  AND cp2.participant_id = $2`,
		participantIDs[0], participantIDs[1],
	).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	// Only "no existing conversation" should fall through to create one. A
	// transient DB error here must be surfaced, not silently treated as
	// "none found" — otherwise we create a duplicate conversation and split
	// the DM thread.
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("look up existing conversation: %w", err)
	}

	// Create new conversation
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var convID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO conversations DEFAULT VALUES RETURNING id`,
	).Scan(&convID); err != nil {
		return "", fmt.Errorf("create conversation: %w", err)
	}

	for _, pid := range participantIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO conversation_participants (conversation_id, participant_id) VALUES ($1, $2)`,
			convID, pid); err != nil {
			return "", fmt.Errorf("add participant: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return convID, nil
}

// SendMessage inserts a new message and updates the conversation's updated_at.
func (r *MessageRepo) SendMessage(ctx context.Context, conversationID, senderID, body string) (*Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var m Message
	if err := tx.QueryRow(ctx,
		`INSERT INTO messages (conversation_id, sender_id, body)
         VALUES ($1, $2, $3)
         RETURNING id, conversation_id, sender_id, body, created_at`,
		conversationID, senderID, body,
	).Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Body, &m.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE conversations SET updated_at = NOW() WHERE id = $1`, conversationID); err != nil {
		return nil, fmt.Errorf("update conversation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &m, nil
}

// ListConversations returns all conversations for a participant with last message preview.
func (r *MessageRepo) ListConversations(ctx context.Context, participantID string) ([]ConversationPreview, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.created_at, c.updated_at,
		       COALESCE(last_m.body, '') as last_message_body,
		       last_m.created_at as last_message_at,
		       COALESCE(unread.cnt, 0) as unread_count,
		       other_p.id as other_id,
		       other_p.display_name as other_name,
		       COALESCE(other_p.avatar_url, '') as other_avatar,
		       other_p.type as other_type
		FROM conversations c
		JOIN conversation_participants cp ON cp.conversation_id = c.id AND cp.participant_id = $1
		-- get the other participant
		LEFT JOIN conversation_participants cp2 ON cp2.conversation_id = c.id AND cp2.participant_id != $1
		LEFT JOIN participants other_p ON other_p.id = cp2.participant_id
		-- last message
		LEFT JOIN LATERAL (
			SELECT body, created_at FROM messages
			WHERE conversation_id = c.id
			ORDER BY created_at DESC LIMIT 1
		) last_m ON TRUE
		-- unread count
		LEFT JOIN LATERAL (
			SELECT COUNT(*) as cnt FROM messages m
			WHERE m.conversation_id = c.id
			  AND m.sender_id != $1
			  AND (cp.last_read_at IS NULL OR m.created_at > cp.last_read_at)
		) unread ON TRUE
		ORDER BY c.updated_at DESC`,
		participantID)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var convs []ConversationPreview
	for rows.Next() {
		var c ConversationPreview
		var otherID, otherName, otherAvatar, otherType string
		if err := rows.Scan(
			&c.ID, &c.CreatedAt, &c.UpdatedAt,
			&c.LastMessageBody, &c.LastMessageAt,
			&c.UnreadCount,
			&otherID, &otherName, &otherAvatar, &otherType,
		); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		if otherID != "" {
			c.OtherParticipant = &OtherParticipantInfo{
				ID:          otherID,
				DisplayName: otherName,
				AvatarURL:   otherAvatar,
				Type:        otherType,
			}
		}
		convs = append(convs, c)
	}
	return convs, rows.Err()
}

// ListMessages returns paginated messages in a conversation.
func (r *MessageRepo) ListMessages(ctx context.Context, conversationID string, limit, offset int) ([]Message, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.conversation_id, m.sender_id,
		       COALESCE(p.display_name, '') as sender_name,
		       COALESCE(p.avatar_url, '') as sender_avatar,
		       m.body, m.created_at
		FROM messages m
		LEFT JOIN participants p ON p.id = m.sender_id
		WHERE m.conversation_id = $1
		ORDER BY m.created_at DESC
		LIMIT $2 OFFSET $3`,
		conversationID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.SenderName, &m.SenderAvatar, &m.Body, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// MarkRead updates last_read_at for a participant in a conversation.
func (r *MessageRepo) MarkRead(ctx context.Context, conversationID, participantID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversation_participants SET last_read_at = NOW()
         WHERE conversation_id = $1 AND participant_id = $2`,
		conversationID, participantID)
	return err
}

// IsParticipant checks if a participant is part of a conversation.
func (r *MessageRepo) IsParticipant(ctx context.Context, conversationID, participantID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM conversation_participants WHERE conversation_id = $1 AND participant_id = $2)`,
		conversationID, participantID,
	).Scan(&exists)
	return exists, err
}
