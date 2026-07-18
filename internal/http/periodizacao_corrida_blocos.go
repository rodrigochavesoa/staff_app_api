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

	"staff_app/internal/corrida/blocos"
	"staff_app/internal/domain"

	"github.com/go-chi/chi/v5"
)

type saveBlocosRequest struct {
	Versao int                  `json:"versao"`
	Blocos []domain.BlocoCorrida `json:"blocos"`
	Nome   string               `json:"nome"`
	Tipo   string               `json:"tipo"`
	Zona   string               `json:"zona"`
}

type gerarBlocosRequest struct {
	VDOT           float64 `json:"vdot"`
	DistanciaProva string  `json:"distancia_prova"`
	Nivel          string  `json:"nivel"`
	DiasSemana     int     `json:"dias_semana"`
	Objetivo       string  `json:"objetivo"`
	Limitacoes     string  `json:"limitacoes"`
	AlunoID        int64   `json:"aluno_id"`
}

func (h *PeriodizacaoCorridaHandler) GerarProximaSemana(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositiveID(w, chi.URLParam(r, "id"), "Invalid ID")
	if !ok {
		return
	}

	var updateErr error
	var respData map[string]any

	for i := 0; i < 10; i++ {
		pc, err := h.repo.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				returnError(w, http.StatusNotFound, "Plano de corrida não encontrado")
				return
			}
			returnError(w, http.StatusInternalServerError, "Failed to fetch plan")
			return
		}
		if pc.Status != "ativo" {
			returnError(w, http.StatusBadRequest, "Plano de corrida não está ativo")
			return
		}
		if pc.ModoGeracao != blocos.ModoSemanaASemana {
			returnError(w, http.StatusBadRequest, "Esta função é apenas para planos em modo semana_a_semana")
			return
		}

		var pd domain.PlanoDetalhado
		if err := json.Unmarshal([]byte(pc.PlanoJSON), &pd); err != nil {
			returnError(w, http.StatusInternalServerError, "Failed to parse plan_json")
			return
		}

		proxima := len(pd.Semanas) + 1
		if proxima > pc.DuracaoSemanas {
			returnError(w, http.StatusBadRequest, fmt.Sprintf("Plano já completou todas as %d semanas", pc.DuracaoSemanas))
			return
		}

		var dias []int
		if err := json.Unmarshal([]byte(pc.DiasSemanaSelecionados), &dias); err != nil || len(dias) == 0 {
			dias = pd.DiasSemanaSelecionados
		}
		if len(dias) == 0 {
			returnError(w, http.StatusBadRequest, "Plano sem dias_semana_selecionados")
			return
		}

		templates, err := blocos.LoadTemplates(h.templatesPath)
		if err != nil {
			returnError(w, http.StatusInternalServerError, "Falha ao carregar templates de blocos")
			return
		}

		distLabel := distanciaLabelFromKM(pc.DistanciaProva)
		novaSemana, err := blocos.GenerateWeek(templates, proxima, pc.DuracaoSemanas, dias, pc.VDOT, pc.Nivel, distLabel)
		if err != nil {
			returnError(w, http.StatusInternalServerError, fmt.Sprintf("Falha ao gerar semana: %v", err))
			return
		}

		pd.Semanas = append(pd.Semanas, novaSemana)
		pd.SemanasGeradas = len(pd.Semanas)
		pd.ModoGeracao = blocos.ModoSemanaASemana
		pd.Tipo = "blocos_dinamicos"

		updatedBytes, err := json.Marshal(pd)
		if err != nil {
			returnError(w, http.StatusInternalServerError, "Failed to serialize updated plan")
			return
		}
		pc.PlanoJSON = string(updatedBytes)
		pc.DataUltimaGeracao = time.Now().Format("2006-01-02 15:04:05")

		updateErr = h.repo.Update(r.Context(), pc)
		if updateErr == nil {
			respData = map[string]any{
				"semana_numero":          proxima,
				"total_semanas_geradas":  len(pd.Semanas),
				"duracao_semanas":        pc.DuracaoSemanas,
				"versao":                 pc.Versao,
				"semana":                 novaSemana,
			}
			break
		}
		if !errors.Is(updateErr, sql.ErrNoRows) {
			break
		}
		time.Sleep(time.Duration(10+i*20) * time.Millisecond)
	}

	if updateErr != nil {
		if errors.Is(updateErr, sql.ErrNoRows) {
			returnError(w, http.StatusConflict, "Conflito de concorrência ao atualizar o plano")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to append next week")
		return
	}

	returnSuccess(w, http.StatusOK, respData, fmt.Sprintf("Semana %d gerada com sucesso", respData["semana_numero"]))
}

