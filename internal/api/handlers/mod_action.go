package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// ModActionHandler handles moderator-only actions on a single community:
// remove/approve/restore content, ban/unban users, and the mod log.
//
// Authorization pattern everywhere: resolve the community from either
// {slug} (community-scoped routes) or via the target (post/comment id),
// then require the caller is creator or a row in community_moderators.
type ModActionHandler struct {
	actions     *repository.ModActionRepo
	moderation  *repository.ModerationRepo
	communities *repository.CommunityRepo
	reports     *repository.ReportRepo
	posts       *repository.PostRepo
	account     *repository.AccountRepo
}

func NewModActionHandler(
	actions *repository.ModActionRepo,
	moderation *repository.ModerationRepo,
	communities *repository.CommunityRepo,
	reports *repository.ReportRepo,
) *ModActionHandler {
	return &ModActionHandler{
		actions:     actions,
		moderation:  moderation,
		communities: communities,
		reports:     reports,
	}
}

// WithPostsAndAccount wires the post + account repos so ApprovePost
// can also un-quarantine the post and mark its author graduated
// (Phase 0.4 behavior). Optional — when unset, ApprovePost still
// dismisses reports the way it always has.
func (h *ModActionHandler) WithPostsAndAccount(posts *repository.PostRepo, account *repository.AccountRepo) {
	h.posts = posts
	h.account = account
}

// requireMod checks the caller is creator or a moderator of the given
// community. Writes an error response and returns false if not.
func (h *ModActionHandler) requireMod(w http.ResponseWriter, r *http.Request, communityID string) (string, bool) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return "", false
	}
	// Creator check
	comm, err := h.communities.GetByID(r.Context(), communityID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "community not found")
		return "", false
	}
	if comm.CreatedBy == claims.ParticipantID {
		return claims.ParticipantID, true
	}
	isMod, err := h.moderation.IsModerator(r.Context(), communityID, claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to check moderator status")
		return "", false
	}
	if !isMod {
		api.Error(w, http.StatusForbidden, "moderator permission required")
		return "", false
	}
	return claims.ParticipantID, true
}

// resolveCommunityBySlug returns the community ID for {slug}, writing
// the error response if not found.
func (h *ModActionHandler) resolveCommunityBySlug(w http.ResponseWriter, r *http.Request) (string, bool) {
	slug := r.PathValue("slug")
	if slug == "" {
		api.Error(w, http.StatusBadRequest, "slug is required")
		return "", false
	}
	comm, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return "", false
		}
		api.Error(w, http.StatusInternalServerError, "failed to get community")
		return "", false
	}
	return comm.ID, true
}

// -------- Posts --------

type modReasonRequest struct {
	Reason string `json:"reason"`
}

// RemovePost handles POST /api/v1/posts/{id}/remove.
func (h *ModActionHandler) RemovePost(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}
	cid, err := h.actions.PostCommunityID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}
	actorID, ok := h.requireMod(w, r, cid)
	if !ok {
		return
	}
	var req modReasonRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	reason := strings.TrimSpace(req.Reason)

	if err := h.actions.RemovePost(r.Context(), postID, actorID, reason); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to remove post")
		return
	}
	_ = h.actions.Log(r.Context(), cid, actorID, repository.ModActionRemovePost, "post", postID, reason)

	// Auto-resolve any pending reports against this post. Best-effort.
	_ = h.reports.ResolveAllForContent(r.Context(), postID, "post", actorID, "resolved")

	api.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// RestorePost handles POST /api/v1/posts/{id}/restore.
func (h *ModActionHandler) RestorePost(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}
	cid, err := h.actions.PostCommunityID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}
	actorID, ok := h.requireMod(w, r, cid)
	if !ok {
		return
	}
	if err := h.actions.RestorePost(r.Context(), postID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to restore post")
		return
	}
	_ = h.actions.Log(r.Context(), cid, actorID, repository.ModActionRestorePost, "post", postID, "")
	api.JSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

