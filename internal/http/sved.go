package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"staff_app/internal/repositories"
	"staff_app/internal/services"

	"github.com/go-chi/chi/v5"
)

type SVEDHandler struct {
	repo repositories.SVEDRepository
}

func NewSVEDHandler(repo repositories.SVEDRepository) *SVEDHandler {
	return &SVEDHandler{repo: repo}
}

type CalcularSVEDRequest struct {
	Reps     int    `json:"reps"`
	Cadencia string `json:"cadencia"`
	Descanso any    `json:"descanso"` // string or int
	Rir      int    `json:"rir"`
	Series   int    `json:"series"`
}

func extrairExerciciosDeFichaJSON(fichaJSON string) []services.ExercicioJSON {
	var parsed struct {
		Exercicios []services.ExercicioJSON `json:"exercicios"`
		Treinos    []struct {
			Exercicios []services.ExercicioJSON `json:"exercicios"`
		} `json:"treinos"`
	}
	if err := json.Unmarshal([]byte(fichaJSON), &parsed); err != nil {
		return nil
	}

	var list []services.ExercicioJSON
	list = append(list, parsed.Exercicios...)
	for _, t := range parsed.Treinos {
		list = append(list, t.Exercicios...)
	}
	return list
}

func svedMetricsFromExercise(fichaID int64, dataCriacao string, ex services.ExercicioJSON) (services.SVEDHistoricoItem, []string) {
	warnings := make([]string, 0)
	if strings.TrimSpace(ex.Nome) == "" {
		warnings = append(warnings, "exercício sem nome")
	}
	if ex.Series == nil {
		warnings = append(warnings, "series ausente; usando default 1")
	}
	if ex.Repeticoes == nil {
		warnings = append(warnings, "repeticoes ausente; usando default 10")
	}
	if ex.Descanso == nil {
		warnings = append(warnings, "descanso ausente; usando default 60s")
	}
	if ex.RIR == nil {
		warnings = append(warnings, "rir ausente; usando default 2")
	}
	if strings.TrimSpace(ex.Cadencia) == "" {
		warnings = append(warnings, "cadencia ausente; usando default 4010")
	}

	series := services.ParseInt(ex.Series, 1)
	reps := services.ParseInt(ex.Repeticoes, 10)
	cadencia := ex.Cadencia
	if cadencia == "" {
		cadencia = "4010"
	}
	restSeconds := services.ParseDescanso(ex.Descanso)
	rir := services.ParseInt(ex.RIR, 2)

	tutTotalEx := reps * series * 4
	if len(cadencia) == 4 {
		cadVals := services.ParseCadencia(cadencia)
		tempoPorRep := cadVals["excentrica"] + cadVals["pausa"] + cadVals["concentrica"] + cadVals["pico"]
		tutTotalEx = reps * tempoPorRep * series
	}

	densidade := services.CalcularDensidade(float64(tutTotalEx), restSeconds)
	ies := services.CalcularIES(series, reps, cadencia, restSeconds, rir)

	return services.SVEDHistoricoItem{
		FichaID:     fichaID,
		Data:        dataCriacao,
		Series:      series,
		Reps:        reps,
		RIR:         rir,
		Cadencia:    cadencia,
		RestSeconds: restSeconds,
		TutTotal:    tutTotalEx,
		Densidade:   densidade,
		IesScore:    ies,
	}, warnings
}

func svedHistoryForExercise(exercicioNome string, sheets []repositories.SVEDSheet) []services.SVEDHistoricoItem {
	historico := make([]services.SVEDHistoricoItem, 0, len(sheets))
	needle := strings.ToLower(strings.TrimSpace(exercicioNome))
	if needle == "" {
		return historico
	}
	for _, sheet := range sheets {
		for _, ex := range extrairExerciciosDeFichaJSON(sheet.FichaJSON) {
			if strings.Contains(strings.ToLower(ex.Nome), needle) {
				item, _ := svedMetricsFromExercise(sheet.ID, sheet.DataCriacao, ex)
				historico = append(historico, item)
				break
			}
		}
	}
	return historico
}

func (h *SVEDHandler) Calcular(w http.ResponseWriter, r *http.Request) {
	var req CalcularSVEDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	restSeconds := services.ParseDescanso(req.Descanso)
	tutPorRep := 4
	if len(req.Cadencia) == 4 {
		cad := services.ParseCadencia(req.Cadencia)
		tutPorRep = cad["excentrica"] + cad["pausa"] + cad["concentrica"] + cad["pico"]
	}
	tutTotal := req.Reps * tutPorRep * req.Series
	densidade := services.CalcularDensidade(float64(tutTotal), restSeconds)
	ies := services.CalcularIES(req.Series, req.Reps, req.Cadencia, restSeconds, req.Rir)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"tut_total": tutTotal,
		"densidade": densidade,
		"ies":       ies,
	})
}

