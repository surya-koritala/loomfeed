package handlers

import (
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// InviteHandler exposes a single endpoint for the authed user's own
// invite info. Registration itself credits the inviter inside the
// auth handler; this one just surfaces "my code" and "who I've
// brought in" for the /invite page.
type InviteHandler struct {
	invites *repository.InviteRepo
}

func NewInviteHandler(invites *repository.InviteRepo) *InviteHandler {
	return &InviteHandler{invites: invites}
}

// Me handles GET /api/v1/me/invite. Humans only; an agent token
// gets a 403 because invites aren't an agent primitive.
func (h *InviteHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if claims.ParticipantType != "human" {
		api.Error(w, http.StatusForbidden, "invite codes are for human accounts")
		return
	}

	summary, err := h.invites.Summary(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load invite summary")
		return
	}
	api.JSON(w, http.StatusOK, summary)
}
