package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/daniels"
	"staff_app/internal/domain"
	"staff_app/internal/repositories"

	"github.com/go-chi/chi/v5"
)

// VDOTHandler handles HTTP requests for VDOT calculations and test management.
type VDOTHandler struct {
	repo repositories.Teste3kmRepository
}

// NewVDOTHandler creates a new VDOTHandler instance.
func NewVDOTHandler(repo repositories.Teste3kmRepository) *VDOTHandler {
	return &VDOTHandler{repo: repo}
}

// CreateRequest defines the request body for creating a new 3k test record.
type CreateRequest struct {
	TempoSegundos int    `json:"tempo_segundos"`
	PSE           *int   `json:"pse,omitempty"`
	Fonte         string `json:"fonte,omitempty"`
	DataTeste     string `json:"data_teste,omitempty"` // YYYY-MM-DD
	Observacoes   string `json:"observacoes,omitempty"`
}

// writeJSONError sends a JSON formatted error response.
func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Create handles POST /api/v1/alunos/{id}/vdot
func (h *VDOTHandler) Create(w http.ResponseWriter, r *http.Request) {
	alunoIDStr := chi.URLParam(r, "id")
	alunoID, err := strconv.ParseInt(alunoIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.TempoSegundos <= 0 {
		writeJSONError(w, "tempo_segundos must be a positive integer", http.StatusBadRequest)
		return
	}

	// Calculate VDOT and zones
	vdotRes, err := daniels.Calculate3kTest(req.TempoSegundos)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Parse date
	dataTeste := time.Now()
	if req.DataTeste != "" {
		if t, err := time.Parse("2006-01-02", req.DataTeste); err == nil {
			dataTeste = t
		} else {
			writeJSONError(w, "Invalid date format. Expected YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	}

	fonte := req.Fonte
	if fonte == "" {
		fonte = "manual"
	}

	// 1. Process PSE and calculate IndiceConfianca
	var pse *int
	var confianca int

	if req.PSE != nil {
		p := *req.PSE
		if p >= 0 && p <= 10 {
			pse = &p
			if p >= 9 {
				confianca = 85
			} else {
				confianca = 70
			}
		} else {
			// Out of bounds PSE is treated as nil/missing
			pse = nil
			confianca = 50
		}
	} else {
		pse = nil
		confianca = 50
	}

	// 2. Trim and limit observacoes to 500 characters
	observacoes := strings.TrimSpace(req.Observacoes)
	if len(observacoes) > 500 {
		observacoes = observacoes[:500]
	}

	teste := &domain.Teste3km{
		AlunoID:         alunoID,
		DataTeste:       dataTeste,
		TempoSegundos:   req.TempoSegundos,
		PSE:             pse,
		Fonte:           fonte,
		VDOT:            vdotRes.VDOT,
		FTPPaceSegundos: vdotRes.FTPPaceSeconds,
		PaceZ1Min:       vdotRes.PaceZ1Min,
		PaceZ1Max:       vdotRes.PaceZ1Max,
		PaceZ2Min:       vdotRes.PaceZ2Min,
		PaceZ2Max:       vdotRes.PaceZ2Max,
		PaceZ3Min:       vdotRes.PaceZ3Min,
		PaceZ3Max:       vdotRes.PaceZ3Max,
		PaceZ4Min:       vdotRes.PaceZ4Min,
		PaceZ4Max:       vdotRes.PaceZ4Max,
		PaceZ5Min:       vdotRes.PaceZ5Min,
		PaceZ5Max:       vdotRes.PaceZ5Max,
		IndiceConfianca: &confianca,
		Observacoes:     observacoes,
	}

	if err := h.repo.Create(r.Context(), teste); err != nil {
		// Detect foreign key violation in SQLite
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			writeJSONError(w, "Failed to save test. Student does not exist.", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(teste)
}

// List handles GET /api/v1/alunos/{id}/vdot
func (h *VDOTHandler) List(w http.ResponseWriter, r *http.Request) {
	alunoIDStr := chi.URLParam(r, "id")
	alunoID, err := strconv.ParseInt(alunoIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	tests, err := h.repo.ListByAlunoID(r.Context(), alunoID)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Always return an empty JSON array instead of null
	if tests == nil {
		tests = []*domain.Teste3km{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tests)
}

// Delete handles DELETE /api/v1/alunos/{id}/vdot/{teste_id}
func (h *VDOTHandler) Delete(w http.ResponseWriter, r *http.Request) {
	alunoIDStr := chi.URLParam(r, "id")
	alunoID, err := strconv.ParseInt(alunoIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	testeIDStr := chi.URLParam(r, "teste_id")
	testeID, err := strconv.ParseInt(testeIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid test ID", http.StatusBadRequest)
		return
	}

	if err := h.repo.Delete(r.Context(), testeID, alunoID); err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			writeJSONError(w, "Test not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