// ApprovePost handles POST /api/v1/posts/{id}/approve.
//
// Two side effects fire here:
//
//  1. Dismisses any pending reports against the post (the
//     historical behavior — mods clicking "approve" on a flagged
//     post say "this is fine, drop the flags").
//
//  2. (Phase 0.4) If the post is currently quarantined (new-account
//     pre-publish queue), flips quarantined → false so it lands on
//     public feeds AND graduates the author so future posts skip
//     the quarantine check.
//
// Idempotent — re-approving an already-approved post is a no-op.
func (h *ModActionHandler) ApprovePost(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}
	cid, err := h.actions.PostCommunityID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}
	actorID, ok := h.requireMod(w, r, cid)
	if !ok {
		return
	}

	// Phase 0.4 — un-quarantine + graduate the author. A schema-backed
	// human-verification gate cannot be bypassed with moderator approval;
	// the verifier endpoint must record the seal first.
	if h.posts != nil {
		if post, err := h.posts.GetByID(r.Context(), postID); err == nil && post != nil && post.Quarantined {
			if err := h.posts.SetQuarantined(r.Context(), postID, false); err != nil {
				if errors.Is(err, repository.ErrHumanVerificationRequired) {
					api.Error(w, http.StatusConflict, "this agent post requires a human verification before publication")
					return
				}
				api.Error(w, http.StatusInternalServerError, "failed to approve quarantined post")
				return
			}
			if h.account != nil && post.AuthorID != "" {
				_ = h.account.MarkGraduated(r.Context(), post.AuthorID)
			}
		}
	}

	_ = h.reports.ResolveAllForContent(r.Context(), postID, "post", actorID, "dismissed")
	_ = h.actions.Log(r.Context(), cid, actorID, repository.ModActionApprovePost, "post", postID, "")

	api.JSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// -------- Comments --------

// RemoveComment handles POST /api/v1/comments/{id}/remove.
func (h *ModActionHandler) RemoveComment(w http.ResponseWriter, r *http.Request) {
	commentID := r.PathValue("id")
	if commentID == "" {
		api.Error(w, http.StatusBadRequest, "comment id is required")
		return
	}
	cid, err := h.actions.CommentCommunityID(r.Context(), commentID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "comment not found")
		return
	}
	actorID, ok := h.requireMod(w, r, cid)
	if !ok {
		return
	}
	var req modReasonRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	reason := strings.TrimSpace(req.Reason)

	if err := h.actions.RemoveComment(r.Context(), commentID, actorID, reason); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to remove comment")
		return
	}
	_ = h.actions.Log(r.Context(), cid, actorID, repository.ModActionRemoveComment, "comment", commentID, reason)
	_ = h.reports.ResolveAllForContent(r.Context(), commentID, "comment", actorID, "resolved")
	api.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// RestoreComment handles POST /api/v1/comments/{id}/restore.
func (h *ModActionHandler) RestoreComment(w http.ResponseWriter, r *http.Request) {
	commentID := r.PathValue("id")
	if commentID == "" {
		api.Error(w, http.StatusBadRequest, "comment id is required")
		return
	}
	cid, err := h.actions.CommentCommunityID(r.Context(), commentID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "comment not found")
		return
	}
	actorID, ok := h.requireMod(w, r, cid)
	if !ok {
		return
	}
	if err := h.actions.RestoreComment(r.Context(), commentID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to restore comment")
		return
	}
	_ = h.actions.Log(r.Context(), cid, actorID, repository.ModActionRestoreComment, "comment", commentID, "")
	api.JSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

// -------- Bans --------

type banRequest struct {
	ParticipantID string `json:"participant_id"`
	Reason        string `json:"reason"`
	ExpiresAt     string `json:"expires_at"` // RFC3339, optional
}

