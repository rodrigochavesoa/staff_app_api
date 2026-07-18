package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"staff_app/internal/domain"
	"staff_app/internal/sqlite"
)

type PlanoHandler struct {
	repo *sqlite.PlanoRepository
}

func NewPlanoHandler(db *sqlite.DB) *PlanoHandler {
	return &PlanoHandler{repo: sqlite.NewPlanoRepository(db)}
}

type planoRequest struct {
	Nome         string  `json:"nome"`
	PrecoDefault float64 `json:"preco_default"`
	Descricao    string  `json:"descricao"`
	Ativo        *bool   `json:"ativo"`
}

func (h *PlanoHandler) List(w http.ResponseWriter, r *http.Request) {
	planos, err := h.repo.List(r.Context(), false)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if planos == nil {
		planos = []*domain.Plano{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total":  len(planos),
		"planos": planos,
	})
}

func (h *PlanoHandler) Create(w http.ResponseWriter, r *http.Request) {
	plano, ok := decodePlanoRequest(w, r)
	if !ok {
		return
	}
	if err := h.repo.Create(r.Context(), plano); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeJSONError(w, "Plan name is already registered", http.StatusConflict)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(plano)
}

func (h *PlanoHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeJSONError(w, "Invalid plan ID", http.StatusBadRequest)
		return
	}
	plano, ok := decodePlanoRequest(w, r)
	if !ok {
		return
	}
	plano.ID = id
	if err := h.repo.Update(r.Context(), plano); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Plan not found", http.StatusNotFound)
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeJSONError(w, "Plan name is already registered", http.StatusConflict)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(plano)
}

func (h *PlanoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeJSONError(w, "Invalid plan ID", http.StatusBadRequest)
		return
	}
	if err := h.repo.Deactivate(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Plan not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodePlanoRequest(w http.ResponseWriter, r *http.Request) (*domain.Plano, bool) {
	var req planoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return nil, false
	}
	req.Nome = strings.TrimSpace(req.Nome)
	req.Descricao = strings.TrimSpace(req.Descricao)
	if req.Nome == "" {
		writeJSONError(w, "nome is required", http.StatusBadRequest)
		return nil, false
	}
	if req.PrecoDefault < 0 {
		writeJSONError(w, "preco_default must be zero or positive", http.StatusBadRequest)
		return nil, false
	}
	ativo := true
	if req.Ativo != nil {
		ativo = *req.Ativo
	}
	return &domain.Plano{
		Nome:         req.Nome,
		PrecoDefault: req.PrecoDefault,
		Descricao:    req.Descricao,
		Ativo:        ativo,
	}, true
}
