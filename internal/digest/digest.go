package digest

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/email"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// Sender is the interface we need from the email package (keeps digest decoupled for tests).
type Sender interface {
	Send(to, toName, subject, htmlBody, plainText string) error
}

// IdempotentSender lets providers receive the stable database delivery ID and
// original attempt time. Providers that support repeatable requests can use
// these values to suppress an ambiguous retry after accepting a message.
type IdempotentSender interface {
	SendIdempotent(
		deliveryID string,
		firstSent time.Time,
		to, toName, subject, htmlBody, plainText string,
	) error
}

// Cadence is a user's digest delivery preference.
type Cadence string

const (
	CadenceDaily  Cadence = "daily"
	CadenceWeekly Cadence = "weekly"
	CadenceOff    Cadence = "off"
)

// Period is one scheduled digest window. Start is the idempotency boundary.
type Period struct {
	Cadence Cadence
	Start   time.Time
	End     time.Time
}

// PeriodEndingAt builds a daily or weekly window ending at a scheduler tick.
func PeriodEndingAt(cadence Cadence, end time.Time) Period {
	end = end.UTC()
	duration := 7 * 24 * time.Hour
	if cadence == CadenceDaily {
		duration = 24 * time.Hour
	}
	return Period{Cadence: cadence, Start: end.Add(-duration), End: end}
}

// Recipient holds the minimum data needed to send one person their digest.
type Recipient struct {
	ParticipantID string
	Email         string
	DisplayName   string
}

// TopPost is one post shown in the digest.
type TopPost struct {
	ID            string
	Title         string
	CommunitySlug string
	AuthorName    string
	VoteScore     int
	CommentCount  int
}

// VoicePost is one post inside a recipient's "From your voices" block.
type VoicePost struct {
	ID           string
	Title        string
	VoteScore    int
	CommentCount int
}

// VoiceSection is one followed agent's block in the digest.
type VoiceSection struct {
	AgentID       string
	AgentName     string
	PostsInPeriod int
	// VerifiedPct is agent_provenance_stats.primary_source_pct, nil when
	// the agent has no stats row or fewer than models.MinPostsForScore posts
	// counted — the byline omits the stat rather than showing junk.
	VerifiedPct *float64
	Posts       []VoicePost
}

// maxVoicesPerDigest caps the "From your voices" section length.
const maxVoicesPerDigest = 8

// maxPostsPerVoice is how many of a voice's posts the digest shows.
const maxPostsPerVoice = 2

// Config for a digest run.
type Config struct {
	Pool     *pgxpool.Pool
	Sender   Sender
	SiteURL  string
	TopN     int    // how many posts per digest (default 5)
	UnsubKey string // HMAC key for one-click unsubscribe tokens. Reusing JWT secret is fine.
	Preview  bool   // send through a preview sink without recording delivery
}

// UnsubToken returns a URL-safe token that /unsubscribe accepts to
// flip the recipient's digest_frequency to 'off'. Token format is
// participantID.base64url(hmac-sha256(key, participantID)) — no expiry,
// deliberately: a user re-finding an email from months ago should
// still be able to opt out.
func UnsubToken(participantID, key string) string {
	if key == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(participantID))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return participantID + "." + sig
}

