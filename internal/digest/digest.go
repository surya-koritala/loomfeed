package digest

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/RoamXAI/loomfeed/internal/email"
	"github.com/RoamXAI/loomfeed/internal/models"
)

// Sender is the interface we need from the email package (keeps digest decoupled for tests).
type Sender interface {
	Send(to, toName, subject, htmlBody, plainText string) error
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
	PostsThisWeek int
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
	Pool       *pgxpool.Pool
	Sender     Sender
	SiteURL    string
	TopN       int    // how many posts per digest (default 5)
	UnsubKey   string // HMAC key for one-click unsubscribe tokens. Reusing JWT secret is fine.
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

// Run sends a weekly digest to every human user with a verified email.
// Picks the top N posts of the last 7 days by vote_score.
func Run(ctx context.Context, cfg Config) (int, error) {
	if cfg.TopN == 0 {
		cfg.TopN = 5
	}

	// Fetch top posts from last 7 days
	posts, err := fetchTopPosts(ctx, cfg.Pool, cfg.TopN)
	if err != nil {
		return 0, fmt.Errorf("fetch top posts: %w", err)
	}
	if len(posts) == 0 {
		slog.Info("digest: no posts to send this week, skipping")
		return 0, nil
	}

	// Fetch recipients (humans with verified email)
	recipients, err := fetchRecipients(ctx, cfg.Pool)
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
	voicesByRecipient, err := fetchVoiceSections(ctx, cfg.Pool, recipientIDs)
	if err != nil {
		slog.Warn("digest: voice sections failed, sending generic digest", "error", err)
		voicesByRecipient = map[string][]VoiceSection{}
	}

	sent := 0
	for _, r := range recipients {
		unsub := UnsubToken(r.ParticipantID, cfg.UnsubKey)
		html, plain := renderDigest(r, posts, voicesByRecipient[r.ParticipantID], cfg.SiteURL, unsub)
		subject := fmt.Sprintf("Top %d loomfeed posts this week", len(posts))
		if err := cfg.Sender.Send(r.Email, r.DisplayName, subject, html, plain); err != nil {
			slog.Error("digest: send failed", "to", r.Email, "error", err)
			continue
		}
		sent++
	}
	slog.Info("digest: run complete", "sent", sent, "recipients", len(recipients), "posts", len(posts))
	return sent, nil
}

func fetchTopPosts(ctx context.Context, pool *pgxpool.Pool, n int) ([]TopPost, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.id, p.title, c.slug, part.display_name, p.vote_score, p.comment_count
		FROM posts p
		JOIN communities c ON c.id = p.community_id
		JOIN participants part ON part.id = p.author_id
		WHERE p.deleted_at IS NULL
		  AND p.created_at > NOW() - INTERVAL '7 days'
		ORDER BY p.vote_score DESC, p.comment_count DESC
		LIMIT $1`, n)
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

func fetchRecipients(ctx context.Context, pool *pgxpool.Pool) ([]Recipient, error) {
	// Send to humans who have verified their email and haven't opted out.
	// digest_frequency='off' excludes users who clicked the unsub link or
	// toggled the preference in settings. Weekly/daily both pass here —
	// cadence is enforced at the scheduler, not per-user.
	rows, err := pool.Query(ctx, `
		SELECT p.id, hu.email, p.display_name
		FROM participants p
		JOIN human_users hu ON hu.participant_id = p.id
		WHERE p.type = 'human'
		  AND p.is_verified = TRUE
		  AND hu.email != ''
		  AND COALESCE(hu.digest_frequency, 'weekly') <> 'off'
		ORDER BY p.id`)
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
// agents' top posts of the last 7 days — grouped by agent, agents
// ordered by their best post's score, top maxPostsPerVoice posts each,
// capped at maxVoicesPerDigest voices. Recipients with no qualifying
// posts are absent from the map. One batch query for the whole run.
func fetchVoiceSections(ctx context.Context, pool *pgxpool.Pool, recipientIDs []string) (map[string][]VoiceSection, error) {
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
			       COUNT(*)  OVER (PARTITION BY f.follower_id, p.author_id) AS posts_this_week,
			       MAX(p.vote_score) OVER (PARTITION BY f.follower_id, p.author_id) AS best_score
			FROM follows f
			JOIN posts p ON p.author_id = f.followed_id
			            AND p.deleted_at IS NULL
			            AND p.created_at > NOW() - INTERVAL '7 days'
			JOIN participants part ON part.id = p.author_id AND part.type = 'agent'
			WHERE f.follower_id = ANY($1)
		)
		SELECT r.follower_id, r.author_id, r.author_name,
		       r.post_id, r.title, r.vote_score, r.comment_count,
		       r.posts_this_week,
		       aps.primary_source_pct, COALESCE(aps.posts_counted, 0)
		FROM ranked r
		LEFT JOIN agent_provenance_stats aps ON aps.agent_id = r.author_id
		WHERE r.rn <= $2
		ORDER BY r.follower_id, r.best_score DESC, r.author_id, r.rn`,
		recipientIDs, maxPostsPerVoice)
	if err != nil {
		return nil, fmt.Errorf("fetch voice sections: %w", err)
	}
	defer rows.Close()

	out := map[string][]VoiceSection{}
	for rows.Next() {
		var followerID, agentID, agentName string
		var vp VoicePost
		var postsThisWeek, statsPostsCounted int
		var verifiedPct *float64
		if err := rows.Scan(&followerID, &agentID, &agentName,
			&vp.ID, &vp.Title, &vp.VoteScore, &vp.CommentCount,
			&postsThisWeek, &verifiedPct, &statsPostsCounted); err != nil {
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
				PostsThisWeek: postsThisWeek, VerifiedPct: verifiedPct,
				Posts: []VoicePost{vp},
			})
		}
		out[followerID] = secs
	}
	return out, rows.Err()
}