func (h *SVEDHandler) GetHistorico(w http.ResponseWriter, r *http.Request) {
	alunoIDStr := chi.URLParam(r, "aluno_id")
	alunoID, err := strconv.ParseInt(alunoIDStr, 10, 64)
	if err != nil || alunoID <= 0 {
		writeJSONError(w, "Invalid aluno ID", http.StatusBadRequest)
		return
	}

	exercicioNome := chi.URLParam(r, "exercicio_nome")
	if exercicioNome == "" {
		writeJSONError(w, "exercicio_nome parameter is required", http.StatusBadRequest)
		return
	}

	alunoNome, err := h.repo.GetAlunoNomeByID(r.Context(), alunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	sheets, err := h.repo.ListFichaSheetsByAlunoNome(r.Context(), alunoNome, 20)
	if err != nil {
		writeJSONError(w, "Failed to query history", http.StatusInternalServerError)
		return
	}

	historico := svedHistoryForExercise(exercicioNome, sheets)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"aluno_nome": alunoNome,
		"exercicio":  exercicioNome,
		"historico":  historico,
	})
}

func (h *SVEDHandler) GetSugestao(w http.ResponseWriter, r *http.Request) {
	fichaIDStr := chi.URLParam(r, "ficha_id")
	fichaID, err := strconv.ParseInt(fichaIDStr, 10, 64)
	if err != nil || fichaID <= 0 {
		writeJSONError(w, "Invalid ficha ID", http.StatusBadRequest)
		return
	}

	exercicioNome := chi.URLParam(r, "exercicio_nome")
	if exercicioNome == "" {
		writeJSONError(w, "exercicio_nome parameter is required", http.StatusBadRequest)
		return
	}

	studentName, err := h.repo.GetFichaAlunoByID(r.Context(), fichaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Ficha não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	sheets, err := h.repo.ListFichaSheetsByAlunoNome(r.Context(), studentName, 5)
	if err != nil {
		writeJSONError(w, "Failed to query history", http.StatusInternalServerError)
		return
	}

	historico := svedHistoryForExercise(exercicioNome, sheets)
	sugestao := services.SugerirProgressaoSVED(exercicioNome, historico)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":         true,
		"sugestao":        sugestao,
		"historico_count": len(historico),
	})
}

