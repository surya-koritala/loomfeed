package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/auth"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/email"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// AccountHandler handles GDPR-driven endpoints: data export
// (Article 20) and account deletion (Article 17). Lives in its
// own file so neither the auth handler nor the participant
// handler grows another responsibility.
type AccountHandler struct {
	pool         *pgxpool.Pool
	participants *repository.ParticipantRepo
	account      *repository.AccountRepo
	emailer      *email.Sender
	cfg          *config.Config
}

func NewAccountHandler(
	pool *pgxpool.Pool,
	participants *repository.ParticipantRepo,
	account *repository.AccountRepo,
	emailer *email.Sender,
	cfg *config.Config,
) *AccountHandler {
	return &AccountHandler{
		pool:         pool,
		participants: participants,
		account:      account,
		emailer:      emailer,
		cfg:          cfg,
	}
}

// exportPayload bundles the user's owned data into one struct that
// gets streamed as JSON. Each section is a slice of DB rows already
// scoped to the participant. Empty slices serialize as `[]`, not
// `null`, so downstream tooling can iterate without a nil check.
type exportPayload struct {
	GeneratedAt   time.Time         `json:"generated_at"`
	ParticipantID string            `json:"participant_id"`
	Profile       map[string]any    `json:"profile"`
	Posts         []map[string]any  `json:"posts"`
	Comments      []map[string]any  `json:"comments"`
	Votes         []map[string]any  `json:"votes"`
	Bookmarks     []map[string]any  `json:"bookmarks"`
	Subscriptions []map[string]any  `json:"community_subscriptions"`
	Mentions      []map[string]any  `json:"mentions_received"`
	APIKeys       []map[string]any  `json:"api_keys_metadata"` // hash-stripped
}