// BanUser handles POST /api/v1/communities/{slug}/bans.
func (h *ModActionHandler) BanUser(w http.ResponseWriter, r *http.Request) {
	cid, ok := h.resolveCommunityBySlug(w, r)
	if !ok {
		return
	}
	actorID, ok := h.requireMod(w, r, cid)
	if !ok {
		return
	}
	var req banRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ParticipantID = strings.TrimSpace(req.ParticipantID)
	if req.ParticipantID == "" {
		api.Error(w, http.StatusBadRequest, "participant_id is required")
		return
	}
	if req.ParticipantID == actorID {
		api.Error(w, http.StatusBadRequest, "cannot ban yourself")
		return
	}
	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			api.Error(w, http.StatusBadRequest, "expires_at must be RFC3339")
			return
		}
		expiresAt = &t
	}
	reason := strings.TrimSpace(req.Reason)
	if err := h.actions.Ban(r.Context(), cid, req.ParticipantID, actorID, reason, expiresAt); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to ban user")
		return
	}
	_ = h.actions.Log(r.Context(), cid, actorID, repository.ModActionBan, "participant", req.ParticipantID, reason)
	api.JSON(w, http.StatusCreated, map[string]string{"status": "banned"})
}

// UnbanUser handles DELETE /api/v1/communities/{slug}/bans/{id}.
func (h *ModActionHandler) UnbanUser(w http.ResponseWriter, r *http.Request) {
	cid, ok := h.resolveCommunityBySlug(w, r)
	if !ok {
		return
	}
	actorID, ok := h.requireMod(w, r, cid)
	if !ok {
		return
	}
	participantID := r.PathValue("id")
	if participantID == "" {
		api.Error(w, http.StatusBadRequest, "participant id is required")
		return
	}
	if err := h.actions.Unban(r.Context(), cid, participantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to unban")
		return
	}
	_ = h.actions.Log(r.Context(), cid, actorID, repository.ModActionUnban, "participant", participantID, "")
	api.JSON(w, http.StatusOK, map[string]string{"status": "unbanned"})
}

// ListBans handles GET /api/v1/communities/{slug}/bans.
func (h *ModActionHandler) ListBans(w http.ResponseWriter, r *http.Request) {
	cid, ok := h.resolveCommunityBySlug(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireMod(w, r, cid); !ok {
		return
	}
	bans, err := h.actions.ListBans(r.Context(), cid)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list bans")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"bans": bans})
}

// -------- Reports --------

// ResolveReport handles PUT /api/v1/reports/{id}/resolve.
//
// Authorization: the caller must be creator-or-moderator of the community
// that owns the reported content. Without this check any authenticated user
// could dismiss/resolve any report platform-wide (the reports table has no
// RLS backstop).
func (h *ModActionHandler) ResolveReport(w http.ResponseWriter, r *http.Request) {
	reportID := r.PathValue("id")
	if reportID == "" {
		api.Error(w, http.StatusBadRequest, "report id is required")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Status != "resolved" && req.Status != "dismissed" {
		api.Error(w, http.StatusBadRequest, "status must be 'resolved' or 'dismissed'")
		return
	}

	report, err := h.reports.GetByID(r.Context(), reportID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "report not found")
		return
	}

	// Resolve the reported content to its community.
	var cid string
	switch report.ContentType {
	case "post":
		cid, err = h.actions.PostCommunityID(r.Context(), report.ContentID)
	case "comment":
		cid, err = h.actions.CommentCommunityID(r.Context(), report.ContentID)
	default:
		api.Error(w, http.StatusBadRequest, "unsupported report content type")
		return
	}
	if err != nil {
		api.Error(w, http.StatusNotFound, "reported content not found")
		return
	}

	actorID, ok := h.requireMod(w, r, cid)
	if !ok {
		return
	}

	if err := h.reports.Resolve(r.Context(), reportID, actorID, req.Status); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to resolve report")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// -------- Log --------

// Log handles GET /api/v1/communities/{slug}/mod-log.
func (h *ModActionHandler) Log(w http.ResponseWriter, r *http.Request) {
	cid, ok := h.resolveCommunityBySlug(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireMod(w, r, cid); !ok {
		return
	}
	actions, err := h.actions.List(r.Context(), cid, 100)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list mod actions")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"actions": actions})
}
