package http

import (
	"database/sql"
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

type HistoricoHandler struct {
	repo                    repositories.HistoricoRepository
	alunoRepo               repositories.AlunoRepository
	fichaWebRepo            repositories.FichaRepository
	periodizacaoCorridaRepo repositories.PeriodizacaoCorridaRepository
	secretKey               string
}

func NewHistoricoHandler(
	repo repositories.HistoricoRepository,
	aluno repositories.AlunoRepository,
	fichaWeb repositories.FichaRepository,
	periodizacao repositories.PeriodizacaoCorridaRepository,
	secretKey string,
) *HistoricoHandler {
	if secretKey == "" {
		secretKey = defaultSecretKey
	}
	return &HistoricoHandler{
		repo:                    repo,
		alunoRepo:               aluno,
		fichaWebRepo:            fichaWeb,
		periodizacaoCorridaRepo: periodizacao,
		secretKey:               secretKey,
	}
}

type AlunoSearchQuery struct {
	Query  string `json:"q"`
	Limit  int    `json:"limit"`
	Ativo  string `json:"ativo"`
}

func (h *HistoricoHandler) SearchAlunos(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSONError(w, "Parâmetro 'q' deve conter pelo menos 2 caracteres", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 15
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
			if limit > 50 {
				limit = 50
			}
		}
	}

	ativo := r.URL.Query().Get("ativo")
	if ativo == "" {
		ativo = "true"
	}

	alunos, err := h.repo.SearchAlunos(r.Context(), q, limit, ativo)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if alunos == nil {
		alunos = []*domain.AlunoSearchResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"alunos": alunos,
	})
}

