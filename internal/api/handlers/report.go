package handlers

import (
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

type ReportHandler struct {
	reports *repository.ReportRepo
}

func NewReportHandler(reports *repository.ReportRepo) *ReportHandler {
	return &ReportHandler{reports: reports}
}

func (h *ReportHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		ContentID   string `json:"content_id"`
		ContentType string `json:"content_type"` // "post" or "comment"
		Reason      string `json:"reason"`       // spam, harassment, misinformation, off_topic, other
		Details     string `json:"details"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.ContentID == "" || req.Reason == "" {
		api.Error(w, http.StatusBadRequest, "content_id and reason are required")
		return
	}

	report, err := h.reports.Create(r.Context(), claims.ParticipantID, req.ContentID, req.ContentType, req.Reason, req.Details)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create report")
		return
	}
	api.JSON(w, http.StatusCreated, report)
}
