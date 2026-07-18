package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/sqlite"

	"github.com/go-chi/chi/v5"
)

// FeedbackHandler handles HTTP routes for training plan feedback and notifications.
type FeedbackHandler struct {
	repo     *sqlite.FeedbackRepository
	linkRepo *sqlite.FichaWebRepository
}

// NewFeedbackHandler creates a new FeedbackHandler instance.
func NewFeedbackHandler(db *sqlite.DB) *FeedbackHandler {
	return &FeedbackHandler{
		repo:     sqlite.NewFeedbackRepository(db),
		linkRepo: sqlite.NewFichaWebRepository(db),
	}
}

// SubmitFeedbackRequest represents payload for POST /api/v1/feedback/{hash}.
type SubmitFeedbackRequest struct {
	Rating     *int    `json:"rating"`
	Comentario *string `json:"comentario"`
}

// SubmitFeedbackResponse represents response returned on feedback submission.
type SubmitFeedbackResponse struct {
	Message    string `json:"message"`
	FeedbackID int64  `json:"feedback_id"`
	Rating     int    `json:"rating"`
}

// Submit handles POST /api/v1/feedback/{hash}
func (h *FeedbackHandler) Submit(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		writeJSONError(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	var req SubmitFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.Rating == nil || *req.Rating < 1 || *req.Rating > 5 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "Rating inválido",
			"message": "Rating deve ser um inteiro entre 1 e 5",
		})
		return
	}

	var commentStr string
	if req.Comentario != nil {
		commentStr = strings.TrimSpace(*req.Comentario)
	}

	// 1. Check if public link exists and is active before trying to write feedback
	link, err := h.linkRepo.GetByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "Ficha não encontrada",
			})
			return
		}
		writeJSONError(w, "Failed to retrieve public link", http.StatusInternalServerError)
		return
	}

	if !link.Ativo {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "Ficha desativada",
			"message": "Não é possível avaliar fichas desativadas",
		})
		return
	}

	// 2. Check if feedback was already sent (returns 409 Conflict with previous rating)
	existing, err := h.repo.GetFeedbackByHash(r.Context(), hash)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":           "Feedback já enviado",
			"message":         "Você já avaliou esta ficha",
			"rating_anterior": existing.Rating,
		})
		return
	}

	// 3. Create feedback
	fb := &domain.FeedbackFicha{
		HashFicha:  hash,
		Rating:     *req.Rating,
		Comentario: &commentStr,
	}

	feedbackID, err := h.repo.CreateFeedback(r.Context(), fb)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "Feedback já enviado",
				"message": "Você já avaliou esta ficha",
			})
			return
		}
		writeJSONError(w, "Erro ao salvar feedback: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := SubmitFeedbackResponse{
		Message:    "Feedback salvo com sucesso",
		FeedbackID: feedbackID,
		Rating:     *req.Rating,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// VerifyResponse represents response returned on checking feedback status.
type VerifyResponse struct {
	HasFeedback bool   `json:"has_feedback"`
	Rating      int    `json:"rating,omitempty"`
	Comentario  string `json:"comentario,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// Verify handles GET /api/v1/feedback/{hash}
func (h *FeedbackHandler) Verify(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		writeJSONError(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	fb, err := h.repo.GetFeedbackByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(VerifyResponse{
				HasFeedback: false,
			})
			return
		}
		writeJSONError(w, "Erro ao verificar feedback", http.StatusInternalServerError)
		return
	}

	var commentStr string
	if fb.Comentario != nil {
		commentStr = *fb.Comentario
	}

	resp := VerifyResponse{
		HasFeedback: true,
		Rating:      fb.Rating,
		Comentario:  commentStr,
		CreatedAt:   fb.CreatedAt.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// PendingFeedbacksResponse represents list of pending feedbacks.
type PendingFeedbacksResponse struct {
	Total     int                     `json:"total"`
	Feedbacks []*domain.FeedbackFicha `json:"feedbacks"`
}

// ListPending handles GET /api/v1/feedback/pendentes
func (h *FeedbackHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.URL.Query().Get("user_id")
	var userID *int64
	if userIDStr != "" {
		id, err := strconv.ParseInt(userIDStr, 10, 64)
		if err == nil {
			userID = &id
		}
	}

	list, err := h.repo.ListPendingFeedbacks(r.Context(), userID)
	if err != nil {
		writeJSONError(w, "Erro ao buscar feedbacks: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if list == nil {
		list = []*domain.FeedbackFicha{}
	}

	resp := PendingFeedbacksResponse{
		Total:     len(list),
		Feedbacks: list,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// MarkReadResponse represents response returned on marking notification read.
type MarkReadResponse struct {
	Message string `json:"message"`
}

// MarkRead handles POST /api/v1/feedback/notificacao/{notificacao_id}/marcar-lido
func (h *FeedbackHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	notifIDStr := chi.URLParam(r, "notificacao_id")
	notifID, err := strconv.ParseInt(notifIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}

	err = h.repo.MarkNotificationLida(r.Context(), notifID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "Notificação não encontrada",
			})
			return
		}
		writeJSONError(w, "Erro ao marcar notificação: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := MarkReadResponse{
		Message: "Notificação marcada como lida",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