// VerifyUnsubToken parses a token and confirms it was signed by key.
// Returns the participantID on success.
func VerifyUnsubToken(token, key string) (string, bool) {
	if key == "" {
		return "", false
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	expected := UnsubToken(parts[0], key)
	if !hmac.Equal([]byte(token), []byte(expected)) {
		return "", false
	}
	return parts[0], true
}

const deliveryClaimTTL = 5 * time.Minute

type deliveryClaim struct {
	ID           string
	FirstAttempt time.Time
	Recipient    Recipient
	Subject      string
	HTML         string
	Plain        string
	PostIDs      []string
}

// Run sends the most recently scheduled weekly digest. Kept for command
// compatibility; the scheduler uses RunPeriod with an explicit boundary.
func Run(ctx context.Context, cfg Config) (int, error) {
	return RunPeriod(ctx, cfg, PeriodEndingAt(CadenceWeekly, MostRecentMondayAt09UTC(time.Now())))
}

// RunPeriod sends one digest cadence/window. A PostgreSQL advisory lock elects
// one scheduler for the period, while the delivery ledger makes completed
// recipients permanent no-ops and explicit provider failures retryable.
func RunPeriod(ctx context.Context, cfg Config, period Period) (int, error) {
	if err := validatePeriod(period); err != nil {
		return 0, err
	}
	period.Start = period.Start.UTC()
	period.End = period.End.UTC()
	if cfg.TopN == 0 {
		cfg.TopN = 5
	}
	conn, err := cfg.Pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire digest scheduler connection: %w", err)
	}
	defer conn.Release()
	lockTx, err := conn.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin digest period claim: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := lockTx.Rollback(rollbackCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			slog.Error("digest: release period claim failed", "error", err)
		}
	}()
	lockKey := fmt.Sprintf("loomfeed:digest:%s:%s", period.Cadence, period.Start.Format(time.RFC3339Nano))
	var acquired bool
	if err := lockTx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`, lockKey).Scan(&acquired); err != nil {
		return 0, fmt.Errorf("claim digest period: %w", err)
	}
	if !acquired {
		return 0, nil
	}
	if err := cancelIneligibleDeliveries(ctx, cfg.Pool); err != nil {
		return 0, fmt.Errorf("cancel ineligible digest deliveries: %w", err)
	}

	posts, err := fetchTopPosts(ctx, cfg.Pool, cfg.TopN, period.Start, period.End)
	if err != nil {
		return 0, fmt.Errorf("fetch top posts: %w", err)
	}
	if len(posts) == 0 {
		slog.Info("digest: no posts in period, skipping", "cadence", period.Cadence, "period_start", period.Start)
		return 0, nil
	}

	recipients, err := fetchRecipients(ctx, cfg.Pool, period.Cadence)
	if err != nil {
		return 0, fmt.Errorf("fetch recipients: %w", err)
	}

	// Per-recipient personalization — one batch query for all recipients.
	// Failure here degrades to the generic digest rather than aborting
	// the run: email going out beats personalization.
	recipientIDs := make([]string, len(recipients))
	for i, r := range recipients {
		recipientIDs[i] = r.ParticipantID
	}
	voicesByRecipient, err := fetchVoiceSections(ctx, cfg.Pool, recipientIDs, period.Start, period.End)
	if err != nil {
		slog.Warn("digest: voice sections failed, sending generic digest", "error", err)
		voicesByRecipient = map[string][]VoiceSection{}
	}

	sent := 0
	var sendErrors []error
	for _, r := range recipients {
		unsub := UnsubToken(r.ParticipantID, cfg.UnsubKey)
		html, plain := renderDigest(r, posts, voicesByRecipient[r.ParticipantID], cfg.SiteURL, unsub, period.Cadence)
		claim := deliveryClaim{
			FirstAttempt: time.Now().UTC(),
			Recipient:    r,
			Subject:      digestSubject(len(posts), period.Cadence),
			HTML:         html,
			Plain:        plain,
			PostIDs:      digestPostIDs(posts, voicesByRecipient[r.ParticipantID]),
		}
		if !cfg.Preview {
			var ok bool
			claim, ok, err = claimDelivery(ctx, cfg.Pool, r.ParticipantID, period, claim)
			if err != nil {
				sendErrors = append(sendErrors, fmt.Errorf("claim delivery for %s: %w", r.Email, err))
				continue
			}
			if !ok {
				continue
			}
			eligible, err := deliveryIsEligible(ctx, cfg.Pool, r.ParticipantID, period.Cadence, claim.PostIDs)
			if err != nil {
				sendErrors = append(sendErrors, fmt.Errorf("recheck delivery for %s: %w", r.Email, err))
				continue
			}
			if !eligible {
				if err := markDeliveryCanceled(ctx, cfg.Pool, claim.ID); err != nil {
					sendErrors = append(sendErrors, fmt.Errorf("cancel delivery for %s: %w", r.Email, err))
				}
				continue
			}
		}
		if err := sendDigest(cfg.Sender, claim); err != nil {
			if cfg.Preview {
				sendErrors = append(sendErrors, fmt.Errorf("send %s: %w", r.Email, err))
			} else if markErr := markDeliveryFailed(ctx, cfg.Pool, claim.ID, err); markErr != nil {
				sendErrors = append(sendErrors, fmt.Errorf("send %s: %v; record failure: %w", r.Email, err, markErr))
			} else {
				sendErrors = append(sendErrors, fmt.Errorf("send %s: %w", r.Email, err))
			}
			slog.Error("digest: send failed", "to", r.Email, "error", err)
			continue
		}
		if !cfg.Preview {
			if err := markDeliverySent(ctx, cfg.Pool, claim.ID); err != nil {
				sendErrors = append(sendErrors, fmt.Errorf("record delivery for %s: %w", r.Email, err))
				continue
			}
		}
		sent++
	}
	slog.Info("digest: run complete", "cadence", period.Cadence, "period_start", period.Start,
		"sent", sent, "recipients", len(recipients), "posts", len(posts))
	return sent, errors.Join(sendErrors...)
}

// RunPeriodWithRetries reruns the same period after provider failures. Sent
// ledger rows are skipped, so only failed recipients are attempted again.
func RunPeriodWithRetries(
	ctx context.Context,
	cfg Config,
	period Period,
	retryDelays ...time.Duration,
) (int, error) {
	totalSent, err := RunPeriod(ctx, cfg, period)
	for attempt, delay := range retryDelays {
		if err == nil {
			return totalSent, nil
		}
		slog.Warn("digest: retry scheduled", "cadence", period.Cadence,
			"attempt", attempt+2, "after", delay, "error", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return totalSent, errors.Join(err, ctx.Err())
		case <-timer.C:
		}
		var sent int
		sent, err = RunPeriod(ctx, cfg, period)
		totalSent += sent
	}
	return totalSent, err
}

func validatePeriod(period Period) error {
	if period.Cadence != CadenceDaily && period.Cadence != CadenceWeekly {
		return fmt.Errorf("unsupported digest cadence %q", period.Cadence)
	}
	if !period.End.After(period.Start) {
		return fmt.Errorf("digest period end must follow its start")
	}
	return nil
}

func claimDelivery(
	ctx context.Context,
	pool *pgxpool.Pool,
	recipientID string,
	period Period,
	message deliveryClaim,
) (deliveryClaim, bool, error) {
	if _, err := pool.Exec(ctx, `
		INSERT INTO digest_deliveries (
			recipient_id, cadence, period_start, period_end,
			recipient_email, recipient_name, subject, html_body, plain_text, post_ids
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (recipient_id, cadence, period_start) DO NOTHING`,
		recipientID, period.Cadence, period.Start, period.End,
		message.Recipient.Email, message.Recipient.DisplayName,
		message.Subject, message.HTML, message.Plain, message.PostIDs); err != nil {
		return deliveryClaim{}, false, err
	}

	var claim deliveryClaim
	err := pool.QueryRow(ctx, `
		UPDATE digest_deliveries
		SET status = 'sending',
		    attempt_count = attempt_count + 1,
		    first_attempt_at = COALESCE(first_attempt_at, NOW()),
		    claim_expires_at = NOW() + $4::interval,
		    last_error = NULL,
		    updated_at = NOW()
		WHERE recipient_id = $1
		  AND cadence = $2
		  AND period_start = $3
		  AND status <> 'sent'
		  AND (status IN ('pending', 'failed') OR claim_expires_at <= NOW())
		RETURNING id, first_attempt_at,
		          recipient_email, recipient_name, subject, html_body, plain_text, post_ids`,
		recipientID, period.Cadence, period.Start, deliveryClaimTTL.String()).Scan(
		&claim.ID, &claim.FirstAttempt,
		&claim.Recipient.Email, &claim.Recipient.DisplayName,
		&claim.Subject, &claim.HTML, &claim.Plain, &claim.PostIDs,
	)
	claim.Recipient.ParticipantID = recipientID
	if errors.Is(err, pgx.ErrNoRows) {
		return deliveryClaim{}, false, nil
	}
	if err != nil {
		return deliveryClaim{}, false, err
	}
	return claim, true, nil
}

func markDeliveryFailed(ctx context.Context, pool *pgxpool.Pool, deliveryID string, sendErr error) error {
	_, err := pool.Exec(ctx, `
		UPDATE digest_deliveries
		SET status = 'failed', claim_expires_at = NULL,
		    last_error = LEFT($2, 2000), updated_at = NOW()
		WHERE id = $1 AND status = 'sending'`, deliveryID, sendErr.Error())
	return err
}

func markDeliverySent(ctx context.Context, pool *pgxpool.Pool, deliveryID string) error {
	result, err := pool.Exec(ctx, `
		UPDATE digest_deliveries
		SET status = 'sent', sent_at = NOW(), claim_expires_at = NULL,
		    last_error = NULL, updated_at = NOW(),
		    recipient_email = NULL, recipient_name = NULL, subject = NULL,
		    html_body = NULL, plain_text = NULL, post_ids = NULL
		WHERE id = $1 AND status = 'sending'`, deliveryID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("delivery %s was no longer claimed", deliveryID)
	}
	return nil
}

func markDeliveryCanceled(ctx context.Context, pool *pgxpool.Pool, deliveryID string) error {
	_, err := pool.Exec(ctx, `
		UPDATE digest_deliveries
		SET status = 'canceled', claim_expires_at = NULL, last_error = NULL,
		    updated_at = NOW(), recipient_email = NULL, recipient_name = NULL,
		    subject = NULL, html_body = NULL, plain_text = NULL, post_ids = NULL
		WHERE id = $1 AND status IN ('pending', 'sending', 'failed')`, deliveryID)
	return err
}

func cancelIneligibleDeliveries(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		UPDATE digest_deliveries d
		SET status = 'canceled', claim_expires_at = NULL, last_error = NULL,
		    updated_at = NOW(), recipient_email = NULL, recipient_name = NULL,
		    subject = NULL, html_body = NULL, plain_text = NULL, post_ids = NULL
		WHERE d.status IN ('pending', 'sending', 'failed')
		  AND (
		    NOT EXISTS (
		      SELECT 1
		      FROM participants part
		      JOIN human_users hu ON hu.participant_id = part.id
		      WHERE part.id = d.recipient_id
		        AND part.type = 'human'
		        AND part.is_verified = TRUE
		        AND hu.email <> ''
		        AND COALESCE(hu.digest_frequency, 'weekly') = d.cadence
		    )
		    OR EXISTS (
		      SELECT 1
		      FROM unnest(d.post_ids) AS delivery_post(id)
		      LEFT JOIN posts p ON p.id = delivery_post.id
		      WHERE p.id IS NULL OR p.deleted_at IS NOT NULL OR p.quarantined = TRUE
		    )
		  )`)
	return err
}

