package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/exercicios/csvsync"
	"staff_app/internal/sqlite"

	"github.com/go-chi/chi/v5"
)

type ExercicioHandler struct {
	repo *sqlite.ExercicioRepository
}

func NewExercicioHandler(db *sqlite.DB) *ExercicioHandler {
	return &ExercicioHandler{
		repo: sqlite.NewExercicioRepository(db),
	}
}

type combinedExercise struct {
	Codigo      int    `json:"codigo"`
	Nome        string `json:"nome"`
	Grupo       string `json:"grupo"`
	MusculoFoco string `json:"musculo_foco"`
	Categoria   string `json:"categoria"`
	Origem      string `json:"origem"`
}

// ListBiblioteca lists active exercises from the DB (catalog materializado na startup).
func (h *ExercicioHandler) ListBiblioteca(w http.ResponseWriter, r *http.Request) {
	busca := strings.TrimSpace(r.URL.Query().Get("busca"))
	grupo := strings.TrimSpace(r.URL.Query().Get("grupo_muscular"))
	if grupo == "" {
		grupo = strings.TrimSpace(r.URL.Query().Get("grupo"))
	}

	dbList, err := h.repo.List(r.Context(), map[string]string{"status": "ativo"})
	if err != nil {
		writeJSONError(w, "Failed to load database exercises", http.StatusInternalServerError)
		return
	}

	combined := make([]combinedExercise, 0, len(dbList))
	for _, ex := range dbList {
		origem := "database"
		if ex.CriadoPor == csvsync.CatalogMarker {
			origem = "csv"
		}
		combined = append(combined, combinedExercise{
			Codigo:      ex.Codigo,
			Nome:        ex.Nome,
			Grupo:       ex.GrupoMuscular,
			MusculoFoco: ex.MusculoFoco,
			Categoria:   ex.Categoria,
			Origem:      origem,
		})
	}

	var filtered []combinedExercise
	for _, ex := range combined {
		matchBusca := true
		if busca != "" {
			matchBusca = strings.Contains(strings.ToLower(ex.Nome), strings.ToLower(busca)) ||
				strconv.Itoa(ex.Codigo) == busca
		}

		matchGrupo := true
		if grupo != "" && !strings.EqualFold(grupo, "todos") {
			if strings.EqualFold(grupo, "terapêuticos") || strings.EqualFold(grupo, "terapeuticos") {
				matchGrupo = ex.Categoria == "terapeutico"
			} else {
				matchGrupo = strings.EqualFold(ex.Grupo, grupo)
			}
		}

		if matchBusca && matchGrupo {
			filtered = append(filtered, ex)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Codigo > filtered[j].Codigo
	})

	var normaisCount, terapeuticosCount int
	for _, ex := range filtered {
		if ex.Categoria == "terapeutico" {
			terapeuticosCount++
		} else {
			normaisCount++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"exercicios":         filtered,
		"total":              len(filtered),
		"total_normais":      normaisCount,
		"total_terapeuticos": terapeuticosCount,
	})
}

type createExercicioRequest struct {
	Nome                 string `json:"nome"`
	Categoria            string `json:"categoria"`
	DescricaoTerapeutica string `json:"descricao_terapeutica"`
	Indicacoes           string `json:"indicacoes"`
	Contraindicacoes     string `json:"contraindicacoes"`
	GrupoMuscular        string `json:"grupo_muscular"`
	MusculoFoco          string `json:"musculo_foco"`
	TipoExercicio        string `json:"tipo_exercicio"`
	Intensidade          string `json:"intensidade"`
	NivelPrioridade      *int   `json:"nivel_prioridade"`
	FonteCientifica      string `json:"fonte_cientifica"`
	UrlSecundaria        string `json:"url_secundaria"`
	NotasProfissional    string `json:"notas_profissional"`
}