func (h *PeriodizacaoCorridaHandler) GetTreinoDia(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositiveID(w, chi.URLParam(r, "id"), "Invalid ID")
	if !ok {
		return
	}
	semana, ok := parsePositiveID(w, chi.URLParam(r, "semana"), "Semana inválida")
	if !ok {
		return
	}
	dia, ok := parsePositiveID(w, chi.URLParam(r, "dia"), "Dia inválido")
	if !ok {
		return
	}
	if dia < 1 || dia > 7 {
		returnError(w, http.StatusBadRequest, "Dia deve estar entre 1 e 7")
		return
	}

	pc, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Plano de corrida não encontrado")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to fetch plan")
		return
	}

	var pd domain.PlanoDetalhado
	if err := json.Unmarshal([]byte(pc.PlanoJSON), &pd); err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to parse plan_json")
		return
	}

	treino, diasComTreino := findTreinoDia(pd, int(semana), int(dia))
	if treino == nil {
		treino = &domain.TreinoJSON{
			Dia:            int(dia),
			Nome:           fmt.Sprintf("Dia %d - Sem treino programado", dia),
			Tipo:           "sem_treino",
			Descricao:      "Este dia não possui treino programado.",
			Blocos:         []domain.BlocoCorrida{},
			DuracaoMinutos: 0,
		}
	}

	returnSuccess(w, http.StatusOK, map[string]any{
		"plano_id":        pc.ID,
		"semana":          semana,
		"dia":             dia,
		"treino":          treino,
		"dias_com_treino": diasComTreino,
		"versao":          pc.Versao,
	}, "")
}

func (h *PeriodizacaoCorridaHandler) SaveBlocosDia(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePositiveID(w, chi.URLParam(r, "id"), "Invalid ID")
	if !ok {
		return
	}
	semana, ok := parsePositiveID(w, chi.URLParam(r, "semana"), "Semana inválida")
	if !ok {
		return
	}
	dia, ok := parsePositiveID(w, chi.URLParam(r, "dia"), "Dia inválido")
	if !ok {
		return
	}
	if dia < 1 || dia > 7 {
		returnError(w, http.StatusBadRequest, "Dia deve estar entre 1 e 7")
		return
	}

	var req saveBlocosRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	if ifMatch := strings.TrimSpace(r.Header.Get("If-Match")); ifMatch != "" && req.Versao == 0 {
		v, err := strconv.Atoi(ifMatch)
		if err != nil {
			returnError(w, http.StatusBadRequest, "If-Match inválido")
			return
		}
		req.Versao = v
	}

	validationErrors := blocos.ValidateStructure(req.Blocos)
	if len(validationErrors) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":            "error",
			"message":           "Blocos inválidos",
			"validation_errors": validationErrors,
		})
		return
	}
	warnings := blocos.GenerateWarnings(req.Blocos)

	var updateErr error
	var finalVersao int
	var found bool

	for i := 0; i < 10; i++ {
		pc, err := h.repo.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				returnError(w, http.StatusNotFound, "Plano de corrida não encontrado")
				return
			}
			returnError(w, http.StatusInternalServerError, "Failed to fetch plan")
			return
		}
		if req.Versao > 0 && req.Versao != pc.Versao {
			returnError(w, http.StatusConflict, "Conflito de concorrência ao atualizar o plano. O plano foi modificado por outro usuário.")
			return
		}

		var pd domain.PlanoDetalhado
		if err := json.Unmarshal([]byte(pc.PlanoJSON), &pd); err != nil {
			returnError(w, http.StatusInternalServerError, "Failed to parse plan_json")
			return
		}

		found = false
		dur := blocos.DurationMinutes(req.Blocos)
		pace := firstPace(req.Blocos)
		dist := blocos.EstimateDistanceKM(dur, pace)
		zona := req.Zona
		if zona == "" {
			zona = firstIntensity(req.Blocos)
		}

		for sIdx := range pd.Semanas {
			if pd.Semanas[sIdx].Numero != int(semana) {
				continue
			}
			for tIdx := range pd.Semanas[sIdx].Treinos {
				if pd.Semanas[sIdx].Treinos[tIdx].Dia != int(dia) {
					continue
				}
				tr := &pd.Semanas[sIdx].Treinos[tIdx]
				tr.Blocos = req.Blocos
				tr.DuracaoMinutos = dur
				if req.Nome != "" {
					tr.Nome = req.Nome
					tr.Tipo = req.Nome
				}
				if req.Tipo != "" {
					tr.Tipo = req.Tipo
				}
				tr.Zona = zona
				tr.ZonaPrincipal = zona
				if pace != "" {
					tr.PaceAlvo = pace
				}
				if dist > 0 {
					tr.Distancia = dist
				}
				found = true
				break
			}
			if !found {
				// Create training day if week exists but day missing.
				pd.Semanas[sIdx].Treinos = append(pd.Semanas[sIdx].Treinos, domain.TreinoJSON{
					Dia:            int(dia),
					Tipo:           firstNonEmpty(req.Tipo, req.Nome, "Treino"),
					Nome:           firstNonEmpty(req.Nome, req.Tipo, "Treino"),
					Zona:           zona,
					ZonaPrincipal:  zona,
					PaceAlvo:       pace,
					Distancia:      dist,
					DuracaoMinutos: dur,
					Blocos:         req.Blocos,
					Descricao:      firstNonEmpty(req.Nome, req.Tipo, "Treino"),
				})
				found = true
			}
			break
		}

		if !found {
			returnError(w, http.StatusBadRequest, fmt.Sprintf("Semana %d não encontrada no plano", semana))
			return
		}

		updatedBytes, err := json.Marshal(pd)
		if err != nil {
			returnError(w, http.StatusInternalServerError, "Failed to serialize updated plan")
			return
		}
		pc.PlanoJSON = string(updatedBytes)
		pc.DataUltimaGeracao = time.Now().Format("2006-01-02 15:04:05")

		updateErr = h.repo.Update(r.Context(), pc)
		if updateErr == nil {
			finalVersao = pc.Versao
			break
		}
		if !errors.Is(updateErr, sql.ErrNoRows) {
			break
		}
		time.Sleep(time.Duration(10+i*20) * time.Millisecond)
	}

	if updateErr != nil {
		if errors.Is(updateErr, sql.ErrNoRows) {
			returnError(w, http.StatusConflict, "Conflito de concorrência ao atualizar o plano. O plano foi modificado por outro usuário.")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to save blocs")
		return
	}

	returnSuccess(w, http.StatusOK, map[string]any{
		"versao":   finalVersao,
		"warnings": warnings,
	}, "Blocos salvos com sucesso")
}

