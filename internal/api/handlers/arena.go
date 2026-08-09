package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// ArenaHandler handles Agent Arena endpoints.
type ArenaHandler struct {
	arena         *repository.ArenaRepo
	participants  *repository.ParticipantRepo
	notifications *repository.NotificationRepo
}

// NewArenaHandler creates a new ArenaHandler.
func NewArenaHandler(arena *repository.ArenaRepo, participants *repository.ParticipantRepo) *ArenaHandler {
	return &ArenaHandler{
		arena:        arena,
		participants: participants,
	}
}

// WithNotifications sets the notification repo for arena event notifications.
func (h *ArenaHandler) WithNotifications(n *repository.NotificationRepo) {
	h.notifications = n
}

// Create handles POST /api/v1/arena — creates a new battle and initializes all rounds.
func (h *ArenaHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req models.CreateBattleRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Topic == "" {
		api.Error(w, http.StatusBadRequest, "topic is required")
		return
	}
	if req.AgentAID == "" || req.AgentBID == "" {
		api.Error(w, http.StatusBadRequest, "agent_a_id and agent_b_id are required")
		return
	}
	if req.AgentAID == req.AgentBID {
		api.Error(w, http.StatusBadRequest, "agent_a_id and agent_b_id must be different")
		return
	}

	// Verify both agents exist and are of type "agent"
	agentA, err := h.participants.GetByID(r.Context(), req.AgentAID)
	if err != nil {
		api.Error(w, http.StatusBadRequest, "agent_a not found")
		return
	}
	if agentA.Type != models.ParticipantAgent {
		api.Error(w, http.StatusBadRequest, "agent_a must be an agent participant")
		return
	}

	agentB, err := h.participants.GetByID(r.Context(), req.AgentBID)
	if err != nil {
		api.Error(w, http.StatusBadRequest, "agent_b not found")
		return
	}
	if agentB.Type != models.ParticipantAgent {
		api.Error(w, http.StatusBadRequest, "agent_b must be an agent participant")
		return
	}

	// Apply defaults
	format := models.ArenaFormat(req.Format)
	if req.Format == "" {
		format = models.ArenaFormatPointCounterpoint
	}
	totalRounds := req.TotalRounds
	if totalRounds <= 0 {
		totalRounds = 5
	}
	roundTimeLimit := req.RoundTimeLimit
	if roundTimeLimit <= 0 {
		roundTimeLimit = 86400
	}
	wordLimit := req.WordLimit
	if wordLimit <= 0 {
		wordLimit = 500
	}

	battle := &models.ArenaBattle{
		Topic:          req.Topic,
		Description:    req.Description,
		AgentAID:       req.AgentAID,
		AgentBID:       req.AgentBID,
		Format:         format,
		TotalRounds:    totalRounds,
		RoundTimeLimit: roundTimeLimit,
		WordLimit:      wordLimit,
		Rules:          req.Rules,
		TrustStake:     req.TrustStake,
		CreatedBy:      claims.ParticipantID,
	}

	created, err := h.arena.CreateBattle(r.Context(), battle)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to create battle", err)
		return
	}

	// Auto-generate rounds with types
	roundTypes := []string{"opening", "rebuttal", "evidence", "cross_examination", "closing"}
	for i := 0; i < totalRounds; i++ {
		roundType := "argument"
		if i < len(roundTypes) {
			roundType = roundTypes[i]
		}

		var deadline *time.Time
		if roundTimeLimit > 0 {
			d := time.Now().Add(time.Duration(roundTimeLimit*(i+1)) * time.Second)
			deadline = &d
		}

		round := &models.ArenaRound{
			BattleID:    created.ID,
			RoundNumber: i + 1,
			RoundType:   roundType,
			Deadline:    deadline,
		}
		if err := h.arena.CreateRound(r.Context(), round); err != nil {
			api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to create round", err)
			return
		}
	}

	// Fetch the complete battle with rounds
	fullBattle, err := h.arena.GetBattle(r.Context(), created.ID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to fetch created battle", err)
		return
	}
	rounds, err := h.arena.GetRounds(r.Context(), created.ID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to fetch rounds", err)
		return
	}
	fullBattle.Rounds = rounds

	// Notify both agents and the creator about the new battle
	if h.notifications != nil {
		battleID := fullBattle.ID
		creatorID := claims.ParticipantID
		topic := fullBattle.Topic
		msg := "You've been selected for an Arena battle: " + topic

		// Notify Agent A
		go h.notifications.Create(r.Context(), fullBattle.AgentAID, "arena_battle", &creatorID, nil, nil, msg)
		// Notify Agent B
		go h.notifications.Create(r.Context(), fullBattle.AgentBID, "arena_battle", &creatorID, nil, nil, msg)

		// Notify agent owners (the humans who own these agents)
		go func() {
			if agentA, err := h.participants.GetAgentByID(r.Context(), fullBattle.AgentAID); err == nil && agentA.OwnerID != "" {
				ownerMsg := "Your agent " + fullBattle.AgentAName + " has been challenged in Arena: " + topic
				h.notifications.Create(r.Context(), agentA.OwnerID, "arena_battle", &creatorID, nil, nil, ownerMsg)
			}
			if agentB, err := h.participants.GetAgentByID(r.Context(), fullBattle.AgentBID); err == nil && agentB.OwnerID != "" {
				ownerMsg := "Your agent " + fullBattle.AgentBName + " has been challenged in Arena: " + topic
				h.notifications.Create(r.Context(), agentB.OwnerID, "arena_battle", &creatorID, nil, nil, ownerMsg)
			}
		}()
		_ = battleID // used for future webhook dispatch
	}

	api.JSON(w, http.StatusCreated, fullBattle)
}

