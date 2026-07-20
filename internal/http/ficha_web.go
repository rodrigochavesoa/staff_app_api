package http

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/repositories"

	"github.com/go-chi/chi/v5"
)

// FichaWebHandler handles API endpoints for public training link management.
type FichaWebHandler struct {
	repo      repositories.FichaRepository
	alunoRepo repositories.AlunoRepository
}

// NewFichaWebHandler creates a new FichaWebHandler instance.
func NewFichaWebHandler(repo repositories.FichaRepository, aluno repositories.AlunoRepository) *FichaWebHandler {
	return &FichaWebHandler{
		repo:      repo,
		alunoRepo: aluno,
	}
}

// generateURLSafeHash generates a random URL-safe hash (32 bytes).
func generateURLSafeHash() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateFichaRequest payload for POST /api/v1/criar-ficha.
type CreateFichaRequest struct {
	FichaID  int64           `json:"ficha_id"`
	AlunoID  int64           `json:"aluno_id"`
	UserID   *int64          `json:"user_id,omitempty"`
	Conteudo json.RawMessage `json:"conteudo,omitempty"`
}

// CreateFichaResponse represents response returned on creation.
type CreateFichaResponse struct {
	Hash         string `json:"hash"`
	Url          string `json:"url"`
	UrlCompleta  string `json:"url_completa"`
	CriadoEm     string `json:"criado_em"`
	ExpiraEm     string `json:"expira_em"`
	DiasValidade int    `json:"dias_validade"`
}

// FichaAlunoSummary represents basic student info nested in metadata.
type FichaAlunoSummary struct {
	ID    int64  `json:"id"`
	Nome  string `json:"nome"`
	Idade int    `json:"idade"`
	Sexo  string `json:"sexo"`
}

// AlunoFichasResponse represents list of links returned for a student.
type AlunoFichasResponse struct {
	AlunoID int64              `json:"aluno_id"`
	Total   int                `json:"total"`
	Fichas  []*domain.FichaWeb `json:"fichas"`
}

// FichaMetadata nested metadata details for retrieval.
type FichaMetadata struct {
	CriadoEm      string            `json:"criado_em"`
	ExpiraEm      string            `json:"expira_em"`
	DiasRestantes int               `json:"dias_restantes"`
	Acessos       int               `json:"acessos"`
	Aluno         FichaAlunoSummary `json:"aluno"`
}

// FichaPublicaResponse represents retrieved public link response structure.
type FichaPublicaResponse struct {
	Hash     string          `json:"hash"`
	Conteudo json.RawMessage `json:"conteudo"`
	Metadata FichaMetadata   `json:"metadata"`
}

// RenewFichaRequest payload for POST /api/v1/renovar/{hash}.
type RenewFichaRequest struct {
	Dias int `json:"dias"`
}

// RenewFichaResponse represents response returned on renewal.
type RenewFichaResponse struct {
	Hash            string `json:"hash"`
	ExpiraEm        string `json:"expira_em"`
	DiasAdicionados int    `json:"dias_adicionados"`
}

// DeactivateFichaResponse represents response returned on deactivation.
type DeactivateFichaResponse struct {
	Message string `json:"message"`
	Hash    string `json:"hash"`
}