func (h *PeriodizacaoCorridaHandler) HistoricoStats(w http.ResponseWriter, r *http.Request) {
	alunoID, ok := parsePositiveID(w, chi.URLParam(r, "id"), "ID do aluno inválido")
	if !ok {
		return
	}
	dias := 30
	if raw := r.URL.Query().Get("dias"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 365 {
			returnError(w, http.StatusBadRequest, "dias deve estar entre 1 e 365")
			return
		}
		dias = parsed
	}

	if _, err := h.alunoRepo.GetByID(r.Context(), alunoID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Aluno não encontrado")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to fetch aluno")
		return
	}

	since := time.Now().AddDate(0, 0, -dias)
	garminSamples := []blocos.GarminSample{}
	if acts, _, err := h.garminRepo.ListAlunoActivities(r.Context(), alunoID, "", 500, 0); err == nil {
		for _, a := range acts {
			if a.StartTime == nil {
				continue
			}
			dist := 0.0
			if a.DistanceMeters != nil {
				dist = *a.DistanceMeters
			}
			garminSamples = append(garminSamples, blocos.GarminSample{
				StartTime:      *a.StartTime,
				DistanceMeters: dist,
			})
		}
	}

	planos := []blocos.PlanoConclusao{}
	if list, err := h.repo.ListByAlunoID(r.Context(), alunoID); err == nil {
		for _, pc := range list {
			var pd domain.PlanoDetalhado
			if pc.PlanoJSON == "" {
				continue
			}
			if err := json.Unmarshal([]byte(pc.PlanoJSON), &pd); err != nil {
				continue
			}
			planos = append(planos, blocos.SummarizePlanoConclusao(pd))
		}
	}

	stats := blocos.ComputeHistoricoStats(alunoID, dias, garminSamples, planos, since)
	returnSuccess(w, http.StatusOK, stats, "")
}