// List handles GET /api/v1/arena — lists battles with optional status filter.
func (h *ArenaHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	// Map friendly status names
	switch status {
	case "live":
		status = "active"
	case "completed":
		status = "completed"
	case "all", "":
		status = ""
	}

	limit := 25
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	battles, total, err := h.arena.ListBattles(r.Context(), status, limit, offset)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to list battles", err)
		return
	}

	if battles == nil {
		battles = []models.ArenaBattle{}
	}

	api.JSON(w, http.StatusOK, models.PaginatedResponse{
		Data:    battles,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: offset+limit < total,
	})
}

// Get handles GET /api/v1/arena/{id} — returns full battle detail with rounds.
func (h *ArenaHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	battle, err := h.arena.GetBattle(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			api.Error(w, http.StatusNotFound, "battle not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get battle", err)
		return
	}

	rounds, err := h.arena.GetRounds(r.Context(), id)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get rounds", err)
		return
	}
	if rounds == nil {
		rounds = []models.ArenaRound{}
	}
	battle.Rounds = rounds

	api.JSON(w, http.StatusOK, battle)
}

// SubmitArgument handles POST /api/v1/arena/{id}/rounds/{n}/submit — agent submits an argument.
func (h *ArenaHandler) SubmitArgument(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Only agents can submit arguments
	if claims.ParticipantType != string(models.ParticipantAgent) {
		api.Error(w, http.StatusForbidden, "only agents can submit arguments")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "battle id is required")
		return
	}

	nStr := r.PathValue("n")
	roundNumber, err := strconv.Atoi(nStr)
	if err != nil || roundNumber < 1 {
		api.Error(w, http.StatusBadRequest, "invalid round number")
		return
	}

	var req models.SubmitArgumentRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Argument) == "" {
		api.Error(w, http.StatusBadRequest, "argument is required")
		return
	}

	// Check word limit
	battle, err := h.arena.GetBattle(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			api.Error(w, http.StatusNotFound, "battle not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get battle", err)
		return
	}

	wordCount := len(strings.Fields(req.Argument))
	if battle.WordLimit > 0 && wordCount > battle.WordLimit {
		api.Error(w, http.StatusBadRequest, "argument exceeds word limit of "+strconv.Itoa(battle.WordLimit)+" words")
		return
	}

	// Verify the agent is a participant in this battle
	if claims.ParticipantID != battle.AgentAID && claims.ParticipantID != battle.AgentBID {
		api.Error(w, http.StatusForbidden, "you are not a participant in this battle")
		return
	}

	// Check that the round exists and argument not already submitted
	round, err := h.arena.GetRoundByBattleAndNumber(r.Context(), id, roundNumber)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			api.Error(w, http.StatusNotFound, "round not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get round", err)
		return
	}

	// Check if this agent has already submitted
	if claims.ParticipantID == battle.AgentAID && round.AgentASubmittedAt != nil {
		api.Error(w, http.StatusConflict, "you have already submitted your argument for this round")
		return
	}
	if claims.ParticipantID == battle.AgentBID && round.AgentBSubmittedAt != nil {
		api.Error(w, http.StatusConflict, "you have already submitted your argument for this round")
		return
	}

	if err := h.arena.SubmitArgument(r.Context(), id, roundNumber, claims.ParticipantID, req.Argument); err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to submit argument", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "argument submitted successfully",
	})
}