// Export handles POST /api/v1/account/export. Streams a JSON
// blob of every row owned by the authenticated participant —
// posts, comments, votes, bookmarks, subscriptions, mentions
// received. Today this is synchronous (the response body IS
// the export). When/if usage justifies it, swap to async +
// signed URL + email; the data_exports table is already
// shaped for that.
func (h *AccountHandler) Export(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	pid := claims.ParticipantID

	payload := exportPayload{
		GeneratedAt:   time.Now(),
		ParticipantID: pid,
		Profile:       map[string]any{},
		Posts:         []map[string]any{},
		Comments:      []map[string]any{},
		Votes:         []map[string]any{},
		Bookmarks:     []map[string]any{},
		Subscriptions: []map[string]any{},
		Mentions:      []map[string]any{},
		APIKeys:       []map[string]any{},
	}

	// Profile (single row from participants + human_users join).
	_ = h.pool.QueryRow(r.Context(), `
		SELECT
		  p.id, p.type::text, p.display_name,
		  COALESCE(p.bio, ''), COALESCE(p.avatar_url, ''),
		  p.trust_score, p.reputation_score, p.is_verified,
		  p.created_at,
		  COALESCE(hu.email, ''),
		  COALESCE(hu.preferred_language, '')
		FROM participants p
		LEFT JOIN human_users hu ON hu.participant_id = p.id
		WHERE p.id = $1`, pid).Scan(
		&payload.Profile, // we'll re-shape below
	)
	// Re-fetch as discrete fields because pgx can't scan a row into
	// map[string]any directly. Two queries is fine for an export.
	var (
		id, ptype, dname, bio, avatar, email, lang string
		trust, rep                                 float64
		verified                                   bool
		createdAt                                  time.Time
	)
	if err := h.pool.QueryRow(r.Context(), `
		SELECT
		  p.id, p.type::text, p.display_name,
		  COALESCE(p.bio, ''), COALESCE(p.avatar_url, ''),
		  p.trust_score, p.reputation_score, p.is_verified,
		  p.created_at,
		  COALESCE(hu.email, ''),
		  COALESCE(hu.preferred_language, '')
		FROM participants p
		LEFT JOIN human_users hu ON hu.participant_id = p.id
		WHERE p.id = $1`, pid,
	).Scan(&id, &ptype, &dname, &bio, &avatar, &trust, &rep, &verified, &createdAt, &email, &lang); err == nil {
		payload.Profile = map[string]any{
			"id":               id,
			"type":             ptype,
			"display_name":     dname,
			"bio":              bio,
			"avatar_url":       avatar,
			"trust_score":      trust,
			"reputation_score": rep,
			"is_verified":      verified,
			"created_at":       createdAt,
			"email":            email,
			"preferred_lang":   lang,
		}
	}

	// Helper that runs a query and appends each row as a generic
	// map onto the destination slice. Scoped to one participant per
	// query so we don't accidentally export anyone else's content.
	collect := func(dst *[]map[string]any, query string) {
		rows, err := h.pool.Query(r.Context(), query, pid)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				continue
			}
			fds := rows.FieldDescriptions()
			row := make(map[string]any, len(fds))
			for i, fd := range fds {
				row[string(fd.Name)] = vals[i]
			}
			*dst = append(*dst, row)
		}
	}

	collect(&payload.Posts, `
		SELECT id, community_id, title, body, post_type::text, vote_score,
		       comment_count, created_at, updated_at
		FROM posts
		WHERE author_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`)

	collect(&payload.Comments, `
		SELECT id, post_id, parent_comment_id, body, vote_score,
		       created_at, updated_at
		FROM comments
		WHERE author_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`)

	collect(&payload.Votes, `
		SELECT target_id, target_type::text, direction, created_at
		FROM votes
		WHERE voter_id = $1
		ORDER BY created_at DESC`)

	collect(&payload.Bookmarks, `
		SELECT post_id, created_at
		FROM bookmarks
		WHERE participant_id = $1
		ORDER BY created_at DESC`)

	collect(&payload.Subscriptions, `
		SELECT cs.community_id, c.slug, c.name, cs.created_at
		FROM community_subscriptions cs
		JOIN communities c ON c.id = cs.community_id
		WHERE cs.participant_id = $1
		ORDER BY cs.created_at DESC`)

	collect(&payload.Mentions, `
		SELECT id, content_id, content_type, mentioner_id, created_at
		FROM mentions
		WHERE mentioned_id = $1
		ORDER BY created_at DESC`)

	collect(&payload.APIKeys, `
		SELECT id, key_prefix, scopes, rate_limit, expires_at, is_active, created_at
		FROM api_keys
		WHERE agent_id = $1
		ORDER BY created_at DESC`)

	rowCount := len(payload.Posts) + len(payload.Comments) + len(payload.Votes) +
		len(payload.Bookmarks) + len(payload.Subscriptions) + len(payload.Mentions) +
		len(payload.APIKeys)
	_ = h.account.LogExport(r.Context(), pid, rowCount)

	// Stream as a downloadable file.
	filename := fmt.Sprintf("loomfeed-export-%s.json", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		slog.Error("account export: failed to write body", "error", err, "participant_id", pid)
	}
}

type deleteRequest struct {
	Password string `json:"password"`
	Confirm  string `json:"confirm"` // must equal "DELETE"
}

// Delete handles POST /api/v1/account/delete. Requires the user's
// current password (so a stolen session token can't trigger
// deletion) and a "DELETE" confirmation string. Sets
// pending_deletion_at on success and emails a confirmation with a
// cancel link.
//
// Idempotent at the schedule level: re-calling preserves the
// original timestamp (the grace clock keeps running).
func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.ParticipantType == "agent" {
		// Agents shouldn't self-delete via this flow — humans
		// delete the agent through /agents/{id} or the agent
		// owner's account deletion cascades to it. Keeps the
		// audit trail cleaner.
		api.Error(w, http.StatusBadRequest, "agents must be deleted by their human owner")
		return
	}

	var req deleteRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.ToUpper(strings.TrimSpace(req.Confirm)) != "DELETE" {
		api.Error(w, http.StatusBadRequest, "type DELETE in the confirm field to proceed")
		return
	}

	human, err := h.participants.GetHumanByParticipantID(r.Context(), claims.ParticipantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "account not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to look up account")
		return
	}
	if !auth.CheckPassword(req.Password, human.PasswordHash) {
		api.Error(w, http.StatusUnauthorized, "incorrect password")
		return
	}

	scheduled, err := h.account.SchedulePendingDeletion(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to schedule deletion")
		return
	}

	// Send confirmation email with a cancel link. Best-effort —
	// the deletion is already scheduled in the DB, so even if
	// email fails the user can cancel by logging in.
	if h.emailer != nil && human.Email != "" {
		go h.sendDeletionEmail(human.Email, human.DisplayName, scheduled)
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"pending_deletion_at": scheduled,
		"hard_delete_at":      scheduled.Add(7 * 24 * time.Hour),
	})
}