// Create cria um novo exercício personalizado (Normal/Terapêutico)
func (h *ExercicioHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createExercicioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	req.Nome = strings.TrimSpace(req.Nome)
	if req.Nome == "" {
		writeJSONError(w, "Nome do exercício é obrigatório.", http.StatusBadRequest)
		return
	}

	req.Categoria = strings.ToLower(strings.TrimSpace(req.Categoria))
	if req.Categoria != "normal" && req.Categoria != "terapeutico" {
		writeJSONError(w, "Categoria inválida. Use 'terapeutico' ou 'normal'.", http.StatusBadRequest)
		return
	}

	// Verificar duplicidade de nome
	existing, err := h.repo.GetByNome(r.Context(), req.Nome)
	if err != nil {
		writeJSONError(w, "Error checking duplicate name", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		writeJSONError(w, "Já existe um exercício com este nome.", http.StatusBadRequest)
		return
	}

	// Determinar ranges e descobrir próximo código
	var min, max int
	if req.Categoria == "terapeutico" {
		min, max = 5000, 5999
	} else {
		min, max = 6000, 9999
	}

	maxCode, err := h.repo.GetMaxCodigoInRange(r.Context(), min, max)
	if err != nil {
		writeJSONError(w, "Error generating code", http.StatusInternalServerError)
		return
	}

	nextCode := maxCode + 1
	if maxCode == 0 {
		nextCode = min
	}
	if nextCode > max {
		writeJSONError(w, fmt.Sprintf("Range de códigos esgotado para categoria '%s'", req.Categoria), http.StatusBadRequest)
		return
	}

	var criadoPor string = "admin"
	if u, ok := r.Context().Value(userContextKey{}).(*domain.User); ok && u != nil {
		criadoPor = u.Username
	}

	prioridade := 2
	if req.NivelPrioridade != nil {
		prioridade = *req.NivelPrioridade
	}

	ex := &domain.ExercicioReabilitacao{
		Codigo:               nextCode,
		Nome:                 req.Nome,
		Categoria:            req.Categoria,
		DescricaoTerapeutica: req.DescricaoTerapeutica,
		Descricao:            req.DescricaoTerapeutica, // alias
		Indicacoes:           req.Indicacoes,
		Contraindicacoes:     req.Contraindicacoes,
		RestricoesSugeridas:  req.Contraindicacoes, // alias
		GrupoMuscular:        req.GrupoMuscular,
		MusculoFoco:          req.MusculoFoco,
		TipoExercicio:        req.TipoExercicio,
		Intensidade:          req.Intensidade,
		NivelPrioridade:      prioridade,
		FonteCientifica:      req.FonteCientifica,
		Url:                  fmt.Sprintf("https://rcstorestaff.com.br/exercicios_html/%d", nextCode),
		UrlSecundaria:        req.UrlSecundaria,
		VideoUrl:             req.UrlSecundaria, // alias
		CriadoPor:            criadoPor,
		CriadoEm:             time.Now(),
		Status:               "ativo",
		NotasProfissional:    req.NotasProfissional,
	}

	if err := h.repo.Create(r.Context(), ex); err != nil {
		writeJSONError(w, "Failed to create exercise: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(ex)
}

// List lista os exercícios cadastrados no banco de dados com filtros
func (h *ExercicioHandler) List(w http.ResponseWriter, r *http.Request) {
	filters := make(map[string]string)
	filters["categoria"] = r.URL.Query().Get("categoria")
	filters["status"] = r.URL.Query().Get("status")
	filters["grupo_muscular"] = r.URL.Query().Get("grupo_muscular")
	filters["tipo_exercicio"] = r.URL.Query().Get("tipo_exercicio")
	filters["busca"] = r.URL.Query().Get("busca")

	list, err := h.repo.List(r.Context(), filters)
	if err != nil {
		writeJSONError(w, "Failed to list exercises", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(list)
}

// GetByID obtém detalhes do exercício
func (h *ExercicioHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	codeStr := chi.URLParam(r, "codigo")
	codigo, err := strconv.Atoi(codeStr)
	if err != nil {
		writeJSONError(w, "Invalid code parameter", http.StatusBadRequest)
		return
	}

	ex, err := h.repo.GetByCodigo(r.Context(), codigo)
	if err != nil {
		writeJSONError(w, "Error retrieving exercise", http.StatusInternalServerError)
		return
	}
	if ex == nil {
		writeJSONError(w, "Exercício não encontrado", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ex)
}

// Update edita atributos de um exercício (bloqueia alteração de codigo e categoria)
func (h *ExercicioHandler) Update(w http.ResponseWriter, r *http.Request) {
	codeStr := chi.URLParam(r, "codigo")
	codigo, err := strconv.Atoi(codeStr)
	if err != nil {
		writeJSONError(w, "Invalid code parameter", http.StatusBadRequest)
		return
	}

	ex, err := h.repo.GetByCodigo(r.Context(), codigo)
	if err != nil {
		writeJSONError(w, "Error retrieving exercise", http.StatusInternalServerError)
		return
	}
	if ex == nil {
		writeJSONError(w, "Exercício não encontrado", http.StatusNotFound)
		return
	}

	var req createExercicioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	req.Nome = strings.TrimSpace(req.Nome)
	if req.Nome == "" {
		writeJSONError(w, "Nome do exercício é obrigatório.", http.StatusBadRequest)
		return
	}

	// Se alterou o nome, verificar duplicidade
	if !strings.EqualFold(req.Nome, ex.Nome) {
		existing, err := h.repo.GetByNome(r.Context(), req.Nome)
		if err == nil && existing != nil {
			writeJSONError(w, "Já existe um exercício com este nome.", http.StatusBadRequest)
			return
		}
	}

	var atualizadoPor string = "admin"
	if u, ok := r.Context().Value(userContextKey{}).(*domain.User); ok && u != nil {
		atualizadoPor = u.Username
	}

	ex.Nome = req.Nome
	ex.DescricaoTerapeutica = req.DescricaoTerapeutica
	ex.Descricao = req.DescricaoTerapeutica // alias
	ex.Indicacoes = req.Indicacoes
	ex.Contraindicacoes = req.Contraindicacoes
	ex.RestricoesSugeridas = req.Contraindicacoes // alias
	ex.GrupoMuscular = req.GrupoMuscular
	ex.MusculoFoco = req.MusculoFoco
	ex.TipoExercicio = req.TipoExercicio
	ex.Intensidade = req.Intensidade
	if req.NivelPrioridade != nil {
		ex.NivelPrioridade = *req.NivelPrioridade
	}
	ex.FonteCientifica = req.FonteCientifica
	ex.UrlSecundaria = req.UrlSecundaria
	ex.VideoUrl = req.UrlSecundaria // alias
	ex.NotasProfissional = req.NotasProfissional
	ex.AtualizadoPor = atualizadoPor
	now := time.Now()
	ex.AtualizadoEm = &now

	if err := h.repo.Update(r.Context(), ex); err != nil {
		writeJSONError(w, "Failed to update exercise", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ex)
}

// Activar ativa um exercício desativado
func (h *ExercicioHandler) Activar(w http.ResponseWriter, r *http.Request) {
	codeStr := chi.URLParam(r, "codigo")
	codigo, err := strconv.Atoi(codeStr)
	if err != nil {
		writeJSONError(w, "Invalid code parameter", http.StatusBadRequest)
		return
	}

	ex, err := h.repo.GetByCodigo(r.Context(), codigo)
	if err != nil {
		writeJSONError(w, "Error retrieving exercise", http.StatusInternalServerError)
		return
	}
	if ex == nil {
		writeJSONError(w, "Exercício não encontrado", http.StatusNotFound)
		return
	}

	ex.Status = "ativo"
	var username string = "admin"
	if u, ok := r.Context().Value(userContextKey{}).(*domain.User); ok && u != nil {
		username = u.Username
	}
	ex.AtualizadoPor = username
	now := time.Now()
	ex.AtualizadoEm = &now

	if err := h.repo.Update(r.Context(), ex); err != nil {
		writeJSONError(w, "Failed to activate exercise", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "message": "Exercício reativado com sucesso."})
}

// Desactivar desativa um exercício (soft delete)
func (h *ExercicioHandler) Desactivar(w http.ResponseWriter, r *http.Request) {
	codeStr := chi.URLParam(r, "codigo")
	codigo, err := strconv.Atoi(codeStr)
	if err != nil {
		writeJSONError(w, "Invalid code parameter", http.StatusBadRequest)
		return
	}

	ex, err := h.repo.GetByCodigo(r.Context(), codigo)
	if err != nil {
		writeJSONError(w, "Error retrieving exercise", http.StatusInternalServerError)
		return
	}
	if ex == nil {
		writeJSONError(w, "Exercício não encontrado", http.StatusNotFound)
		return
	}

	ex.Status = "inativo"
	var username string = "admin"
	if u, ok := r.Context().Value(userContextKey{}).(*domain.User); ok && u != nil {
		username = u.Username
	}
	ex.AtualizadoPor = username
	now := time.Now()
	ex.AtualizadoEm = &now

	if err := h.repo.Update(r.Context(), ex); err != nil {
		writeJSONError(w, "Failed to deactivate exercise", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "message": "Exercício desativado com sucesso."})
}

// Delete realiza hard delete permanente (requer header X-Confirm-Hard-Delete: CONFIRMAR)
func (h *ExercicioHandler) Delete(w http.ResponseWriter, r *http.Request) {
	confirm := r.Header.Get("X-Confirm-Hard-Delete")
	if confirm != "CONFIRMAR" {
		writeJSONError(w, "⚠️ Você deve fornecer o cabeçalho X-Confirm-Hard-Delete: CONFIRMAR para deletar permanentemente o exercício.", http.StatusBadRequest)
		return
	}

	codeStr := chi.URLParam(r, "codigo")
	codigo, err := strconv.Atoi(codeStr)
	if err != nil {
		writeJSONError(w, "Invalid code parameter", http.StatusBadRequest)
		return
	}

	ex, err := h.repo.GetByCodigo(r.Context(), codigo)
	if err != nil {
		writeJSONError(w, "Error retrieving exercise", http.StatusInternalServerError)
		return
	}
	if ex == nil {
		writeJSONError(w, "Exercício não encontrado", http.StatusNotFound)
		return
	}

	if err := h.repo.Delete(r.Context(), codigo); err != nil {
		writeJSONError(w, "Failed to hard delete exercise", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "message": "Exercício deletado permanentemente do sistema."})
}

// Estatisticas retorna contagens gerais e ranges livres
func (h *ExercicioHandler) Estatisticas(w http.ResponseWriter, r *http.Request) {
	stats, err := h.repo.GetEstatisticas(r.Context())
	if err != nil {
		writeJSONError(w, "Failed to get statistics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(stats)
}

// Grupos lista grupos musculares únicos
func (h *ExercicioHandler) Grupos(w http.ResponseWriter, r *http.Request) {
	grupos, err := h.repo.GetUniqueGrupos(r.Context())
	if err != nil {
		writeJSONError(w, "Failed to get groups", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(grupos)
}

// Tipos lista tipos de exercícios únicos
func (h *ExercicioHandler) Tipos(w http.ResponseWriter, r *http.Request) {
	tipos, err := h.repo.GetUniqueTipos(r.Context())
	if err != nil {
		writeJSONError(w, "Failed to get types", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tipos)
}

// ----------------------------------------------------------------------------
// SUGESTÕES DE REABILITAÇÃO (JUDGE SYSTEM)
// ----------------------------------------------------------------------------

// ListSugestoes lista sugestões pendentes
func (h *ExercicioHandler) ListSugestoes(w http.ResponseWriter, r *http.Request) {
	prioStr := r.URL.Query().Get("prioridade")
	ordem := r.URL.Query().Get("ordem")

	var prio *int
	if prioStr != "" {
		val, err := strconv.Atoi(prioStr)
		if err == nil {
			prio = &val
		}
	}

	list, err := h.repo.ListSugestoes(r.Context(), prio, ordem)
	if err != nil {
		writeJSONError(w, "Failed to list suggestions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(list)
}

// ApproveSugestao aprova sugestão e insere exercício na faixa 5000-5999 (Transacional)
func (h *ExercicioHandler) ApproveSugestao(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSONError(w, "Invalid ID parameter", http.StatusBadRequest)
		return
	}

	sug, err := h.repo.GetSugestaoByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Error retrieving suggestion", http.StatusInternalServerError)
		return
	}
	if sug == nil {
		writeJSONError(w, "Sugestão não encontrada ou já aprovada/rejeitada", http.StatusNotFound)
		return
	}

	var req createExercicioRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // Corpo é opcional para edição pré-aprovação

	nomeFinal := strings.TrimSpace(req.Nome)
	if nomeFinal == "" {
		nomeFinal = sug.NomeExercicio
	}

	// Verificar duplicidade de nome final
	existing, err := h.repo.GetByNome(r.Context(), nomeFinal)
	if err == nil && existing != nil {
		writeJSONError(w, "Já existe um exercício com este nome.", http.StatusBadRequest)
		return
	}

	var approvedBy string = "admin"
	if u, ok := r.Context().Value(userContextKey{}).(*domain.User); ok && u != nil {
		approvedBy = u.Username
	}

	desc := strings.TrimSpace(req.DescricaoTerapeutica)
	if desc == "" {
		desc = sug.JustificativaClinica
	}

	ex := &domain.ExercicioReabilitacao{
		Nome:                 nomeFinal,
		Categoria:            "terapeutico",
		DescricaoTerapeutica: desc,
		Descricao:            desc,
		Indicacoes:           req.Indicacoes,
		Contraindicacoes:     req.Contraindicacoes,
		RestricoesSugeridas:  req.Contraindicacoes,
		GrupoMuscular:        req.GrupoMuscular,
		MusculoFoco:          req.MusculoFoco,
		TipoExercicio:        sug.TipoExercicio,
		NivelPrioridade:      sug.NivelPrioridade,
		FonteCientifica:      sug.RagFonte,
		CriadoPor:            approvedBy,
		CriadoEm:             time.Now(),
		Status:               "ativo",
		NotasProfissional:    req.NotasProfissional,
	}

	code, err := h.repo.AprovarSugestao(r.Context(), id, ex, approvedBy)
	if err != nil {
		writeJSONError(w, "Failed to approve suggestion: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "success",
		"codigo": code,
		"url":    fmt.Sprintf("https://rcstorestaff.com.br/exercicios_html/%d", code),
	})
}

type rejectSugestaoRequest struct {
	Motivo string `json:"motivo"`
}

// RejectSugestao rejeita sugestão
func (h *ExercicioHandler) RejectSugestao(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSONError(w, "Invalid ID parameter", http.StatusBadRequest)
		return
	}

	var req rejectSugestaoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	req.Motivo = strings.TrimSpace(req.Motivo)
	if req.Motivo == "" {
		writeJSONError(w, "Motivo da rejeição é obrigatório", http.StatusBadRequest)
		return
	}

	var rejectedBy string = "admin"
	if u, ok := r.Context().Value(userContextKey{}).(*domain.User); ok && u != nil {
		rejectedBy = u.Username
	}

	if err := h.repo.RejeitarSugestao(r.Context(), id, req.Motivo, rejectedBy); err != nil {
		writeJSONError(w, "Failed to reject suggestion: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "message": "Sugestão rejeitada com sucesso."})
}