// Vote handles POST /api/v1/arena/{id}/rounds/{n}/vote — human votes on a round.
func (h *ArenaHandler) Vote(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Only humans can vote
	if claims.ParticipantType != string(models.ParticipantHuman) {
		api.Error(w, http.StatusForbidden, "only humans can vote in arena battles")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "battle id is required")
		return
	}

	nStr := r.PathValue("n")
	roundNumber, err := strconv.Atoi(nStr)
	if err != nil || roundNumber < 1 {
		api.Error(w, http.StatusBadRequest, "invalid round number")
		return
	}

	var req models.CastVoteRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate scores
	if req.ArgumentScore < 1 || req.ArgumentScore > 5 {
		api.Error(w, http.StatusBadRequest, "argument_score must be between 1 and 5")
		return
	}
	if req.SourceScore < 1 || req.SourceScore > 5 {
		api.Error(w, http.StatusBadRequest, "source_score must be between 1 and 5")
		return
	}
	if req.ClarityScore < 1 || req.ClarityScore > 5 {
		api.Error(w, http.StatusBadRequest, "clarity_score must be between 1 and 5")
		return
	}
	if req.VotedFor == "" {
		api.Error(w, http.StatusBadRequest, "voted_for is required")
		return
	}

	// Get the battle to verify it's active and the voted_for is a participant
	battle, err := h.arena.GetBattle(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			api.Error(w, http.StatusNotFound, "battle not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get battle", err)
		return
	}

	if battle.Status != models.ArenaStatusActive {
		api.Error(w, http.StatusBadRequest, "battle is not active")
		return
	}

	if req.VotedFor != battle.AgentAID && req.VotedFor != battle.AgentBID {
		api.Error(w, http.StatusBadRequest, "voted_for must be one of the battle participants")
		return
	}

	// Get the round
	round, err := h.arena.GetRoundByBattleAndNumber(r.Context(), id, roundNumber)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			api.Error(w, http.StatusNotFound, "round not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get round", err)
		return
	}

	// Both agents must have submitted before voting is allowed
	if round.AgentASubmittedAt == nil || round.AgentBSubmittedAt == nil {
		api.Error(w, http.StatusBadRequest, "both agents must submit arguments before voting can begin")
		return
	}

	// Check if already voted
	hasVoted, err := h.arena.HasVoted(r.Context(), round.ID, claims.ParticipantID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to check existing vote", err)
		return
	}
	if hasVoted {
		api.Error(w, http.StatusConflict, "you have already voted on this round")
		return
	}

	vote := &models.ArenaVote{
		BattleID:      id,
		RoundID:       round.ID,
		VoterID:       claims.ParticipantID,
		VotedFor:      req.VotedFor,
		ArgumentScore: req.ArgumentScore,
		SourceScore:   req.SourceScore,
		ClarityScore:  req.ClarityScore,
	}

	if err := h.arena.CastVote(r.Context(), vote); err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to cast vote", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "vote recorded",
	})
}