func renderDigest(r Recipient, posts []TopPost, voices []VoiceSection, siteURL, unsubToken string) (html, plain string) {
	unsubURL := siteURL + "/unsubscribe?token=" + unsubToken
	var htmlSb strings.Builder
	var plainSb strings.Builder

	htmlSb.WriteString(fmt.Sprintf(`<!DOCTYPE html><html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#fefcfa;color:#18181b;padding:24px;max-width:600px;margin:0 auto">`))
	htmlSb.WriteString(fmt.Sprintf(`<h1 style="font-family:Georgia,serif;font-weight:400;font-size:28px;letter-spacing:-0.01em;margin:0 0 8px">Hi %s 👋</h1>`, escapeHTML(r.DisplayName)))
	htmlSb.WriteString(fmt.Sprintf(`<p style="color:#52525b;font-size:15px;margin:0 0 24px">Here are the top %d posts on loomfeed this week.</p>`, len(posts)))

	plainSb.WriteString(fmt.Sprintf("Hi %s,\n\n", r.DisplayName))
	plainSb.WriteString(fmt.Sprintf("Here are the top %d posts on loomfeed this week:\n\n", len(posts)))

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
			if v.PostsThisWeek == 1 {
				noun = "post"
			}
			stat := ""
			if v.VerifiedPct != nil {
				stat = fmt.Sprintf(" · %d%% verified sources", int(*v.VerifiedPct*100+0.5))
			}
			htmlSb.WriteString(fmt.Sprintf(
				`<div style="padding:14px 0;border-bottom:1px solid #f4f4f5">`+
					`<div style="font-size:15px;font-weight:600;color:#18181b">%s <span style="font-weight:400;color:#71717a;font-size:12px">· %d %s this week%s</span></div>`,
				escapeHTML(v.AgentName), v.PostsThisWeek, noun, escapeHTML(stat)))
			plainSb.WriteString(fmt.Sprintf("%s · %d %s this week%s\n", v.AgentName, v.PostsThisWeek, noun, stat))

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
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.UTC)
	for next.Before(now) || next.Weekday() != time.Monday {
		next = next.Add(24 * time.Hour)
	}
	return next
}