func (h *HistoricoHandler) GetFrequencia(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	alunoID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || alunoID <= 0 {
		writeJSONError(w, "ID do aluno inválido", http.StatusBadRequest)
		return
	}

	_, err = h.alunoRepo.GetByID(r.Context(), alunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	mesStr := r.URL.Query().Get("mes")
	mes := int(now.Month())
	if mesStr != "" {
		if val, err := strconv.Atoi(mesStr); err == nil && val >= 1 && val <= 12 {
			mes = val
		} else {
			writeJSONError(w, "Mês inválido", http.StatusBadRequest)
			return
		}
	}

	anoStr := r.URL.Query().Get("ano")
	ano := now.Year()
	if anoStr != "" {
		if val, err := strconv.Atoi(anoStr); err == nil && val > 1900 && val < 2100 {
			ano = val
		} else {
			writeJSONError(w, "Ano inválido", http.StatusBadRequest)
			return
		}
	}

	treinosList, err := h.repo.GetTreinosRealizadosByAluno(r.Context(), alunoID, mes, ano)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	corridas, err := h.periodizacaoCorridaRepo.ListByAlunoID(r.Context(), alunoID)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var completedRuns []domain.DiaFrequencia
	var activeRunningPlanned = 0

	for _, pc := range corridas {
		if pc.PlanoJSON == "" {
			continue
		}

		var pd domain.PlanoDetalhado
		if err := json.Unmarshal([]byte(pc.PlanoJSON), &pd); err != nil {
			continue
		}

		startDate, err := time.Parse("2006-01-02", pc.DataInicio)
		if err != nil {
			continue
		}

		// Alinha a semana ao Monday (calendário de frequência).
		segundaInicio := mondayOfWeek(startDate)

		for _, semana := range pd.Semanas {
			semanaStart := segundaInicio.AddDate(0, 0, (semana.Numero-1)*7)

			for _, treino := range semana.Treinos {
				treinoDate := semanaStart.AddDate(0, 0, treino.Dia-1)
				tYear, tMonth, _ := treinoDate.Date()

				if tYear == ano && int(tMonth) == mes {
					if pc.Status == "ativo" {
						activeRunningPlanned++
					}

					if treino.Concluido {
						completedRuns = append(completedRuns, domain.DiaFrequencia{
							Data:       treinoDate.Format("2006-01-02"),
							Realizado:  true,
							TipoTreino: treino.Tipo,
							TipoFicha:  "corrida",
							Observacao: treino.Descricao,
						})
					}
				}
			}
		}
	}

	var diasFrequencia []domain.DiaFrequencia
	diasComDor := 0

	for _, tr := range treinosList {
		tipoTreino := "Musculação"
		if tr.TipoTreino != nil {
			tipoTreino = *tr.TipoTreino
		}
		obs := ""
		if tr.Observacao != nil {
			obs = *tr.Observacao
			if strings.Contains(strings.ToLower(obs), "dor") || strings.Contains(strings.ToLower(obs), "lesão") || strings.Contains(strings.ToLower(obs), "desconforto") {
				diasComDor++
			}
		}
		diasFrequencia = append(diasFrequencia, domain.DiaFrequencia{
			Data:       tr.DataTreino,
			Realizado:  true,
			TipoTreino: tipoTreino,
			TipoFicha:  "musculacao",
			Observacao: obs,
		})
	}

	for _, run := range completedRuns {
		if strings.Contains(strings.ToLower(run.Observacao), "dor") || strings.Contains(strings.ToLower(run.Observacao), "lesão") || strings.Contains(strings.ToLower(run.Observacao), "desconforto") {
			diasComDor++
		}
		diasFrequencia = append(diasFrequencia, run)
	}

	// Estimativa de treinos planejados no mês.
	// Musculação: frequência da ficha ativa ou padrão 12/mês.
	musculacaoPlanned := 12
	fichasLinks, err := h.fichaWebRepo.ListByAlunoID(r.Context(), alunoID, false)
	if err == nil && len(fichasLinks) > 0 {
		var content map[string]any
		if err := json.Unmarshal([]byte(fichasLinks[0].ConteudoJSON), &content); err == nil {
			if freqVal, ok := content["frequencia_semanal"].(float64); ok {
				musculacaoPlanned = int(freqVal) * 4
			}
		}
	}

	totalPlanejados := musculacaoPlanned + activeRunningPlanned
	totalRealizados := len(diasFrequencia)

	taxaCompletude := 0.0
	if totalPlanejados > 0 {
		taxaCompletude = (float64(totalRealizados) / float64(totalPlanejados)) * 100
		if taxaCompletude > 100.0 {
			taxaCompletude = 100.0 // teto 100%
		}
	}

	resp := domain.FrequenciaMensalResponse{
		AlunoID: alunoID,
		Mes:     mes,
		Ano:     ano,
		EstatisticasMensais: domain.FrequenciaEstatisticas{
			TotalRealizados: totalRealizados,
			TotalPlanejados: totalPlanejados,
			TaxaCompletude:  taxaCompletude,
			DiasComDor:      diasComDor,
		},
		DiasFrequencia: diasFrequencia,
	}

	if resp.DiasFrequencia == nil {
		resp.DiasFrequencia = []domain.DiaFrequencia{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *HistoricoHandler) GetTreinos(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	alunoID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || alunoID <= 0 {
		writeJSONError(w, "ID do aluno inválido", http.StatusBadRequest)
		return
	}

	_, err = h.alunoRepo.GetByID(r.Context(), alunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	mesStr := r.URL.Query().Get("mes")
	mes := 0
	if mesStr != "" {
		if val, err := strconv.Atoi(mesStr); err == nil && val >= 1 && val <= 12 {
			mes = val
		}
	}

	anoStr := r.URL.Query().Get("ano")
	ano := 0
	if anoStr != "" {
		if val, err := strconv.Atoi(anoStr); err == nil && val > 1900 && val < 2100 {
			ano = val
		}
	}

	treinos, err := h.repo.GetTreinosRealizadosByAluno(r.Context(), alunoID, mes, ano)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if treinos == nil {
		treinos = []*domain.TreinoRealizado{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"aluno_id": alunoID,
		"treinos":  treinos,
	})
}

type MarkTreinoRequest struct {
	FichaID    int64   `json:"ficha_id"`
	HashFicha  string  `json:"hash_ficha,omitempty"`
	AlunoID    int64   `json:"aluno_id,omitempty"`
	DataTreino string  `json:"data_treino"` // YYYY-MM-DD
	TipoTreino string  `json:"tipo_treino,omitempty"`
	TipoFicha  string  `json:"tipo_ficha"`   // obrigatório: musculacao
	Observacao string  `json:"observacao,omitempty"`
}

func (h *HistoricoHandler) MarkTreino(w http.ResponseWriter, r *http.Request) {
	var req MarkTreinoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	req.TipoFicha = strings.TrimSpace(strings.ToLower(req.TipoFicha))
	if req.TipoFicha == "corrida" {
		writeJSONError(w, "Treinos de corrida devem ser marcados através do fluxo próprio de periodização", http.StatusBadRequest)
		return
	}
	if req.TipoFicha != "musculacao" {
		writeJSONError(w, "Tipo de ficha inválido. Deve ser 'musculacao'", http.StatusBadRequest)
		return
	}

	if req.FichaID <= 0 {
		writeJSONError(w, "Ficha ID é obrigatório", http.StatusBadRequest)
		return
	}

	req.DataTreino = strings.TrimSpace(req.DataTreino)
	if req.DataTreino == "" {
		writeJSONError(w, "Data do treino é obrigatória", http.StatusBadRequest)
		return
	}
	if _, err := time.Parse("2006-01-02", req.DataTreino); err != nil {
		writeJSONError(w, "Data do treino inválida. Deve estar no formato YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	var resolvedAlunoID int64
	var resolvedHashFicha *string

	// Resolve aluno_id: autenticado, via hash da ficha, ou por nome.
	user, authenticated := UserFromContext(r.Context())
	if authenticated {
		if req.AlunoID > 0 {
			resolvedAlunoID = req.AlunoID
		} else {
			studentName, err := h.repo.GetFichaTreinoAlunoNomeByID(r.Context(), req.FichaID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeJSONError(w, "Ficha de musculação não encontrada", http.StatusNotFound)
					return
				}
				writeJSONError(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			resolvedAlunoID, err = h.repo.GetAlunoIDByNome(r.Context(), studentName)
			if err != nil {
				resolvedAlunoID = 0
			}
		}
		if req.HashFicha != "" {
			val := req.HashFicha
			resolvedHashFicha = &val
		}
	} else {
		// Anônimo: hash_ficha é obrigatório.
		req.HashFicha = strings.TrimSpace(req.HashFicha)
		if req.HashFicha == "" {
			writeJSONError(w, "Hash da ficha é obrigatório para marcação anônima", http.StatusUnauthorized)
			return
		}

		fw, err := h.fichaWebRepo.GetByHash(r.Context(), req.HashFicha)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, "Link público de ficha inválido ou inexistente", http.StatusNotFound)
				return
			}
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if !fw.Ativo || fw.ExpiraEm.Before(time.Now()) {
			writeJSONError(w, "Este link público de ficha expirou ou está desativado", http.StatusForbidden)
			return
		}

		if fw.FichaID != req.FichaID {
			writeJSONError(w, "Ficha ID incorreto para o hash fornecido", http.StatusBadRequest)
			return
		}

		resolvedAlunoID = fw.AlunoID
		val := fw.Hash
		resolvedHashFicha = &val
	}

	// Se letra omitida, sugere a próxima na sequência A→B→C→D→A.
	tipoTreino := strings.TrimSpace(req.TipoTreino)
	if tipoTreino == "" {
		sessions, err := h.repo.GetTreinosRealizadosByAluno(r.Context(), resolvedAlunoID, 0, 0)
		if err == nil {
			var lastLetter string
			for _, s := range sessions {
				if s.FichaID == req.FichaID && s.TipoTreino != nil && *s.TipoTreino != "" {
					lastLetter = strings.ToUpper(strings.TrimSpace(*s.TipoTreino))
					break
				}
			}

			switch lastLetter {
			case "A":
				tipoTreino = "B"
			case "B":
				tipoTreino = "C"
			case "C":
				tipoTreino = "D"
			default:
				tipoTreino = "A"
			}
		} else {
			tipoTreino = "A"
		}
	}

	tr := &domain.TreinoRealizado{
		FichaID:    req.FichaID,
		AlunoID:    &resolvedAlunoID,
		HashFicha:  resolvedHashFicha,
		DataTreino: req.DataTreino,
		TipoTreino: &tipoTreino,
		TipoFicha:  "musculacao",
	}

	if req.Observacao != "" {
		val := req.Observacao
		tr.Observacao = &val
	}

	if authenticated {
		_ = user.ID
	}

	if err := h.repo.MarkTreinoRealizado(r.Context(), tr); err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": fmt.Sprintf("Treino %s marcado com sucesso para %s!", tipoTreino, req.DataTreino),
	})
}

type UnmarkTreinoRequest struct {
	FichaID    int64  `json:"ficha_id"`
	HashFicha  string `json:"hash_ficha,omitempty"`
	DataTreino string `json:"data_treino"` // YYYY-MM-DD
}

func (h *HistoricoHandler) UnmarkTreino(w http.ResponseWriter, r *http.Request) {
	var req UnmarkTreinoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if req.FichaID <= 0 {
		writeJSONError(w, "Ficha ID é obrigatório", http.StatusBadRequest)
		return
	}

	req.DataTreino = strings.TrimSpace(req.DataTreino)
	if req.DataTreino == "" {
		writeJSONError(w, "Data do treino é obrigatória", http.StatusBadRequest)
		return
	}

	_, authenticated := UserFromContext(r.Context())
	if !authenticated {
		// Anônimo: hash_ficha é obrigatório para desmarcar.
		req.HashFicha = strings.TrimSpace(req.HashFicha)
		if req.HashFicha == "" {
			writeJSONError(w, "Hash da ficha é obrigatório para desmarcação anônima", http.StatusUnauthorized)
			return
		}

		fw, err := h.fichaWebRepo.GetByHash(r.Context(), req.HashFicha)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, "Link público de ficha inválido ou inexistente", http.StatusNotFound)
				return
			}
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if !fw.Ativo || fw.ExpiraEm.Before(time.Now()) {
			writeJSONError(w, "Este link público de ficha expirou ou está desativado", http.StatusForbidden)
			return
		}

		if fw.FichaID != req.FichaID {
			writeJSONError(w, "Ficha ID incorreto para o hash fornecido", http.StatusBadRequest)
			return
		}
	}

	err := h.repo.UnmarkTreinoRealizado(r.Context(), req.FichaID, req.DataTreino)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Nenhuma marcação de treino encontrada para os parâmetros informados", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Marcação de treino removida com sucesso.",
	})
}

