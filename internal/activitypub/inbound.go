package activitypub

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
	"golang.org/x/net/html"
)

// LocalTarget is a verified Loomfeed object URI. For a reply to a comment,
// ParentCommentID and TargetID are that comment while PostID remains its post.
type LocalTarget struct {
	PostID          string
	ParentCommentID *string
	TargetID        string
	TargetType      models.TargetType
}

// ResolveLocalTarget accepts only canonical objects on this Loomfeed origin.
// It supports outbound post Notes and local comment permalink URLs.
func ResolveLocalTarget(origin, objectURI string) (LocalTarget, error) {
	base, err := url.Parse(strings.TrimRight(origin, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return LocalTarget{}, fmt.Errorf("invalid local ActivityPub origin")
	}
	object, err := url.Parse(objectURI)
	if err != nil || object.Scheme == "" || object.Host == "" {
		return LocalTarget{}, fmt.Errorf("invalid ActivityPub object URI")
	}
	if !strings.EqualFold(base.Scheme, object.Scheme) || !strings.EqualFold(base.Host, object.Host) {
		return LocalTarget{}, fmt.Errorf("ActivityPub object is not local")
	}
	parts := strings.Split(strings.Trim(object.EscapedPath(), "/"), "/")
	if len(parts) < 2 || parts[0] != "post" {
		return LocalTarget{}, fmt.Errorf("unsupported local ActivityPub object path")
	}
	postID, err := url.PathUnescape(parts[1])
	if err != nil {
		return LocalTarget{}, fmt.Errorf("invalid post id encoding")
	}
	if _, err := uuid.Parse(postID); err != nil {
		return LocalTarget{}, fmt.Errorf("invalid local post id")
	}
	target := LocalTarget{PostID: postID, TargetID: postID, TargetType: models.TargetPost}
	if len(parts) >= 4 && parts[2] == "comment" {
		commentID, err := url.PathUnescape(parts[3])
		if err != nil {
			return LocalTarget{}, fmt.Errorf("invalid comment id encoding")
		}
		if _, err := uuid.Parse(commentID); err != nil {
			return LocalTarget{}, fmt.Errorf("invalid local comment id")
		}
		target.ParentCommentID = &commentID
		target.TargetID = commentID
		target.TargetType = models.TargetComment
	}
	return target, nil
}

var blockHTMLNodes = map[string]bool{
	"address": true, "article": true, "blockquote": true, "div": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"li": true, "p": true, "pre": true, "section": true,
}

// PlainTextContent converts untrusted ActivityStreams HTML to local comment
// text. Script/style/template content is discarded rather than merely escaped.
func PlainTextContent(content string, maxRunes int) (string, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return "", fmt.Errorf("parse remote Note content: %w", err)
	}
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			name := strings.ToLower(node.Data)
			if name == "script" || name == "style" || name == "template" {
				return
			}
			if name == "br" {
				out.WriteByte('\n')
				return
			}
			if blockHTMLNodes[name] && out.Len() > 0 {
				out.WriteString("\n\n")
			}
		}
		if node.Type == html.TextNode {
			out.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.Type == html.ElementNode && blockHTMLNodes[strings.ToLower(node.Data)] {
			out.WriteString("\n\n")
		}
	}
	walk(doc)

	lines := strings.Split(strings.ReplaceAll(out.String(), "\u00a0", " "), "\n")
	normalized := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if len(normalized) > 0 && !blank {
				normalized = append(normalized, "")
				blank = true
			}
			continue
		}
		normalized = append(normalized, line)
		blank = false
	}
	for len(normalized) > 0 && normalized[len(normalized)-1] == "" {
		normalized = normalized[:len(normalized)-1]
	}
	plain := strings.TrimSpace(strings.Join(normalized, "\n"))
	if plain == "" {
		return "", fmt.Errorf("remote Note content is empty")
	}
	if maxRunes > 0 && utf8.RuneCountInString(plain) > maxRunes {
		return "", fmt.Errorf("remote Note content exceeds %d characters", maxRunes)
	}
	return plain, nil
}

// TrustWeight turns Loomfeed's locally computed 0-100 remote trust into a
// 0.05-1.0 vote contribution. The cold-start floor prevents a valid Like from
// being indistinguishable from no activity while keeping it far below a local
// full-weight vote.
func TrustWeight(localTrust float64) float64 {
	if math.IsNaN(localTrust) || math.IsInf(localTrust, 0) {
		return 0.05
	}
	return math.Min(1, math.Max(0.05, localTrust/100))
}

type RemoteActorProfile struct {
	URI               string
	PreferredUsername string
	DisplayName       string
	AvatarURL         string
	ActorType         string
	InboxURI          string
	LocalTrust        float64
}

type InboundReply struct {
	ActivityID      string
	ObjectID        string
	Actor           RemoteActorProfile
	PostID          string
	ParentCommentID *string
	Body            string
}

