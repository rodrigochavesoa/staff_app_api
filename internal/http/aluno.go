package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"staff_app/internal/domain"
	"staff_app/internal/sqlite"

	"github.com/go-chi/chi/v5"
)

// AlunoHandler handles REST HTTP requests for students (alunos).
type AlunoHandler struct {
	repo *sqlite.AlunoRepository
}

// NewAlunoHandler creates a new AlunoHandler instance.
func NewAlunoHandler(db *sqlite.DB) *AlunoHandler {
	return &AlunoHandler{
		repo: sqlite.NewAlunoRepository(db),
	}
}

// AlunoRequest represents the payload for POST/PUT requests.
type AlunoRequest struct {
	Nome                 string   `json:"nome"`
	Idade                int      `json:"idade"`
	Sexo                 string   `json:"sexo"`
	Email                string   `json:"email"`
	Telefone             string   `json:"telefone,omitempty"`
	Objetivo             string   `json:"objetivo,omitempty"`
	ExclusoesPermanentes string   `json:"exclusoes_permanentes,omitempty"`
	Turma                string   `json:"turma,omitempty"`
	UsuarioID            *int64   `json:"usuario_id,omitempty"`
	PlanoID              *int64   `json:"plano_id,omitempty"`
	PlanoValor           *float64 `json:"plano_valor,omitempty"`
	PlanoPago            bool     `json:"plano_pago,omitempty"`
	PlanoAtivo           bool     `json:"plano_ativo,omitempty"`
	PlanoInicio          *string  `json:"plano_inicio,omitempty"`
	PlanoFim             *string  `json:"plano_fim,omitempty"`
	CadastroAprovado     bool     `json:"cadastro_aprovado,omitempty"`
	CadastroAprovadoPor  *int64   `json:"cadastro_aprovado_por,omitempty"`
	PreRegistroID        *int64   `json:"pre_registro_id,omitempty"`
	Ativo                *bool    `json:"ativo,omitempty"`
}

// safeRuneSlice slices a string safely by runes to prevent UTF-8 corruption.
func safeRuneSlice(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}

// validate verifies request input validation rules.
func (req *AlunoRequest) validate() error {
	req.Nome = strings.TrimSpace(req.Nome)
	if req.Nome == "" {
		return errors.New("nome is required")
	}
	if utf8.RuneCountInString(req.Nome) > 100 {
		return errors.New("nome cannot exceed 100 characters")
	}

	if req.Idade <= 0 || req.Idade > 120 {
		return errors.New("idade must be a valid age between 1 and 120")
	}

	req.Sexo = strings.ToUpper(strings.TrimSpace(req.Sexo))
	if req.Sexo != "M" && req.Sexo != "F" && req.Sexo != "O" {
		return errors.New("sexo must be M, F, or O")
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		return errors.New("valid email is required")
	}
	if utf8.RuneCountInString(req.Email) > 100 {
		return errors.New("email cannot exceed 100 characters")
	}

	// Apply safe rune slicing for optional fields
	req.Telefone = safeRuneSlice(strings.TrimSpace(req.Telefone), 20)
	req.Objetivo = safeRuneSlice(strings.TrimSpace(req.Objetivo), 250)
	req.ExclusoesPermanentes = safeRuneSlice(strings.TrimSpace(req.ExclusoesPermanentes), 1000)
	req.Turma = safeRuneSlice(strings.TrimSpace(req.Turma), 10)

	return nil
}

// Create handles POST /api/v1/alunos
func (h *AlunoHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req AlunoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if err := req.validate(); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ativo := true
	if req.Ativo != nil {
		ativo = *req.Ativo
	}

	var approvalEm *time.Time
	if req.CadastroAprovado {
		t := time.Now()
		approvalEm = &t
	}

	aluno := &domain.Aluno{
		Nome:                 req.Nome,
		Idade:                req.Idade,
		Sexo:                 req.Sexo,
		Email:                req.Email,
		Telefone:             req.Telefone,
		Objetivo:             req.Objetivo,
		ExclusoesPermanentes: req.ExclusoesPermanentes,
		Turma:                req.Turma,
		UsuarioID:            req.UsuarioID,
		PlanoID:              req.PlanoID,
		PlanoValor:           req.PlanoValor,
		PlanoPago:            req.PlanoPago,
		PlanoAtivo:           req.PlanoAtivo,
		PlanoInicio:          req.PlanoInicio,
		PlanoFim:             req.PlanoFim,
		CadastroAprovado:     req.CadastroAprovado,
		CadastroAprovadoPor:  req.CadastroAprovadoPor,
		CadastroAprovadoEm:   approvalEm,
		PreRegistroID:        req.PreRegistroID,
		Ativo:                ativo,
	}

	if err := h.repo.Create(r.Context(), aluno); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeJSONError(w, "Email is already registered", http.StatusConflict)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(aluno)
}

// GetByID handles GET /api/v1/alunos/{id}
func (h *AlunoHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	aluno, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Student not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(aluno)
}

// List handles GET /api/v1/alunos
func (h *AlunoHandler) List(w http.ResponseWriter, r *http.Request) {
	busca := r.URL.Query().Get("busca")
	inativos := r.URL.Query().Get("inativos") == "1"

	alunos, err := h.repo.List(r.Context(), busca, inativos)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if alunos == nil {
		alunos = []*domain.Aluno{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(alunos)
}

// Update handles PUT /api/v1/alunos/{id}
func (h *AlunoHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	// Make sure the student exists
	existing, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Student not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var req AlunoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if err := req.validate(); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Map updated fields
	existing.Nome = req.Nome
	existing.Idade = req.Idade
	existing.Sexo = req.Sexo
	existing.Email = req.Email
	existing.Telefone = req.Telefone
	existing.Objetivo = req.Objetivo
	existing.ExclusoesPermanentes = req.ExclusoesPermanentes
	existing.Turma = req.Turma
	existing.UsuarioID = req.UsuarioID
	existing.PlanoID = req.PlanoID
	existing.PlanoValor = req.PlanoValor
	existing.PlanoPago = req.PlanoPago
	existing.PlanoAtivo = req.PlanoAtivo
	existing.PlanoInicio = req.PlanoInicio
	existing.PlanoFim = req.PlanoFim

	// Handle approval transitions
	if req.CadastroAprovado && !existing.CadastroAprovado {
		t := time.Now()
		existing.CadastroAprovadoEm = &t
	}
	existing.CadastroAprovado = req.CadastroAprovado
	existing.CadastroAprovadoPor = req.CadastroAprovadoPor
	existing.PreRegistroID = req.PreRegistroID

	if req.Ativo != nil {
		existing.Ativo = *req.Ativo
	}

	if err := h.repo.Update(r.Context(), existing); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeJSONError(w, "Email is already registered by another student", http.StatusConflict)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(existing)
}

// Delete handles DELETE /api/v1/alunos/{id} (soft delete)
func (h *AlunoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Student not found or already inactive", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Reactivate handles POST /api/v1/alunos/{id}/reativar
func (h *AlunoHandler) Reactivate(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	if err := h.repo.Reactivate(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Student not found or already active", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
