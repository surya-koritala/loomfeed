package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

type QuizHandler struct {
	quizzes *repository.QuizRepo
	posts   *repository.PostRepo
}

func NewQuizHandler(quizzes *repository.QuizRepo, posts *repository.PostRepo) *QuizHandler {
	return &QuizHandler{quizzes: quizzes, posts: posts}
}

type quizQuestion struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Options []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"options"`
	Correct     string `json:"correct"`
	Explanation string `json:"explanation"`
}

type quizMetadata struct {
	Questions []quizQuestion `json:"questions"`
}

type submitQuizRequest struct {
	Answers map[string]string `json:"answers"`
}

type questionResult struct {
	QuestionID    string `json:"question_id"`
	Correct       bool   `json:"correct"`
	CorrectAnswer string `json:"correct_answer"`
	Explanation   string `json:"explanation"`
}

func (h *QuizHandler) Submit(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	post, err := h.posts.GetByID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}

	metaBytes, err := json.Marshal(post.Metadata["quiz"])
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "invalid quiz metadata")
		return
	}
	var quiz quizMetadata
	if err := json.Unmarshal(metaBytes, &quiz); err != nil || len(quiz.Questions) == 0 {
		api.Error(w, http.StatusBadRequest, "this post does not have a valid quiz")
		return
	}

	var req submitQuizRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	score := 0
	total := len(quiz.Questions)
	var results []questionResult
	for _, q := range quiz.Questions {
		userAnswer := req.Answers[q.ID]
		correct := userAnswer == q.Correct
		if correct {
			score++
		}
		results = append(results, questionResult{
			QuestionID:    q.ID,
			Correct:       correct,
			CorrectAnswer: q.Correct,
			Explanation:   q.Explanation,
		})
	}

	attempt, err := h.quizzes.SubmitAttempt(r.Context(), postID, claims.ParticipantID, req.Answers, score, total)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to save quiz attempt")
		return
	}

	stats, _ := h.quizzes.GetStats(r.Context(), postID)
	percentile := 0
	if stats != nil && stats.TotalAttempts > 1 {
		belowCount := 0
		for s, count := range stats.ScoreDistribution {
			if s < score {
				belowCount += count
			}
		}
		percentile = belowCount * 100 / stats.TotalAttempts
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"attempt_id": attempt.ID,
		"score":      score,
		"total":      total,
		"percentile": percentile,
		"results":    results,
	})
}

func (h *QuizHandler) Stats(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	stats, err := h.quizzes.GetStats(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get quiz stats")
		return
	}

	api.JSON(w, http.StatusOK, stats)
}

func (h *QuizHandler) MyAttempt(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	attempt, err := h.quizzes.GetUserAttempt(r.Context(), postID, claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get attempt")
		return
	}
	if attempt == nil {
		api.JSON(w, http.StatusOK, map[string]any{"attempted": false})
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{"attempted": true, "attempt": attempt})
}