type InboundLike struct {
	ActivityID string
	Actor      RemoteActorProfile
	TargetID   string
	TargetType models.TargetType
	Weight     float64
}

type InboundRepo struct {
	pool *pgxpool.Pool
}

func NewInboundRepo(pool *pgxpool.Pool) *InboundRepo {
	return &InboundRepo{pool: pool}
}

func (r *InboundRepo) ensureRemoteActor(ctx context.Context, tx pgx.Tx, actor RemoteActorProfile) (string, error) {
	if actor.URI == "" {
		return "", fmt.Errorf("remote actor URI is required")
	}
	parsed, err := url.Parse(actor.URI)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid remote actor URI")
	}
	trust := math.Min(100, math.Max(0, actor.LocalTrust))
	displayName := strings.TrimSpace(actor.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(actor.PreferredUsername)
	}
	if displayName == "" {
		displayName = parsed.Host
	}
	identity := "@" + strings.TrimPrefix(actor.PreferredUsername, "@") + "@" + parsed.Host
	if actor.PreferredUsername != "" && !strings.Contains(displayName, identity) {
		displayName += " (" + identity + ")"
	}
	runes := []rune(displayName)
	if len(runes) > 100 {
		displayName = string(runes[:100])
	}
	actorType := actor.ActorType
	if actorType == "" {
		actorType = "Person"
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, actor.URI); err != nil {
		return "", fmt.Errorf("lock remote actor identity: %w", err)
	}

	var participantID string
	err = tx.QueryRow(ctx, `SELECT participant_id FROM ap_remote_actors WHERE actor_uri = $1 FOR UPDATE`, actor.URI).Scan(&participantID)
	if err != nil && err != pgx.ErrNoRows {
		return "", fmt.Errorf("lookup remote actor: %w", err)
	}
	if err == pgx.ErrNoRows {
		if err := tx.QueryRow(ctx, `
			INSERT INTO participants (type, display_name, avatar_url, bio, trust_score, reputation_score)
			VALUES ('remote', $1, NULLIF($2, ''), $3, $4, $4)
			RETURNING id`, displayName, actor.AvatarURL, "Federated actor from "+parsed.Host, trust,
		).Scan(&participantID); err != nil {
			return "", fmt.Errorf("create remote participant: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ap_remote_actors
				(participant_id, actor_uri, preferred_username, actor_type, inbox_uri, instance_host)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			participantID, actor.URI, actor.PreferredUsername, actorType, actor.InboxURI, parsed.Host,
		); err != nil {
			return "", fmt.Errorf("create remote actor mapping: %w", err)
		}
		return participantID, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE participants SET display_name = $2, avatar_url = NULLIF($3, ''),
			trust_score = $4, reputation_score = $4, updated_at = NOW()
		WHERE id = $1`, participantID, displayName, actor.AvatarURL, trust); err != nil {
		return "", fmt.Errorf("refresh remote participant: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ap_remote_actors SET preferred_username = $2, actor_type = $3,
			inbox_uri = $4, updated_at = NOW() WHERE participant_id = $1`,
		participantID, actor.PreferredUsername, actorType, actor.InboxURI); err != nil {
		return "", fmt.Errorf("refresh remote actor mapping: %w", err)
	}
	return participantID, nil
}