type HistoricoFichaResponse struct {
	domain.HistoricoFicha
	FichaJSON json.RawMessage `json:"ficha_json,omitempty"`
	PlanoJSON json.RawMessage `json:"plano_json,omitempty"`
}

func (h *HistoricoHandler) GetHistoricoDetalhes(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "ID de histórico inválido", http.StatusBadRequest)
		return
	}

	hf, err := h.repo.GetHistoricoFichaByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Ficha arquivada não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	resp := HistoricoFichaResponse{
		HistoricoFicha: *hf,
	}

	if hf.FichaJSON != nil && *hf.FichaJSON != "" {
		resp.FichaJSON = json.RawMessage(*hf.FichaJSON)
	}

	if hf.PlanoJSON != nil && *hf.PlanoJSON != "" {
		resp.PlanoJSON = json.RawMessage(*hf.PlanoJSON)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *HistoricoHandler) GetTreinosMes(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash_ficha")
	if hash == "" {
		hash = r.URL.Query().Get("hash")
	}
	if hash == "" {
		writeJSONError(w, "Parâmetro 'hash_ficha' é obrigatório", http.StatusBadRequest)
		return
	}

	fw, err := h.fichaWebRepo.GetByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Link público não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error retrieving link", http.StatusInternalServerError)
		return
	}

	if !fw.Ativo {
		writeJSONError(w, "Este link público está desativado", http.StatusGone)
		return
	}
	if fw.ExpiraEm.Before(time.Now()) {
		writeJSONError(w, "Este link público expirou", http.StatusGone)
		return
	}

	now := time.Now()
	mesStr := r.URL.Query().Get("mes")
	mes := int(now.Month())
	if mesStr != "" {
		if val, err := strconv.Atoi(mesStr); err == nil && val >= 1 && val <= 12 {
			mes = val
		} else {
			writeJSONError(w, "Mês inválido", http.StatusBadRequest)
			return
		}
	}

	anoStr := r.URL.Query().Get("ano")
	ano := now.Year()
	if anoStr != "" {
		if val, err := strconv.Atoi(anoStr); err == nil && val > 1900 && val < 2100 {
			ano = val
		} else {
			writeJSONError(w, "Ano inválido", http.StatusBadRequest)
			return
		}
	}

	treinosList, err := h.repo.GetTreinosRealizadosByAluno(r.Context(), fw.AlunoID, mes, ano)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	corridas, err := h.periodizacaoCorridaRepo.ListByAlunoID(r.Context(), fw.AlunoID)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var completedRuns []domain.DiaFrequencia
	var activeRunningPlanned = 0

	for _, pc := range corridas {
		if pc.PlanoJSON == "" {
			continue
		}
		var pd domain.PlanoDetalhado
		if err := json.Unmarshal([]byte(pc.PlanoJSON), &pd); err != nil {
			continue
		}
		startDate, err := time.Parse("2006-01-02", pc.DataInicio)
		if err != nil {
			continue
		}
		segundaInicio := mondayOfWeek(startDate)

		for _, semana := range pd.Semanas {
			semanaStart := segundaInicio.AddDate(0, 0, (semana.Numero-1)*7)
			for _, t := range semana.Treinos {
				tDate := semanaStart.AddDate(0, 0, t.Dia-1)
				tYear, tMonth, _ := tDate.Date()
				if tYear == ano && int(tMonth) == mes {
					if pc.Status == "ativo" {
						activeRunningPlanned++
					}
					if t.Concluido {
						completedRuns = append(completedRuns, domain.DiaFrequencia{
							Data:       tDate.Format("2006-01-02"),
							Realizado:  true,
							TipoTreino: t.Tipo,
							TipoFicha:  "corrida",
							Observacao: t.Descricao,
						})
					}
				}
			}
		}
	}

	var diasFrequencia []domain.DiaFrequencia
	diasComDor := 0

	for _, tr := range treinosList {
		tipo := "Musculação"
		if tr.TipoTreino != nil {
			tipo = *tr.TipoTreino
		}
		obs := ""
		if tr.Observacao != nil {
			obs = *tr.Observacao
			if strings.Contains(strings.ToLower(obs), "dor") || strings.Contains(strings.ToLower(obs), "lesão") || strings.Contains(strings.ToLower(obs), "desconforto") {
				diasComDor++
			}
		}
		diasFrequencia = append(diasFrequencia, domain.DiaFrequencia{
			Data:       tr.DataTreino,
			Realizado:  true,
			TipoTreino: tipo,
			TipoFicha:  "musculacao",
			Observacao: obs,
		})
	}

	for _, run := range completedRuns {
		if strings.Contains(strings.ToLower(run.Observacao), "dor") || strings.Contains(strings.ToLower(run.Observacao), "lesão") || strings.Contains(strings.ToLower(run.Observacao), "desconforto") {
			diasComDor++
		}
		diasFrequencia = append(diasFrequencia, run)
	}

	musculacaoPlanned := 12
	var content map[string]any
	if err := json.Unmarshal([]byte(fw.ConteudoJSON), &content); err == nil {
		if freqVal, ok := content["frequencia_semanal"].(float64); ok {
			musculacaoPlanned = int(freqVal) * 4
		}
	}

	totalPlanejados := musculacaoPlanned + activeRunningPlanned
	totalRealizados := len(diasFrequencia)
	taxaCompletude := 0.0
	if totalPlanejados > 0 {
		taxaCompletude = (float64(totalRealizados) / float64(totalPlanejados)) * 100
		if taxaCompletude > 100.0 {
			taxaCompletude = 100.0
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"mes":              mes,
		"ano":              ano,
		"total_realizados": totalRealizados,
		"total_planejados": totalPlanejados,
		"taxa_completude":  taxaCompletude,
		"dias_com_dor":     diasComDor,
		"frequencia":       diasFrequencia,
	})
}

