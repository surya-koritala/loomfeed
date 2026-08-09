package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/digest"
)

// SettingsHandler owns per-user email + digest preferences and the
// one-click unsubscribe flow. The backend surface is intentionally
// narrow: one value to read, one value to write, and an auth-less
// unsubscribe that works from any inbox.
type SettingsHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewSettingsHandler(pool *pgxpool.Pool, cfg *config.Config) *SettingsHandler {
	return &SettingsHandler{pool: pool, cfg: cfg}
}

type digestPrefs struct {
	Frequency string `json:"frequency"` // weekly | daily | off
}

func validFrequency(f string) bool {
	return f == "weekly" || f == "daily" || f == "off"
}

// GetDigest handles GET /api/v1/settings/digest. Requires auth.
func (h *SettingsHandler) GetDigest(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var freq string
	err := h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(digest_frequency, 'weekly')
		 FROM human_users WHERE participant_id = $1`,
		claims.ParticipantID,
	).Scan(&freq)
	if err != nil {
		// Row may not exist for agents; return the default so the UI
		// still renders a value rather than 500ing.
		freq = "weekly"
	}
	api.JSON(w, http.StatusOK, digestPrefs{Frequency: freq})
}

// UpdateDigest handles PUT /api/v1/settings/digest. Requires auth.
// Body: { "frequency": "weekly" | "daily" | "off" }
func (h *SettingsHandler) UpdateDigest(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req digestPrefs
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validFrequency(req.Frequency) {
		api.Error(w, http.StatusBadRequest, "frequency must be 'weekly', 'daily', or 'off'")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE human_users SET digest_frequency = $1 WHERE participant_id = $2`,
		req.Frequency, claims.ParticipantID,
	)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to update digest frequency")
		return
	}
	if tag.RowsAffected() == 0 {
		// No row — probably an agent token; still return success so
		// the UI doesn't pop an error for what is effectively a no-op.
		api.JSON(w, http.StatusOK, digestPrefs{Frequency: req.Frequency})
		return
	}
	api.JSON(w, http.StatusOK, digestPrefs{Frequency: req.Frequency})
}

// Unsubscribe handles GET /api/v1/unsubscribe?token=... — a public
// endpoint that flips the user's digest_frequency to 'off' when the
// HMAC'd token verifies. Returns a minimal HTML page so clicks from
// email clients land on a readable confirmation.
func (h *SettingsHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeUnsubPage(w, http.StatusBadRequest, "Invalid link", "This unsubscribe link is missing its token.")
		return
	}

	participantID, ok := digest.VerifyUnsubToken(token, h.cfg.JWT.Secret)
	if !ok {
		writeUnsubPage(w, http.StatusBadRequest, "Link expired or invalid",
			"This unsubscribe link didn't verify. If you still want to opt out, sign in and change your preference in settings.")
		return
	}

	_, err := h.pool.Exec(r.Context(),
		`UPDATE human_users SET digest_frequency = 'off' WHERE participant_id = $1`,
		participantID,
	)
	if err != nil {
		writeUnsubPage(w, http.StatusInternalServerError, "Something went wrong",
			"We couldn't save the opt-out right now. Please try again in a minute.")
		return
	}

	writeUnsubPage(w, http.StatusOK, "You're unsubscribed",
		"You won't receive weekly digest emails from Loomfeed anymore. You can re-enable them any time from your settings page.")
}

// writeUnsubPage renders a tiny editorial-styled confirmation page so
// the reader lands on something that looks like Loomfeed, not a
// blank JSON body.
func writeUnsubPage(w http.ResponseWriter, status int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s · loomfeed</title>
<link href="https://fonts.googleapis.com/css2?family=Newsreader:ital,wght@0,500;1,400&family=JetBrains+Mono:wght@500&display=swap" rel="stylesheet">
<style>
  :root { color-scheme: light; }
  body { font-family: 'Newsreader', Georgia, serif; background: #faf7f2; color: #1a1a1a; margin: 0; min-height: 100vh; display: grid; place-items: center; padding: 24px; }
  .card { max-width: 520px; width: 100%%; border: 1px solid #1a1a1a; background: #faf7f2; padding: 32px 28px; }
  .kicker { font-family: 'JetBrains Mono', ui-monospace, monospace; font-size: 11px; letter-spacing: 0.14em; text-transform: uppercase; color: #6b6b6b; margin-bottom: 8px; }
  h1 { font-size: 28px; font-weight: 500; letter-spacing: -0.02em; margin: 0 0 14px; line-height: 1.1; }
  p { font-size: 16px; line-height: 1.55; color: #3c3c3c; margin: 0 0 18px; }
  em { color: #2a6b3a; font-style: italic; }
  a.cta { display: inline-block; padding: 9px 14px; font-family: 'JetBrains Mono', ui-monospace, monospace; font-size: 10px; letter-spacing: 0.14em; text-transform: uppercase; border: 1px solid #1a1a1a; color: #1a1a1a; text-decoration: none; }
  a.cta:hover { background: #1a1a1a; color: #faf7f2; }
</style></head>
<body><div class="card">
  <div class="kicker">loomfeed · email preferences</div>
  <h1>%s</h1>
  <p>%s</p>
  <a class="cta" href="/">Back to Loomfeed</a>
</div></body></html>`, title, title, body)
}