func (r *InboundRepo) IngestReply(ctx context.Context, reply InboundReply) (*models.Comment, bool, error) {
	if reply.ActivityID == "" || reply.ObjectID == "" || reply.PostID == "" || strings.TrimSpace(reply.Body) == "" {
		return nil, false, fmt.Errorf("incomplete federated reply")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin federated reply: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	participantID, err := r.ensureRemoteActor(ctx, tx, reply.Actor)
	if err != nil {
		return nil, false, err
	}
	depth := 0
	if reply.ParentCommentID != nil {
		if err := tx.QueryRow(ctx, `
			SELECT depth + 1 FROM comments
			WHERE id = $1 AND post_id = $2 AND deleted_at IS NULL`, *reply.ParentCommentID, reply.PostID,
		).Scan(&depth); err != nil {
			return nil, false, fmt.Errorf("resolve federated reply parent: %w", err)
		}
	}
	var comment models.Comment
	err = tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO comments
				(post_id, parent_comment_id, author_id, author_type, body, depth, thread_type,
				 federated_object_id, federated_activity_id, federated_actor_uri)
			VALUES ($1, $2, $3, 'remote', $4, $5, 'main', $6, $7, $8)
			ON CONFLICT (federated_object_id) WHERE federated_object_id IS NOT NULL DO NOTHING
			RETURNING id, post_id, parent_comment_id, author_id, author_type, body,
				vote_score, depth, is_answer, thread_type, created_at, updated_at
		), bump_post AS (
			UPDATE posts SET comment_count = comment_count + 1
			WHERE id = $1 AND EXISTS (SELECT 1 FROM inserted)
		), bump_participant AS (
			UPDATE participants SET comment_count = comment_count + 1
			WHERE id = $3 AND EXISTS (SELECT 1 FROM inserted)
		)
		SELECT * FROM inserted`,
		reply.PostID, reply.ParentCommentID, participantID, reply.Body, depth,
		reply.ObjectID, reply.ActivityID, reply.Actor.URI,
	).Scan(
		&comment.ID, &comment.PostID, &comment.ParentCommentID, &comment.AuthorID,
		&comment.AuthorType, &comment.Body, &comment.VoteScore, &comment.Depth,
		&comment.IsAnswer, &comment.ThreadType, &comment.CreatedAt, &comment.UpdatedAt,
	)
	created := true
	if err == pgx.ErrNoRows {
		created = false
		err = tx.QueryRow(ctx, `
			SELECT id, post_id, parent_comment_id, author_id, author_type, body,
				vote_score, depth, is_answer, thread_type, created_at, updated_at
			FROM comments WHERE federated_object_id = $1`, reply.ObjectID,
		).Scan(
			&comment.ID, &comment.PostID, &comment.ParentCommentID, &comment.AuthorID,
			&comment.AuthorType, &comment.Body, &comment.VoteScore, &comment.Depth,
			&comment.IsAnswer, &comment.ThreadType, &comment.CreatedAt, &comment.UpdatedAt,
		)
	}
	if err != nil {
		return nil, false, fmt.Errorf("insert federated reply: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit federated reply: %w", err)
	}
	return &comment, created, nil
}

func (r *InboundRepo) IngestLike(ctx context.Context, like InboundLike) (int, bool, error) {
	if like.ActivityID == "" || like.TargetID == "" || (like.TargetType != models.TargetPost && like.TargetType != models.TargetComment) {
		return 0, false, fmt.Errorf("incomplete federated Like")
	}
	if like.Weight <= 0 || like.Weight > 1 || math.IsNaN(like.Weight) || math.IsInf(like.Weight, 0) {
		return 0, false, fmt.Errorf("invalid federated Like weight")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("begin federated Like: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	participantID, err := r.ensureRemoteActor(ctx, tx, like.Actor)
	if err != nil {
		return 0, false, err
	}
	// Serialize score aggregation on the target. Without this lock, two Likes
	// from different remote actors can each recalculate against a snapshot that
	// does not include the other's uncommitted vote, leaving a lost score.
	var lockedTargetID string
	if like.TargetType == models.TargetPost {
		err = tx.QueryRow(ctx, `SELECT id FROM posts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, like.TargetID).Scan(&lockedTargetID)
	} else {
		err = tx.QueryRow(ctx, `SELECT id FROM comments WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, like.TargetID).Scan(&lockedTargetID)
	}
	if err != nil {
		return 0, false, fmt.Errorf("lock federated Like target: %w", err)
	}
	var voteID string
	err = tx.QueryRow(ctx, `
		INSERT INTO votes
			(target_id, target_type, voter_id, voter_type, direction, weight,
			 federated_activity_id, federated_actor_uri)
		VALUES ($1, $2, $3, 'remote', 'up', $4, $5, $6)
		ON CONFLICT DO NOTHING
		RETURNING id`, like.TargetID, like.TargetType, participantID, like.Weight, like.ActivityID, like.Actor.URI,
	).Scan(&voteID)
	created := true
	if err == pgx.ErrNoRows {
		created = false
		err = tx.QueryRow(ctx, `
			SELECT id FROM votes WHERE target_id = $1 AND target_type = $2 AND voter_id = $3`,
			like.TargetID, like.TargetType, participantID,
		).Scan(&voteID)
	}
	if err != nil {
		return 0, false, fmt.Errorf("insert federated Like: %w", err)
	}

	var score int
	if like.TargetType == models.TargetPost {
		err = tx.QueryRow(ctx, `
			UPDATE posts SET vote_score = ROUND(COALESCE((
				SELECT SUM(CASE WHEN direction = 'up' THEN weight ELSE -weight END)
				FROM votes WHERE target_id = $1 AND target_type = 'post'
			), 0))::integer
			WHERE id = $1 RETURNING vote_score`, like.TargetID).Scan(&score)
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE comments SET
				vote_score = ROUND(COALESCE((SELECT SUM(CASE WHEN direction = 'up' THEN weight ELSE -weight END) FROM votes WHERE target_id = $1 AND target_type = 'comment'), 0))::integer,
				upvote_count = ROUND(COALESCE((SELECT SUM(weight) FROM votes WHERE target_id = $1 AND target_type = 'comment' AND direction = 'up'), 0))::integer,
				downvote_count = ROUND(COALESCE((SELECT SUM(weight) FROM votes WHERE target_id = $1 AND target_type = 'comment' AND direction = 'down'), 0))::integer
			WHERE id = $1 RETURNING vote_score`, like.TargetID).Scan(&score)
	}
	if err != nil {
		return 0, false, fmt.Errorf("recalculate federated target score: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("commit federated Like: %w", err)
	}
	return score, created, nil
}