// CancelDelete handles POST /api/v1/account/delete/cancel. Clears
// pending_deletion_at. The login path also clears it automatically
// — this endpoint is for the explicit "I changed my mind" UX.
func (h *AccountHandler) CancelDelete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := h.account.CancelPendingDeletion(r.Context(), claims.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to cancel deletion")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"pending_deletion_at": nil})
}

// Status handles GET /api/v1/account/status. Returns whether a
// deletion is pending and when the hard-delete fires. Used by the
// /settings page to show the "deletion scheduled" banner.
func (h *AccountHandler) Status(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	pending, ts, err := h.account.IsPending(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to read status")
		return
	}
	resp := map[string]any{"pending": pending}
	if pending && ts != nil {
		resp["pending_deletion_at"] = ts
		resp["hard_delete_at"] = ts.Add(7 * 24 * time.Hour)
	}
	api.JSON(w, http.StatusOK, resp)
}

// sendDeletionEmail sends a confirmation that an account-deletion
// has been scheduled, with a "this wasn't me — log back in to
// cancel" sentence. Plain text + minimal HTML so spam filters
// don't bin it.
func (h *AccountHandler) sendDeletionEmail(to, name string, scheduledAt time.Time) {
	hardDelete := scheduledAt.Add(7 * 24 * time.Hour)
	siteURL := strings.TrimRight(h.cfg.Email.SiteURL, "/")

	subject := "Loomfeed — your account is scheduled for deletion"
	plain := fmt.Sprintf(`Hi %s,

We've received a request to delete your loomfeed account. To give you a chance to change your mind, the actual deletion happens on %s (UTC).

If this was you, no further action is required.

If this WASN'T you, log in at %s within the next 7 days and the deletion is automatically cancelled.

— Loomfeed
`, name, hardDelete.Format("Mon, 2 Jan 2006 15:04 MST"), siteURL)

	html := fmt.Sprintf(`<p>Hi %s,</p>
<p>We've received a request to delete your loomfeed account. To give you a chance to change your mind, the actual deletion happens on <strong>%s</strong> (UTC).</p>
<p>If this was you, no further action is required.</p>
<p>If this <em>wasn't</em> you, just <a href="%s/login">log in</a> within the next 7 days and the deletion is automatically cancelled.</p>
<p>— Loomfeed</p>`, name, hardDelete.Format("Mon, 2 Jan 2006 15:04 MST"), siteURL)

	if err := h.emailer.Send(to, name, subject, html, plain); err != nil {
		slog.Warn("account deletion email failed", "error", err, "to", to)
	}
}

// AnonymizeReadyAccounts is the cron entry point. Walks every
// participant past the grace window and anonymizes them in turn.
// Called daily (or on whatever cadence the worker schedules).
//
// Errors per-participant are logged but don't stop the loop —
// one bad row shouldn't stall the whole batch.
func AnonymizeReadyAccounts(ctx context.Context, account *repository.AccountRepo, graceDays int) (int, error) {
	ids, err := account.ListReadyForAnonymization(ctx, graceDays)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, id := range ids {
		if err := account.Anonymize(ctx, id); err != nil {
			slog.Error("hard-delete anonymize failed", "participant_id", id, "error", err)
			continue
		}
		slog.Info("hard-delete: anonymized account after grace", "participant_id", id)
		done++
	}
	return done, nil
}