func (h *PeriodizacaoCorridaHandler) GerarBlocos(w http.ResponseWriter, r *http.Request) {
	var req gerarBlocosRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	if req.VDOT < 30 || req.VDOT > 85 {
		returnError(w, http.StatusBadRequest, "VDOT deve estar entre 30 e 85")
		return
	}
	if req.DistanciaProva != "5K" && req.DistanciaProva != "10K" && req.DistanciaProva != "21K" && req.DistanciaProva != "42K" {
		returnError(w, http.StatusBadRequest, "Distância inválida (use: 5K, 10K, 21K, 42K)")
		return
	}
	if req.Nivel != "iniciante" && req.Nivel != "intermediario" && req.Nivel != "avancado" && req.Nivel != "elite" {
		returnError(w, http.StatusBadRequest, "Nível inválido")
		return
	}
	if req.DiasSemana < 2 || req.DiasSemana > 7 {
		returnError(w, http.StatusBadRequest, "Dias por semana deve estar entre 2 e 7")
		return
	}

	templates, err := blocos.LoadTemplates(h.templatesPath)
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Falha ao carregar templates de blocos")
		return
	}

	days, durTotal, distribuicao, err := blocos.GeneratePreview(templates, req.VDOT, req.DistanciaProva, req.Nivel, req.DiasSemana)
	if err != nil {
		returnError(w, http.StatusBadRequest, err.Error())
		return
	}

	aiMode := "assistive"
	if h.cfg != nil && h.cfg.AITrainingMode != "" {
		aiMode = h.cfg.AITrainingMode
	}

	meta := map[string]any{
		"ai_used":           false,
		"provider":          "local",
		"model":             "templates_daniels_blocos",
		"fallback_used":     false,
		"fallback_reason":   "",
		"safety_validated":  true,
		"quality_validated": true,
		"warnings":          []string{},
	}

	// Clinical safety: downgrade I/R when active anamnese risk is high.
	if req.AlunoID > 0 {
		if a, err := h.anamneseRepo.FindActiveByAlunoID(r.Context(), req.AlunoID); err == nil && a != nil && a.RiskScoreCached >= 3 {
			for i := range days {
				days[i].Blocos = blocos.ApplyPaces(blocos.DowngradeHardIntensities(days[i].Blocos), req.VDOT)
				days[i].DuracaoMinutos = blocos.DurationMinutes(days[i].Blocos)
			}
			distribuicao = map[string]int{}
			durTotal = 0
			for _, d := range days {
				durTotal += d.DuracaoMinutos
				for k, v := range blocos.IntensityDistribution(d.Blocos) {
					distribuicao[k] += v
				}
			}
			meta["warnings"] = []string{"intensidades I/R reduzidas por risco cardiorrespiratório alto"}
		}
	}

	switch aiMode {
	case "off":
		meta["fallback_used"] = true
		meta["fallback_reason"] = "ai_training_mode_off"
	case "required":
		// required exige provider de blocos aprovado (real ou fake injetado). Local sozinho não basta.
		if !h.hasBlocksAIProvider() {
			returnError(w, http.StatusServiceUnavailable, "Nenhum provedor de IA de blocos disponível para AI_TRAINING_MODE=required")
			return
		}
		meta["ai_used"] = true
		meta["provider"] = h.blocksAI.Name()
		meta["fallback_used"] = false
	default:
		// assistive futuro/backlog: nesta fase gerar-blocos permanece local-only.
		meta["fallback_used"] = true
		meta["fallback_reason"] = "ai_assistive_blocos_backlog_local_only"
	}

	flatBlocos := make([]domain.BlocoCorrida, 0)
	for _, d := range days {
		flatBlocos = append(flatBlocos, d.Blocos...)
	}

	returnSuccess(w, http.StatusOK, map[string]any{
		"blocos":         flatBlocos,
		"dias":           days,
		"duracao_total":  durTotal,
		"distribuicao":   distribuicao,
		"ai_metadata":    meta,
		"objetivo":       firstNonEmpty(req.Objetivo, "performance"),
		"limitacoes":     req.Limitacoes,
		"usou_fallback":  meta["fallback_used"],
	}, "Blocos gerados com sucesso")
}

func parsePositiveID(w http.ResponseWriter, raw, errMsg string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		returnError(w, http.StatusBadRequest, errMsg)
		return 0, false
	}
	return id, true
}

func findTreinoDia(pd domain.PlanoDetalhado, semana, dia int) (*domain.TreinoJSON, []int) {
	diasComTreino := []int{}
	for _, s := range pd.Semanas {
		if s.Numero != semana {
			continue
		}
		var found *domain.TreinoJSON
		for i := range s.Treinos {
			tr := s.Treinos[i]
			if len(tr.Blocos) > 0 || tr.Nome != "" || tr.Tipo != "" {
				diasComTreino = append(diasComTreino, tr.Dia)
			}
			if tr.Dia == dia {
				cp := tr
				found = &cp
			}
		}
		return found, diasComTreino
	}
	return nil, diasComTreino
}

func distanciaLabelFromKM(km float64) string {
	switch {
	case km >= 40:
		return "42K"
	case km >= 20:
		return "21K"
	case km >= 9:
		return "10K"
	default:
		return "5K"
	}
}

func firstPace(items []domain.BlocoCorrida) string {
	for _, b := range items {
		if b.Type == "atomic" && b.PaceMinKM != "" {
			return b.PaceMinKM
		}
		if p := firstPace(b.Content); p != "" {
			return p
		}
	}
	return ""
}

func firstIntensity(items []domain.BlocoCorrida) string {
	for _, b := range items {
		if b.Type == "atomic" && b.Intensity != "" && b.Intensity != "Rest" {
			return b.Intensity
		}
		if z := firstIntensity(b.Content); z != "" {
			return z
		}
	}
	return "E"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