func (h *SVEDHandler) GetSugestoesFicha(w http.ResponseWriter, r *http.Request) {
	fichaIDStr := chi.URLParam(r, "ficha_id")
	fichaID, err := strconv.ParseInt(fichaIDStr, 10, 64)
	if err != nil || fichaID <= 0 {
		writeJSONError(w, "Invalid ficha ID", http.StatusBadRequest)
		return
	}

	detail, err := h.repo.GetFichaDetailByID(r.Context(), fichaID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Ficha não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	alunoID, alunoIDOK, err := h.repo.GetAlunoIDByNomeLatest(r.Context(), detail.AlunoNome)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	exercicios := extrairExerciciosDeFichaJSON(detail.FichaJSON)
	responseWarnings := make([]string, 0)
	if len(exercicios) == 0 {
		responseWarnings = append(responseWarnings, "ficha sem exercícios parseáveis para SVED")
	}

	sheets, err := h.repo.ListFichaSheetsByAlunoNome(r.Context(), detail.AlunoNome, 20)
	if err != nil {
		writeJSONError(w, "Failed to query history", http.StatusInternalServerError)
		return
	}

	sugestoes := make([]map[string]any, 0, len(exercicios))
	for _, ex := range exercicios {
		if strings.TrimSpace(ex.Nome) == "" {
			responseWarnings = append(responseWarnings, "exercício sem nome ignorado")
			continue
		}
		current, warnings := svedMetricsFromExercise(fichaID, "", ex)
		historico := svedHistoryForExercise(ex.Nome, sheets)
		sugestao := services.SugerirProgressaoSVED(ex.Nome, historico)

		sugestoes = append(sugestoes, map[string]any{
			"exercicio":       ex.Nome,
			"grupo_muscular":  ex.GrupoMuscular,
			"series":          current.Series,
			"reps":            current.Reps,
			"rir":             current.RIR,
			"cadencia":        current.Cadencia,
			"rest_seconds":    current.RestSeconds,
			"tut_total":       current.TutTotal,
			"densidade":       current.Densidade,
			"ies_score":       current.IesScore,
			"historico_count": len(historico),
			"sugestao":        sugestao,
			"warnings":        warnings,
		})
	}

	resp := map[string]any{
		"success":          true,
		"ficha_id":         fichaID,
		"aluno":            detail.AlunoNome,
		"titulo":           detail.Titulo,
		"total_exercicios": len(sugestoes),
		"sugestoes":        sugestoes,
		"warnings":         responseWarnings,
	}
	if alunoIDOK {
		resp["aluno_id"] = alunoID
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type ExerciseDashboardStats struct {
	DensidadeSoma float64
	IesSoma       float64
	Count         int
}

func (h *SVEDHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	alunoIDStr := chi.URLParam(r, "aluno_id")
	alunoID, err := strconv.ParseInt(alunoIDStr, 10, 64)
	if err != nil || alunoID <= 0 {
		writeJSONError(w, "Invalid aluno ID", http.StatusBadRequest)
		return
	}

	alunoNome, err := h.repo.GetAlunoNomeByID(r.Context(), alunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	agg, err := h.repo.GetAggregatedStatsByAluno(r.Context(), alunoNome)
	if err != nil {
		writeJSONError(w, "Failed to query stats gerais", http.StatusInternalServerError)
		return
	}

	statsGerais := map[string]float64{
		"ies_medio_geral":       math.Round(agg.IesMedio*10) / 10,
		"densidade_media_geral": math.Round(agg.DensidadeMedia*100) / 100,
		"volume_efetivo_geral":  math.Round(agg.VolumeEfetivo*10) / 10,
	}

	sheets, err := h.repo.ListDashboardSheetsByAluno(r.Context(), alunoNome, 20)
	if err != nil {
		writeJSONError(w, "Failed to query sheets history", http.StatusInternalServerError)
		return
	}

	var evolucaoTemporal []map[string]any
	var historicoSved []map[string]any
	exerciseGrouping := make(map[string]*ExerciseDashboardStats)

	for _, sheet := range sheets {
		exercicios := extrairExerciciosDeFichaJSON(sheet.FichaJSON)
		totalEx := 1
		if len(exercicios) > 0 {
			totalEx = len(exercicios)
		}

		for _, ex := range exercicios {
			bloco := "principal"
			if ex.Bloco != "" {
				bloco = ex.Bloco
			}
			if bloco != "principal" {
				continue
			}

			series := services.ParseInt(ex.Series, 1)
			reps := services.ParseInt(ex.Repeticoes, 10)
			cadencia := ex.Cadencia
			if cadencia == "" {
				cadencia = "4010"
			}
			restSeconds := services.ParseDescanso(ex.Descanso)
			rir := services.ParseInt(ex.RIR, 2)

			var tutTotalEx int
			if len(cadencia) == 4 {
				cadVals := services.ParseCadencia(cadencia)
				tempoPorRep := cadVals["excentrica"] + cadVals["pausa"] + cadVals["concentrica"] + cadVals["pico"]
				tutTotalEx = reps * tempoPorRep * series
			} else {
				tutTotalEx = reps * series * 4
			}

			dens := services.CalcularDensidade(float64(tutTotalEx), restSeconds)
			iesVal := services.CalcularIES(series, reps, cadencia, restSeconds, rir)

			exNameClean := strings.TrimSpace(ex.Nome)
			if exNameClean == "" {
				continue
			}

			if _, exists := exerciseGrouping[exNameClean]; !exists {
				exerciseGrouping[exNameClean] = &ExerciseDashboardStats{}
			}
			exerciseGrouping[exNameClean].DensidadeSoma += dens
			exerciseGrouping[exNameClean].IesSoma += iesVal
			exerciseGrouping[exNameClean].Count++
		}

		evolucaoTemporal = append(evolucaoTemporal, map[string]any{
			"ficha_id":         sheet.ID,
			"titulo":           sheet.Turma,
			"data":             sheet.DataCriacao,
			"ies_medio":        sheet.IesScore,
			"tut_medio":        float64(sheet.TutTotal),
			"densidade_media":  sheet.Densidade,
			"volume_efetivo":   float64(sheet.VolumeSved),
			"total_exercicios": totalEx,
		})

		historicoSved = append(historicoSved, map[string]any{
			"ficha_id":  sheet.ID,
			"data":      sheet.DataCriacao,
			"ies_score": sheet.IesScore,
			"densidade": sheet.Densidade,
		})
	}

	var densidadePorExercicio []map[string]any
	for name, st := range exerciseGrouping {
		if st.Count > 0 {
			densidadePorExercicio = append(densidadePorExercicio, map[string]any{
				"nome":            name,
				"densidade_media": math.Round((st.DensidadeSoma/float64(st.Count))*100) / 100,
				"ies_medio":       math.Round((st.IesSoma/float64(st.Count))*10) / 10,
			})
		}
	}

	sort.Slice(densidadePorExercicio, func(i, j int) bool {
		return densidadePorExercicio[i]["densidade_media"].(float64) > densidadePorExercicio[j]["densidade_media"].(float64)
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":                 true,
		"stats_gerais":            statsGerais,
		"evolucao_temporal":       evolucaoTemporal,
		"densidade_por_exercicio": densidadePorExercicio,
		"historico_sved":          historicoSved,
	})
}
