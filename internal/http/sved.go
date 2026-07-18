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

	"staff_app/internal/services"
	"staff_app/internal/sqlite"

	"github.com/go-chi/chi/v5"
)

type SVEDHandler struct {
	db *sqlite.DB
}

func NewSVEDHandler(db *sqlite.DB) *SVEDHandler {
	return &SVEDHandler{db: db}
}

type CalcularSVEDRequest struct {
	Reps     int    `json:"reps"`
	Cadencia string `json:"cadencia"`
	Descanso any    `json:"descanso"` // string or int
	Rir      int    `json:"rir"`
	Series   int    `json:"series"`
}

type svedSheet struct {
	ID          int64
	DataCriacao string
	FichaJSON   string
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

func svedHistoryForExercise(exercicioNome string, sheets []svedSheet) []services.SVEDHistoricoItem {
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

	var alunoNome string
	err = h.db.QueryRowContext(r.Context(), "SELECT nome FROM alunos WHERE id = ?", alunoID).Scan(&alunoNome)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, data_criacao, ficha_json 
		FROM fichas_treino_web 
		WHERE aluno = ? 
		ORDER BY data_criacao DESC LIMIT 20
	`, alunoNome)
	if err != nil {
		writeJSONError(w, "Failed to query history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var historico []services.SVEDHistoricoItem
	for rows.Next() {
		var fichaID int64
		var dataCriacao string
		var fichaJSON string
		if err := rows.Scan(&fichaID, &dataCriacao, &fichaJSON); err != nil {
			continue
		}

		exercicios := extrairExerciciosDeFichaJSON(fichaJSON)

		for _, ex := range exercicios {
			if strings.Contains(strings.ToLower(ex.Nome), strings.ToLower(exercicioNome)) {
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

				densidade := services.CalcularDensidade(float64(tutTotalEx), restSeconds)
				ies := services.CalcularIES(series, reps, cadencia, restSeconds, rir)

				historico = append(historico, services.SVEDHistoricoItem{
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
				})
				break
			}
		}
	}

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

	var studentName string
	err = h.db.QueryRowContext(r.Context(), "SELECT aluno FROM fichas_treino_web WHERE id = ?", fichaID).Scan(&studentName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Ficha não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, data_criacao, ficha_json 
		FROM fichas_treino_web 
		WHERE aluno = ? 
		ORDER BY data_criacao DESC LIMIT 5
	`, studentName)
	if err != nil {
		writeJSONError(w, "Failed to query history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var historico []services.SVEDHistoricoItem
	for rows.Next() {
		var fID int64
		var dataCriacao string
		var fichaJSON string
		if err := rows.Scan(&fID, &dataCriacao, &fichaJSON); err != nil {
			continue
		}

		exercicios := extrairExerciciosDeFichaJSON(fichaJSON)

		for _, ex := range exercicios {
			if strings.Contains(strings.ToLower(ex.Nome), strings.ToLower(exercicioNome)) {
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

				densidade := services.CalcularDensidade(float64(tutTotalEx), restSeconds)
				ies := services.CalcularIES(series, reps, cadencia, restSeconds, rir)

				historico = append(historico, services.SVEDHistoricoItem{
					FichaID:     fID,
					Data:        dataCriacao,
					Series:      series,
					Reps:        reps,
					RIR:         rir,
					Cadencia:    cadencia,
					RestSeconds: restSeconds,
					TutTotal:    tutTotalEx,
					Densidade:   densidade,
					IesScore:    ies,
				})
				break
			}
		}
	}

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

	var alunoNome string
	var titulo string
	var fichaJSON string
	err = h.db.QueryRowContext(r.Context(), `
		SELECT aluno, COALESCE(turma, 'Ficha'), ficha_json
		FROM fichas_treino_web
		WHERE id = ?
	`, fichaID).Scan(&alunoNome, &titulo, &fichaJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Ficha não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var alunoID sql.NullInt64
	if err := h.db.QueryRowContext(r.Context(), "SELECT id FROM alunos WHERE nome = ? ORDER BY id DESC LIMIT 1", alunoNome).Scan(&alunoID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	exercicios := extrairExerciciosDeFichaJSON(fichaJSON)
	responseWarnings := make([]string, 0)
	if len(exercicios) == 0 {
		responseWarnings = append(responseWarnings, "ficha sem exercícios parseáveis para SVED")
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, COALESCE(data_criacao, ''), ficha_json
		FROM fichas_treino_web
		WHERE aluno = ?
		ORDER BY data_criacao DESC
		LIMIT 20
	`, alunoNome)
	if err != nil {
		writeJSONError(w, "Failed to query history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sheets := make([]svedSheet, 0, 20)
	for rows.Next() {
		var sheet svedSheet
		if err := rows.Scan(&sheet.ID, &sheet.DataCriacao, &sheet.FichaJSON); err != nil {
			writeJSONError(w, "Failed to scan history", http.StatusInternalServerError)
			return
		}
		sheets = append(sheets, sheet)
	}
	if err := rows.Err(); err != nil {
		writeJSONError(w, "Failed to iterate history", http.StatusInternalServerError)
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
		"aluno":            alunoNome,
		"titulo":           titulo,
		"total_exercicios": len(sugestoes),
		"sugestoes":        sugestoes,
		"warnings":         responseWarnings,
	}
	if alunoID.Valid {
		resp["aluno_id"] = alunoID.Int64
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

	var alunoNome string
	err = h.db.QueryRowContext(r.Context(), "SELECT nome FROM alunos WHERE id = ?", alunoID).Scan(&alunoNome)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var statsGerais struct {
		IesMedioGeral       float64 `json:"ies_medio_geral"`
		DensidadeMediaGeral float64 `json:"densidade_media_geral"`
		VolumeEfetivoGeral  float64 `json:"volume_efetivo_geral"`
	}

	err = h.db.QueryRowContext(r.Context(), `
		SELECT COALESCE(AVG(ies_score), 0.0), COALESCE(AVG(densidade), 0.0), COALESCE(AVG(volume_sved), 0.0) 
		FROM fichas_treino_web 
		WHERE aluno = ?
	`, alunoNome).Scan(&statsGerais.IesMedioGeral, &statsGerais.DensidadeMediaGeral, &statsGerais.VolumeEfetivoGeral)
	if err != nil {
		writeJSONError(w, "Failed to query stats gerais", http.StatusInternalServerError)
		return
	}

	statsGerais.IesMedioGeral = math.Round(statsGerais.IesMedioGeral*10) / 10
	statsGerais.DensidadeMediaGeral = math.Round(statsGerais.DensidadeMediaGeral*100) / 100
	statsGerais.VolumeEfetivoGeral = math.Round(statsGerais.VolumeEfetivoGeral*10) / 10

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, COALESCE(turma, 'Ficha'), data_criacao, ies_score, tut_total, densidade, volume_sved, ficha_json 
		FROM fichas_treino_web 
		WHERE aluno = ? 
		ORDER BY data_criacao DESC LIMIT 20
	`, alunoNome)
	if err != nil {
		writeJSONError(w, "Failed to query sheets history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var evolucaoTemporal []map[string]any
	var historicoSved []map[string]any
	exerciseGrouping := make(map[string]*ExerciseDashboardStats)

	for rows.Next() {
		var id int64
		var turma string
		var dataCriacao string
		var iesScore float64
		var tutTotal int
		var densidade float64
		var volumeSved int
		var fichaJSON string

		if err := rows.Scan(&id, &turma, &dataCriacao, &iesScore, &tutTotal, &densidade, &volumeSved, &fichaJSON); err != nil {
			continue
		}

		exercicios := extrairExerciciosDeFichaJSON(fichaJSON)
		totalEx := 1
		if len(exercicios) > 0 {
			totalEx = len(exercicios)
		}

		// Group exercises for densidade_por_exercicio
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
			"ficha_id":         id,
			"titulo":           turma,
			"data":             dataCriacao,
			"ies_medio":        iesScore,
			"tut_medio":        float64(tutTotal),
			"densidade_media":  densidade,
			"volume_efetivo":   float64(volumeSved),
			"total_exercicios": totalEx,
		})

		historicoSved = append(historicoSved, map[string]any{
			"ficha_id":  id,
			"data":      dataCriacao,
			"ies_score": iesScore,
			"densidade": densidade,
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