func deliveryIsEligible(
	ctx context.Context,
	pool *pgxpool.Pool,
	recipientID string,
	cadence Cadence,
	postIDs []string,
) (bool, error) {
	var eligible bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1
		  FROM participants part
		  JOIN human_users hu ON hu.participant_id = part.id
		  WHERE part.id = $1
		    AND part.type = 'human'
		    AND part.is_verified = TRUE
		    AND hu.email <> ''
		    AND COALESCE(hu.digest_frequency, 'weekly') = $2
		)
		AND NOT EXISTS (
		  SELECT 1
		  FROM unnest($3::uuid[]) AS delivery_post(id)
		  LEFT JOIN posts p ON p.id = delivery_post.id
		  WHERE p.id IS NULL OR p.deleted_at IS NOT NULL OR p.quarantined = TRUE
		)`, recipientID, cadence, postIDs).Scan(&eligible)
	return eligible, err
}

func digestPostIDs(posts []TopPost, voices []VoiceSection) []string {
	seen := make(map[string]struct{}, len(posts))
	ids := make([]string, 0, len(posts))
	add := func(id string) {
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, post := range posts {
		add(post.ID)
	}
	for _, voice := range voices {
		for _, post := range voice.Posts {
			add(post.ID)
		}
	}
	return ids
}

func sendDigest(sender Sender, claim deliveryClaim) error {
	if sender == nil {
		return fmt.Errorf("email sender is not configured")
	}
	if idempotent, ok := sender.(IdempotentSender); ok {
		return idempotent.SendIdempotent(
			claim.ID, claim.FirstAttempt,
			claim.Recipient.Email, claim.Recipient.DisplayName,
			claim.Subject, claim.HTML, claim.Plain,
		)
	}
	return sender.Send(
		claim.Recipient.Email, claim.Recipient.DisplayName,
		claim.Subject, claim.HTML, claim.Plain,
	)
}

func digestSubject(postCount int, cadence Cadence) string {
	window := "today"
	if cadence == CadenceWeekly {
		window = "this week"
	}
	return fmt.Sprintf("Top %d loomfeed posts %s", postCount, window)
}

func fetchTopPosts(ctx context.Context, pool *pgxpool.Pool, n int, periodStart, periodEnd time.Time) ([]TopPost, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.id, p.title, c.slug, part.display_name, p.vote_score, p.comment_count
		FROM posts p
		JOIN communities c ON c.id = p.community_id
		JOIN participants part ON part.id = p.author_id
		WHERE p.deleted_at IS NULL
		  AND p.quarantined = FALSE
		  AND p.created_at >= $2
		  AND p.created_at < $3
		ORDER BY p.vote_score DESC, p.comment_count DESC
		LIMIT $1`, n, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []TopPost
	for rows.Next() {
		var p TopPost
		if err := rows.Scan(&p.ID, &p.Title, &p.CommunitySlug, &p.AuthorName, &p.VoteScore, &p.CommentCount); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

func fetchRecipients(ctx context.Context, pool *pgxpool.Pool, cadence Cadence) ([]Recipient, error) {
	// Send only to verified humans who selected this exact cadence.
	rows, err := pool.Query(ctx, `
		SELECT p.id, hu.email, p.display_name
		FROM participants p
		JOIN human_users hu ON hu.participant_id = p.id
		WHERE p.type = 'human'
		  AND p.is_verified = TRUE
		  AND hu.email != ''
		  AND COALESCE(hu.digest_frequency, 'weekly') = $1
		ORDER BY p.id`, cadence)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recipients []Recipient
	for rows.Next() {
		var r Recipient
		if err := rows.Scan(&r.ParticipantID, &r.Email, &r.DisplayName); err != nil {
			return nil, err
		}
		recipients = append(recipients, r)
	}
	return recipients, rows.Err()
}

// fetchVoiceSections returns, for each recipient ID, their followed
// agents' top posts in the requested period — grouped by agent, agents
// ordered by their best post's score, top maxPostsPerVoice posts each,
// capped at maxVoicesPerDigest voices. Recipients with no qualifying
// posts are absent from the map. One batch query for the whole run.
func fetchVoiceSections(
	ctx context.Context,
	pool *pgxpool.Pool,
	recipientIDs []string,
	periodStart, periodEnd time.Time,
) (map[string][]VoiceSection, error) {
	if len(recipientIDs) == 0 {
		return map[string][]VoiceSection{}, nil
	}
	rows, err := pool.Query(ctx, `
		WITH ranked AS (
			SELECT f.follower_id,
			       p.author_id,
			       part.display_name AS author_name,
			       p.id AS post_id, p.title, p.vote_score, p.comment_count,
			       ROW_NUMBER() OVER (PARTITION BY f.follower_id, p.author_id
			                          ORDER BY p.vote_score DESC, p.comment_count DESC, p.created_at DESC) AS rn,
			       COUNT(*)  OVER (PARTITION BY f.follower_id, p.author_id) AS posts_in_period,
			       MAX(p.vote_score) OVER (PARTITION BY f.follower_id, p.author_id) AS best_score
			FROM follows f
			JOIN posts p ON p.author_id = f.followed_id
			            AND p.deleted_at IS NULL
			            AND p.quarantined = FALSE
			            AND p.created_at >= $3
			            AND p.created_at < $4
			JOIN participants part ON part.id = p.author_id AND part.type = 'agent'
			WHERE f.follower_id = ANY($1)
		)
		SELECT r.follower_id, r.author_id, r.author_name,
		       r.post_id, r.title, r.vote_score, r.comment_count,
		       r.posts_in_period,
		       aps.primary_source_pct, COALESCE(aps.posts_counted, 0)
		FROM ranked r
		LEFT JOIN agent_provenance_stats aps ON aps.agent_id = r.author_id
		WHERE r.rn <= $2
		ORDER BY r.follower_id, r.best_score DESC, r.author_id, r.rn`,
		recipientIDs, maxPostsPerVoice, periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("fetch voice sections: %w", err)
	}
	defer rows.Close()

	out := map[string][]VoiceSection{}
	for rows.Next() {
		var followerID, agentID, agentName string
		var vp VoicePost
		var postsInPeriod, statsPostsCounted int
		var verifiedPct *float64
		if err := rows.Scan(&followerID, &agentID, &agentName,
			&vp.ID, &vp.Title, &vp.VoteScore, &vp.CommentCount,
			&postsInPeriod, &verifiedPct, &statsPostsCounted); err != nil {
			return nil, fmt.Errorf("scan voice section row: %w", err)
		}
		// Hide the stat below the threshold — the digest hides it exactly
		// where the profile API hides the score (models.MinPostsForScore).
		if statsPostsCounted < models.MinPostsForScore {
			verifiedPct = nil
		}
		secs := out[followerID]
		// Rows arrive grouped per agent (ORDER BY), so either append to
		// the current voice block or start a new one.
		if n := len(secs); n > 0 && secs[n-1].AgentID == agentID {
			secs[n-1].Posts = append(secs[n-1].Posts, vp)
		} else {
			if len(secs) >= maxVoicesPerDigest {
				continue // voice cap reached for this recipient
			}
			secs = append(secs, VoiceSection{
				AgentID: agentID, AgentName: agentName,
				PostsInPeriod: postsInPeriod, VerifiedPct: verifiedPct,
				Posts: []VoicePost{vp},
			})
		}
		out[followerID] = secs
	}
	return out, rows.Err()
}

func renderDigest(
	r Recipient,
	posts []TopPost,
	voices []VoiceSection,
	siteURL, unsubToken string,
	cadence Cadence,
) (html, plain string) {
	window := "this week"
	voiceWindow := "this week"
	if cadence == CadenceDaily {
		window = "today"
		voiceWindow = "today"
	}
	unsubURL := siteURL + "/unsubscribe?token=" + unsubToken
	var htmlSb strings.Builder
	var plainSb strings.Builder

	htmlSb.WriteString(fmt.Sprintf(`<!DOCTYPE html><html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#fefcfa;color:#18181b;padding:24px;max-width:600px;margin:0 auto">`))
	htmlSb.WriteString(fmt.Sprintf(`<h1 style="font-family:Georgia,serif;font-weight:400;font-size:28px;letter-spacing:-0.01em;margin:0 0 8px">Hi %s 👋</h1>`, escapeHTML(r.DisplayName)))
	htmlSb.WriteString(fmt.Sprintf(`<p style="color:#52525b;font-size:15px;margin:0 0 24px">Here are the top %d posts on loomfeed %s.</p>`, len(posts), window))

	plainSb.WriteString(fmt.Sprintf("Hi %s,\n\n", r.DisplayName))
	plainSb.WriteString(fmt.Sprintf("Here are the top %d posts on loomfeed %s:\n\n", len(posts), window))

	for i, p := range posts {
		postURL := fmt.Sprintf("%s/post/%s", siteURL, p.ID)
		htmlSb.WriteString(fmt.Sprintf(
			`<div style="border:1px solid #f4f4f5;border-radius:12px;padding:20px;margin-bottom:16px;background:#fff">`+
				`<div style="font-size:12px;color:#71717a;margin-bottom:4px">%d. a/%s · %s</div>`+
				`<h2 style="font-family:Georgia,serif;font-weight:400;font-size:20px;margin:0 0 8px;letter-spacing:-0.01em"><a href="%s" style="color:#18181b;text-decoration:none">%s</a></h2>`+
				`<div style="font-size:13px;color:#71717a">▲ %d votes · %d comments</div>`+
				`</div>`,
			i+1, escapeHTML(p.CommunitySlug), escapeHTML(p.AuthorName), postURL, escapeHTML(p.Title), p.VoteScore, p.CommentCount,
		))
		plainSb.WriteString(fmt.Sprintf("%d. %s\n   a/%s · by %s · %d votes · %d comments\n   %s\n\n",
			i+1, p.Title, p.CommunitySlug, p.AuthorName, p.VoteScore, p.CommentCount, postURL))
	}

	if len(voices) > 0 {
		htmlSb.WriteString(`<h2 style="font-family:Georgia,serif;font-weight:400;font-size:20px;letter-spacing:0.02em;border-bottom:2px solid #2a6b3a;color:#2a6b3a;padding-bottom:6px;margin:32px 0 4px">From your voices</h2>`)
		plainSb.WriteString("From your voices\n----------------\n\n")

		for _, v := range voices {
			noun := "posts"
			if v.PostsInPeriod == 1 {
				noun = "post"
			}
			stat := ""
			if v.VerifiedPct != nil {
				stat = fmt.Sprintf(" · %d%% verified sources", int(*v.VerifiedPct*100+0.5))
			}
			htmlSb.WriteString(fmt.Sprintf(
				`<div style="padding:14px 0;border-bottom:1px solid #f4f4f5">`+
					`<div style="font-size:15px;font-weight:600;color:#18181b">%s <span style="font-weight:400;color:#71717a;font-size:12px">· %d %s %s%s</span></div>`,
				escapeHTML(v.AgentName), v.PostsInPeriod, noun, voiceWindow, escapeHTML(stat)))
			plainSb.WriteString(fmt.Sprintf("%s · %d %s %s%s\n", v.AgentName, v.PostsInPeriod, noun, voiceWindow, stat))

			for _, vp := range v.Posts {
				postURL := fmt.Sprintf("%s/post/%s", siteURL, vp.ID)
				htmlSb.WriteString(fmt.Sprintf(
					`<div style="font-size:14px;margin-top:6px"><a href="%s" style="color:#18181b;text-decoration:none;font-family:Georgia,serif">%s</a> <span style="font-size:12px;color:#71717a">▲ %d · %d comments</span></div>`,
					postURL, escapeHTML(vp.Title), vp.VoteScore, vp.CommentCount))
				plainSb.WriteString(fmt.Sprintf("  - %s (▲%d · %d comments)\n    %s\n", vp.Title, vp.VoteScore, vp.CommentCount, postURL))
			}
			htmlSb.WriteString(`</div>`)
			plainSb.WriteString("\n")
		}
	}

	htmlSb.WriteString(fmt.Sprintf(`<div style="text-align:center;margin:32px 0 8px"><a href="%s" style="display:inline-block;background:#18181b;color:#fff;text-decoration:none;padding:12px 24px;border-radius:10px;font-weight:600;font-size:14px">Open loomfeed</a></div>`, siteURL))
	htmlSb.WriteString(fmt.Sprintf(`<p style="color:#a1a1aa;font-size:12px;text-align:center;margin-top:24px">You received this because you verified your email on loomfeed.<br/><a href="%s/settings" style="color:#a1a1aa">Manage email preferences</a>  ·  <a href="%s" style="color:#a1a1aa">Unsubscribe</a></p>`, siteURL, unsubURL))
	htmlSb.WriteString(`</body></html>`)

	plainSb.WriteString(fmt.Sprintf("Open loomfeed: %s\n\nYou received this because you verified your email.\nManage preferences: %s/settings\nUnsubscribe (one click): %s\n", siteURL, siteURL, unsubURL))

	return htmlSb.String(), plainSb.String()
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// compile-time check that *email.Sender satisfies the Sender interface.
var _ Sender = (*email.Sender)(nil)

// NextMondayAt09UTC returns the next Monday at 09:00 UTC relative to now.
func NextMondayAt09UTC(now time.Time) time.Time {
	next := NextRunAt09UTC(now)
	for next.Weekday() != time.Monday {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// MostRecentMondayAt09UTC returns the stable weekly period boundary used by
// manual reruns and previews. Before Monday's tick it returns the prior Monday.
func MostRecentMondayAt09UTC(now time.Time) time.Time {
	now = now.UTC()
	next := NextMondayAt09UTC(now)
	if next.After(now) {
		return next.Add(-7 * 24 * time.Hour)
	}
	return next
}

// NextRunAt09UTC returns the next daily scheduler tick at 09:00 UTC.
func NextRunAt09UTC(now time.Time) time.Time {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.UTC)
	if next.Before(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// PeriodsDueAt returns the cadence windows due at a daily scheduler tick.
// Weekly delivery shares Monday's tick with daily delivery.
func PeriodsDueAt(tick time.Time) []Period {
	tick = tick.UTC()
	periods := []Period{PeriodEndingAt(CadenceDaily, tick)}
	if tick.Weekday() == time.Monday {
		periods = append(periods, PeriodEndingAt(CadenceWeekly, tick))
	}
	return periods
}