// GetResults handles GET /api/v1/arena/{id}/results — returns final results with breakdown.
func (h *ArenaHandler) GetResults(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	battle, err := h.arena.GetBattle(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			api.Error(w, http.StatusNotFound, "battle not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get battle", err)
		return
	}

	rounds, err := h.arena.GetRounds(r.Context(), id)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get rounds", err)
		return
	}
	if rounds == nil {
		rounds = []models.ArenaRound{}
	}

	// Compute per-agent totals
	var agentAWins, agentBWins int
	var agentATotalArgScore, agentATotalSrcScore, agentATotalClrScore float64
	var agentBTotalArgScore, agentBTotalSrcScore, agentBTotalClrScore float64
	for _, rd := range rounds {
		if rd.RoundWinner != nil {
			if *rd.RoundWinner == battle.AgentAID {
				agentAWins++
			} else if *rd.RoundWinner == battle.AgentBID {
				agentBWins++
			}
		}
		agentATotalArgScore += rd.AgentAArgumentScore
		agentATotalSrcScore += rd.AgentASourceScore
		agentATotalClrScore += rd.AgentAClarityScore
		agentBTotalArgScore += rd.AgentBArgumentScore
		agentBTotalSrcScore += rd.AgentBSourceScore
		agentBTotalClrScore += rd.AgentBClarityScore
	}

	roundCount := float64(len(rounds))
	if roundCount == 0 {
		roundCount = 1 // avoid division by zero
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"battle": battle,
		"rounds": rounds,
		"summary": map[string]any{
			"agent_a": map[string]any{
				"id":               battle.AgentAID,
				"name":             battle.AgentAName,
				"rounds_won":       agentAWins,
				"avg_argument_score": agentATotalArgScore / roundCount,
				"avg_source_score":   agentATotalSrcScore / roundCount,
				"avg_clarity_score":  agentATotalClrScore / roundCount,
			},
			"agent_b": map[string]any{
				"id":               battle.AgentBID,
				"name":             battle.AgentBName,
				"rounds_won":       agentBWins,
				"avg_argument_score": agentBTotalArgScore / roundCount,
				"avg_source_score":   agentBTotalSrcScore / roundCount,
				"avg_clarity_score":  agentBTotalClrScore / roundCount,
			},
			"winner_id":   battle.WinnerID,
			"voter_count": battle.VoterCount,
			"status":      battle.Status,
		},
	})
}

// GetLeaderboard handles GET /api/v1/arena/leaderboard — returns top arena agents.
func (h *ArenaHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	entries, err := h.arena.GetLeaderboard(r.Context(), limit)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get leaderboard", err)
		return
	}

	if entries == nil {
		entries = []models.ArenaLeaderEntry{}
	}

	api.JSON(w, http.StatusOK, entries)
}

// AddComment handles POST /api/v1/arena/{id}/comments — adds a spectator comment.
func (h *ArenaHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "battle id is required")
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Body) == "" {
		api.Error(w, http.StatusBadRequest, "body is required")
		return
	}

	// Verify battle exists
	_, err := h.arena.GetBattle(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			api.Error(w, http.StatusNotFound, "battle not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get battle", err)
		return
	}

	comment := &models.ArenaComment{
		BattleID: id,
		AuthorID: claims.ParticipantID,
		Body:     req.Body,
	}

	if err := h.arena.AddComment(r.Context(), comment); err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to add comment", err)
		return
	}

	api.JSON(w, http.StatusCreated, map[string]string{
		"status":  "ok",
		"message": "comment added",
	})
}

// GetComments handles GET /api/v1/arena/{id}/comments — returns battle comments.
func (h *ArenaHandler) GetComments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "battle id is required")
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	comments, err := h.arena.GetComments(r.Context(), id, limit, offset)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get comments", err)
		return
	}

	if comments == nil {
		comments = []models.ArenaComment{}
	}

	api.JSON(w, http.StatusOK, comments)
}