// Create handles POST /api/v1/criar-ficha
func (h *FichaWebHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateFichaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.FichaID <= 0 || req.AlunoID <= 0 {
		writeJSONError(w, "ficha_id and aluno_id must be valid positive integers", http.StatusBadRequest)
		return
	}

	// 1. Verify student exists
	_, err := h.alunoRepo.GetByID(r.Context(), req.AlunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Student not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Failed to verify student existence", http.StatusInternalServerError)
		return
	}

	// 2. Load or define content
	var conteudoStr string
	if len(req.Conteudo) > 0 {
		conteudoStr = string(req.Conteudo)
	} else {
		// Fetch latest snapshot content from legacy table
		var err error
		conteudoStr, err = h.repo.GetFichaJSON(r.Context(), req.FichaID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, "Ficha treino not found in legacy system", http.StatusNotFound)
				return
			}
			writeJSONError(w, "Failed to retrieve legacy training plan snapshot", http.StatusInternalServerError)
			return
		}
	}

	// Validate content is a valid JSON object map
	var temp map[string]any
	if err := json.Unmarshal([]byte(conteudoStr), &temp); err != nil {
		writeJSONError(w, "Content must be a valid JSON object structure", http.StatusBadRequest)
		return
	}

	// 3. Generate unique URL-safe hash
	hash, err := generateURLSafeHash()
	if err != nil {
		writeJSONError(w, "Internal server error generating hash", http.StatusInternalServerError)
		return
	}

	// 4. Set expiration to 30 days
	now := time.Now()
	validDays := 30
	expiration := now.Add(time.Duration(validDays) * 24 * time.Hour)

	fw := &domain.FichaWeb{
		Hash:         hash,
		FichaID:      req.FichaID,
		AlunoID:      req.AlunoID,
		UserID:       req.UserID,
		ConteudoJSON: conteudoStr,
		CriadoEm:     now,
		ExpiraEm:     expiration,
		Ativo:        true,
	}

	if err := h.repo.Create(r.Context(), fw); err != nil {
		writeJSONError(w, "Failed to create public link record", http.StatusInternalServerError)
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	siteURL := scheme + "://" + r.Host

	resp := CreateFichaResponse{
		Hash:         fw.Hash,
		Url:          "/ficha/" + fw.Hash,
		UrlCompleta:  siteURL + "/ficha/" + fw.Hash,
		CriadoEm:     fw.CriadoEm.Format(time.RFC3339),
		ExpiraEm:     fw.ExpiraEm.Format(time.RFC3339),
		DiasValidade: validDays,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetByHashJSON handles GET /api/v1/ficha/{hash}/json
func (h *FichaWebHandler) GetByHashJSON(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		writeJSONError(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	fw, err := h.repo.GetByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Public link not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error retrieving link", http.StatusInternalServerError)
		return
	}

	// Validate status
	if !fw.Ativo {
		writeJSONError(w, "This public link has been deactivated", http.StatusGone)
		return
	}

	// Validate expiration
	if fw.ExpiraEm.Before(time.Now()) {
		writeJSONError(w, "This public link has expired", http.StatusGone)
		return
	}

	// Fetch student details for metadata
	aluno, err := h.alunoRepo.GetByID(r.Context(), fw.AlunoID)
	if err != nil {
		writeJSONError(w, "Failed to retrieve student details", http.StatusInternalServerError)
		return
	}

	// Log access statistics
	userAgent := r.Header.Get("User-Agent")
	if userAgent == "" {
		userAgent = "Unknown"
	}

	ipAddress := r.RemoteAddr
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		ipAddress = forwardedFor
	}

	if err := h.repo.IncrementAccessCount(r.Context(), hash, userAgent, ipAddress); err != nil {
		// Log but do not block response
	}

	diasRestantes := int(time.Until(fw.ExpiraEm).Hours() / 24)
	if diasRestantes < 0 {
		diasRestantes = 0
	}

	resp := FichaPublicaResponse{
		Hash:     fw.Hash,
		Conteudo: json.RawMessage(fw.ConteudoJSON),
		Metadata: FichaMetadata{
			CriadoEm:      fw.CriadoEm.Format(time.RFC3339),
			ExpiraEm:      fw.ExpiraEm.Format(time.RFC3339),
			DiasRestantes: diasRestantes,
			Acessos:       fw.Acessos + 1, // Include the current logged access
			Aluno: FichaAlunoSummary{
				ID:    aluno.ID,
				Nome:  aluno.Nome,
				Idade: aluno.Idade,
				Sexo:  aluno.Sexo,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetStats handles GET /api/v1/stats/{hash}
func (h *FichaWebHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		writeJSONError(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	stats, err := h.repo.GetStats(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Link stats not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error retrieving statistics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(stats)
}

// Renew handles POST /api/v1/renovar/{hash}
func (h *FichaWebHandler) Renew(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		writeJSONError(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	// Fetch current record to get ficha_id
	fw, err := h.repo.GetByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Public link not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error retrieving link", http.StatusInternalServerError)
		return
	}

	if !fw.Ativo {
		writeJSONError(w, "Cannot renew an inactive link", http.StatusBadRequest)
		return
	}

	var req RenewFichaRequest
	// Decode request body if provided (it is optional)
	dias := 30
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Dias > 0 {
			dias = req.Dias
		}
	}

	// Fetch latest snapshot content from legacy table to synchronize
	newContent, err := h.repo.GetFichaJSON(r.Context(), fw.FichaID)
	var newContentPtr *string
	if err == nil && newContent != "" {
		newContentPtr = &newContent
	}

	// Renew expiration days (if current expiration is in the future, add to it; if in the past, add from now)
	var newExpiration time.Time
	now := time.Now()
	if fw.ExpiraEm.After(now) {
		newExpiration = fw.ExpiraEm.Add(time.Duration(dias) * 24 * time.Hour)
	} else {
		newExpiration = now.Add(time.Duration(dias) * 24 * time.Hour)
	}

	if err := h.repo.Renew(r.Context(), hash, newExpiration, newContentPtr); err != nil {
		writeJSONError(w, "Internal server error renewing link", http.StatusInternalServerError)
		return
	}

	resp := RenewFichaResponse{
		Hash:            fw.Hash,
		ExpiraEm:        newExpiration.Format(time.RFC3339),
		DiasAdicionados: dias,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// Deactivate handles POST /api/v1/desativar/{hash}
func (h *FichaWebHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		writeJSONError(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	if err := h.repo.Deactivate(r.Context(), hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Public link not found or already inactive", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error deactivating link", http.StatusInternalServerError)
		return
	}

	resp := DeactivateFichaResponse{
		Message: "Ficha desativada com sucesso",
		Hash:    hash,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// ListByAluno handles GET /api/v1/aluno/{aluno_id}/fichas
func (h *FichaWebHandler) ListByAluno(w http.ResponseWriter, r *http.Request) {
	alunoIDStr := chi.URLParam(r, "aluno_id")
	alunoID, err := strconv.ParseInt(alunoIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	// Verify student exists
	_, err = h.alunoRepo.GetByID(r.Context(), alunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Student not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Failed to verify student existence", http.StatusInternalServerError)
		return
	}

	includeExpired := r.URL.Query().Get("include_expired") == "true" || r.URL.Query().Get("include_expired") == "1"

	list, err := h.repo.ListByAlunoID(r.Context(), alunoID, includeExpired)
	if err != nil {
		writeJSONError(w, "Internal server error listing student links", http.StatusInternalServerError)
		return
	}

	if list == nil {
		list = []*domain.FichaWeb{}
	}

	resp := AlunoFichasResponse{
		AlunoID: alunoID,
		Total:   len(list),
		Fichas:  list,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetFichaTreinoLetra handles public GET /api/v1/ficha/{hash}/treino/{letra}
func (h *FichaWebHandler) GetFichaTreinoLetra(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	letra := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "letra")))

	if hash == "" || letra == "" {
		writeJSONError(w, "Parâmetros 'hash' e 'letra' são obrigatórios", http.StatusBadRequest)
		return
	}

	fw, err := h.repo.GetByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Link público não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error retrieving link", http.StatusInternalServerError)
		return
	}

	// Validate status
	if !fw.Ativo {
		writeJSONError(w, "Este link público está desativado", http.StatusGone)
		return
	}

	// Validate expiration
	if fw.ExpiraEm.Before(time.Now()) {
		writeJSONError(w, "Este link público expirou", http.StatusGone)
		return
	}

	// Parse content json
	var content map[string]any
	if err := json.Unmarshal([]byte(fw.ConteudoJSON), &content); err != nil {
		writeJSONError(w, "Internal server error parsing workout structure", http.StatusInternalServerError)
		return
	}

	treinoLetra, ok := findTreinoByLetra(content["treinos"], letra)
	if !ok {
		writeJSONError(w, fmt.Sprintf("Treino correspondente à letra '%s' não encontrado", letra), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"letra":  letra,
		"treino": treinoLetra,
	})
}

// findTreinoByLetra resolves a workout letter from either:
//   - map form: {"A": {...}, "B": {...}}
//   - array form used by fichas periodizadas: [{"letra":"A",...}, {"letra":"B",...}]
func findTreinoByLetra(raw any, letra string) (any, bool) {
	letra = strings.ToUpper(strings.TrimSpace(letra))
	if letra == "" || raw == nil {
		return nil, false
	}
	if treinosMap, ok := raw.(map[string]any); ok {
		if treino, ok := treinosMap[letra]; ok {
			return treino, true
		}
		// tolerate lowercase keys in snapshots
		if treino, ok := treinosMap[strings.ToLower(letra)]; ok {
			return treino, true
		}
		return nil, false
	}
	treinosArr, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	for _, item := range treinosArr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemLetra, _ := m["letra"].(string)
		if strings.EqualFold(strings.TrimSpace(itemLetra), letra) {
			return item, true
		}
	}
	return nil, false
}

