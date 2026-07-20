package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"staff_app/internal/repositories"
)

type RelatoriosHandler struct {
	repo repositories.RelatoriosRepository
}

func NewRelatoriosHandler(repo repositories.RelatoriosRepository) *RelatoriosHandler {
	return &RelatoriosHandler{repo: repo}
}

func (h *RelatoriosHandler) GetDashboardResumo(w http.ResponseWriter, r *http.Request) {
	resumo, err := h.repo.GetDashboardResumo(r.Context())
	if err != nil {
		writeJSONError(w, "Failed to retrieve dashboard resumo: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resumo)
}

func (h *RelatoriosHandler) GetPatologias(w http.ResponseWriter, r *http.Request) {
	patologias, err := h.repo.GetPatologiasCobertura(r.Context())
	if err != nil {
		writeJSONError(w, "Failed to retrieve patologia coverage report: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(patologias)
}

func (h *RelatoriosHandler) GetSubutilizados(w http.ResponseWriter, r *http.Request) {
	minRecomStr := r.URL.Query().Get("min_recomendacoes")
	minRecom := 1
	if minRecomStr != "" {
		if val, err := strconv.Atoi(minRecomStr); err == nil && val > 0 {
			minRecom = val
		}
	}

	subutilizados, err := h.repo.GetExerciciosSubutilizados(r.Context(), minRecom)
	if err != nil {
		writeJSONError(w, "Failed to retrieve subutilized exercises report: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(subutilizados)
}

func (h *RelatoriosHandler) GetAprovacao(w http.ResponseWriter, r *http.Request) {
	diasStr := r.URL.Query().Get("dias")
	dias := 30
	if diasStr != "" {
		if val, err := strconv.Atoi(diasStr); err == nil && val > 0 {
			dias = val
		}
	}

	aprovacoes, err := h.repo.GetRelatorioAprovacao(r.Context(), dias)
	if err != nil {
		writeJSONError(w, "Failed to retrieve approval rate report: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(aprovacoes)
}
