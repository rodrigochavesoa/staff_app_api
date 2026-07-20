package http

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/repositories"
	"staff_app/internal/services"

	"github.com/go-chi/chi/v5"
)

type FichaTreinoHandler struct {
	repo              repositories.FichaTreinoRepository
	alunoRepo         repositories.AlunoRepository
	fichaRepo         repositories.FichaRepository
	anamRepo          repositories.AnamneseRepository
	ragRepo           repositories.RAGRepository
	trainingAI        *services.TrainingProviderChain
	evidencePipeline  *services.EvidencePipeline
	evidenceTelemetry services.EvidencePipelineTelemetryRecorder
}

func NewFichaTreinoHandler(
	repo repositories.FichaTreinoRepository,
	aluno repositories.AlunoRepository,
	ficha repositories.FichaRepository,
	anam repositories.AnamneseRepository,
	rag repositories.RAGRepository,
	evidence *services.EvidencePipeline,
	telemetry services.EvidencePipelineTelemetryRecorder,
	trainingAI ...*services.TrainingProviderChain,
) *FichaTreinoHandler {
	var chain *services.TrainingProviderChain
	if len(trainingAI) > 0 {
		chain = trainingAI[0]
	}
	return &FichaTreinoHandler{
		repo:              repo,
		alunoRepo:         aluno,
		fichaRepo:         ficha,
		anamRepo:          anam,
		ragRepo:           rag,
		trainingAI:        chain,
		evidencePipeline:  evidence,
		evidenceTelemetry: telemetry,
	}
}

// generateToken gera um token aleatório seguro para links públicos.
func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type ExercicioPrescritoRequest struct {
	Nome          string `json:"nome"`
	GrupoMuscular string `json:"grupo_muscular"`
	Series        int    `json:"series"`
	Repeticoes    string `json:"repeticoes"`
	Carga         string `json:"carga"`
	Descanso      string `json:"descanso"`
	Observacoes   string `json:"observacoes"`
	Cadencia      string `json:"cadencia,omitempty"`
	RIR           int    `json:"rir,omitempty"`
}

type CreateManualFichaRequest struct {
	AlunoID     int64                       `json:"aluno_id"`
	TituloFicha string                      `json:"titulo_ficha"`
	Observacoes string                      `json:"observacoes"`
	Exercicios  []ExercicioPrescritoRequest `json:"exercicios"`
}

func (h *FichaTreinoHandler) CreateManual(w http.ResponseWriter, r *http.Request) {
	var req CreateManualFichaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.AlunoID <= 0 {
		writeJSONError(w, "aluno_id is required", http.StatusBadRequest)
		return
	}
	req.TituloFicha = strings.TrimSpace(req.TituloFicha)
	if req.TituloFicha == "" {
		writeJSONError(w, "titulo_ficha is required", http.StatusBadRequest)
		return
	}

	aluno, err := h.alunoRepo.GetByID(r.Context(), req.AlunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !aluno.Ativo {
		writeJSONError(w, "Aluno inativo", http.StatusBadRequest)
		return
	}

	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")

	fichaData := map[string]any{
		"aluno":            aluno.Nome,
		"titulo":           req.TituloFicha,
		"tipo":             "manual",
		"observacoes":      req.Observacoes,
		"exercicios":       req.Exercicios,
		"total_exercicios": len(req.Exercicios),
		"data_criacao":     nowStr,
	}
	fichaJSONBytes, err := json.Marshal(fichaData)
	if err != nil {
		writeJSONError(w, "Failed to marshal ficha_json", http.StatusInternalServerError)
		return
	}

	var svedExs []services.ExercicioJSON
	for _, ex := range req.Exercicios {
		svedExs = append(svedExs, services.ExercicioJSON{
			Nome:          ex.Nome,
			GrupoMuscular: ex.GrupoMuscular,
			Series:        ex.Series,
			Repeticoes:    ex.Repeticoes,
			Carga:         ex.Carga,
			Descanso:      ex.Descanso,
			Observacoes:   ex.Observacoes,
			Cadencia:      ex.Cadencia,
			RIR:           ex.RIR,
		})
	}
	metricas := services.CalcularMetricasSVED(svedExs)

	var firstSeries string
	var firstRIR int = 2
	var firstCadencia string = "4010"
	var firstRestSeconds int = 60
	if len(req.Exercicios) > 0 {
		firstSeries = strconv.Itoa(req.Exercicios[0].Series)
		firstRIR = req.Exercicios[0].RIR
		if firstRIR <= 0 {
			firstRIR = 2
		}
		firstCadencia = req.Exercicios[0].Cadencia
		if firstCadencia == "" {
			firstCadencia = "4010"
		}
		firstRestSeconds = services.ParseDescanso(req.Exercicios[0].Descanso)
	}

	f := &domain.FichaTreinoWeb{
		Aluno:             aluno.Nome,
		Idade:             aluno.Idade,
		Sexo:              aluno.Sexo,
		Objetivo:          aluno.Objetivo,
		Modalidade:        "Manual",
		Nivel:             "N/A",
		FrequenciaSemanal: 0,
		DuracaoTreino:     0,
		Restricoes:        req.Observacoes,
		Feedback:          "Ficha criada manualmente",
		Turma:             req.TituloFicha,
		ListaExercicios:   "manual_custom",
		DataCriacao:       now,
		FichaJSON:         string(fichaJSONBytes),
		TipoFicha:         "manual",
		NumTreinos:        1,
		Versao:            1,
		IesScore:          metricas.IesMedioJoules,
		VolumeSved:        metricas.VolumeSved,
		Densidade:         metricas.DensidadeMedia,
		TutTotal:          metricas.TutTotal,
		Series:            firstSeries,
		RIR:               firstRIR,
		Cadencia:          firstCadencia,
		RestSeconds:       firstRestSeconds,
	}

	if err := h.repo.Create(r.Context(), f); err != nil {
		writeJSONError(w, "Failed to create manual ficha", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "success",
		"message": "Ficha manual criada com sucesso!",
		"data": map[string]any{
			"id":           f.ID,
			"aluno":        f.Aluno,
			"idade":        f.Idade,
			"sexo":         f.Sexo,
			"objetivo":     f.Objetivo,
			"modalidade":   f.Modalidade,
			"restricoes":   f.Restricoes,
			"turma":        f.Turma,
			"data_criacao": nowStr,
			"ficha_json":   fichaData,
		},
	})
}

func (h *FichaTreinoHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "Invalid ficha ID", http.StatusBadRequest)
		return
	}

	f, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Ficha não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "success",
		"data": map[string]any{
			"id":                 f.ID,
			"aluno":              f.Aluno,
			"idade":              f.Idade,
			"sexo":               f.Sexo,
			"objetivo":           f.Objetivo,
			"modalidade":         f.Modalidade,
			"nivel":              f.Nivel,
			"frequencia_semanal": f.FrequenciaSemanal,
			"duracao_treino":     f.DuracaoTreino,
			"restricoes":         f.Restricoes,
			"feedback":           f.Feedback,
			"turma":              f.Turma,
			"lista_exercicios":   f.ListaExercicios,
			"data_criacao":       f.DataCriacao.Format("2006-01-02 15:04:05"),
			"ficha_json":         json.RawMessage(f.FichaJSON),
			"tipo_ficha":         f.TipoFicha,
			"num_treinos":        f.NumTreinos,
			"versao":             f.Versao,
		},
	})
}

type EditManualFichaRequest struct {
	Observacoes      string                      `json:"observacoes"`
	Exercicios       []ExercicioPrescritoRequest `json:"exercicios"`
	ParametrosTreino *EditFichaParams            `json:"parametros_treino,omitempty"`
	Versao           int                         `json:"versao"`
}

type EditFichaParams struct {
	Perfil     string `json:"perfil"`
	Foco       string `json:"foco"`
	Frequencia int    `json:"frequencia"`
	Duracao    int    `json:"duracao"`
}

func (h *FichaTreinoHandler) EditManual(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "Invalid ficha ID", http.StatusBadRequest)
		return
	}

	var req EditManualFichaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	ifMatch := r.Header.Get("If-Match")
	var reqVersion int
	if ifMatch != "" {
		v, err := strconv.Atoi(strings.Trim(ifMatch, `"`))
		if err == nil {
			reqVersion = v
		}
	}

	if reqVersion == 0 {
		reqVersion = req.Versao
	}

	if reqVersion <= 0 {
		writeJSONError(w, "versao or If-Match header must be a positive integer to ensure optimistic lock", http.StatusBadRequest)
		return
	}

	f, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Ficha não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if f.Versao != reqVersion {
		writeJSONError(w, "Conflito de concorrência ao atualizar a ficha (OCC)", http.StatusConflict)
		return
	}

	if req.ParametrosTreino != nil {
		if req.ParametrosTreino.Perfil != "" {
			f.Nivel = req.ParametrosTreino.Perfil
		}
		if req.ParametrosTreino.Foco != "" {
			f.Objetivo = req.ParametrosTreino.Foco
		}
		if req.ParametrosTreino.Frequencia > 0 {
			f.FrequenciaSemanal = req.ParametrosTreino.Frequencia
		}
		if req.ParametrosTreino.Duracao > 0 {
			f.DuracaoTreino = req.ParametrosTreino.Duracao
		}
	}

	f.Restricoes = req.Observacoes

	var svedExs []services.ExercicioJSON
	for _, ex := range req.Exercicios {
		svedExs = append(svedExs, services.ExercicioJSON{
			Nome:          ex.Nome,
			GrupoMuscular: ex.GrupoMuscular,
			Series:        ex.Series,
			Repeticoes:    ex.Repeticoes,
			Carga:         ex.Carga,
			Descanso:      ex.Descanso,
			Observacoes:   ex.Observacoes,
			Cadencia:      ex.Cadencia,
			RIR:           ex.RIR,
		})
	}
	metricas := services.CalcularMetricasSVED(svedExs)

	var firstSeries string
	var firstRIR int = 2
	var firstCadencia string = "4010"
	var firstRestSeconds int = 60
	if len(req.Exercicios) > 0 {
		firstSeries = strconv.Itoa(req.Exercicios[0].Series)
		firstRIR = req.Exercicios[0].RIR
		if firstRIR <= 0 {
			firstRIR = 2
		}
		firstCadencia = req.Exercicios[0].Cadencia
		if firstCadencia == "" {
			firstCadencia = "4010"
		}
		firstRestSeconds = services.ParseDescanso(req.Exercicios[0].Descanso)
	}

	f.IesScore = metricas.IesMedioJoules
	f.VolumeSved = metricas.VolumeSved
	f.Densidade = metricas.DensidadeMedia
	f.TutTotal = metricas.TutTotal
	f.Series = firstSeries
	f.RIR = firstRIR
	f.Cadencia = firstCadencia
	f.RestSeconds = firstRestSeconds

	fichaData := map[string]any{
		"aluno":            f.Aluno,
		"titulo":           f.Turma,
		"tipo":             f.TipoFicha,
		"observacoes":      req.Observacoes,
		"exercicios":       req.Exercicios,
		"total_exercicios": len(req.Exercicios),
		"ultima_edicao":    time.Now().Format("2006-01-02 15:04:05"),
	}
	fichaJSONBytes, err := json.Marshal(fichaData)
	if err != nil {
		writeJSONError(w, "Failed to marshal updated JSON", http.StatusInternalServerError)
		return
	}
	f.FichaJSON = string(fichaJSONBytes)

	if err := h.repo.Update(r.Context(), f); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Conflito de concorrência ao atualizar a ficha (OCC)", http.StatusConflict)
			return
		}
		writeJSONError(w, "Failed to update ficha", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "success",
		"message": "Ficha atualizada com sucesso!",
		"data":    f,
	})
}

func (h *FichaTreinoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	confirm := r.Header.Get("X-Confirm-Hard-Delete")
	if confirm != "CONFIRMAR" {
		writeJSONError(w, "Cabeçalho X-Confirm-Hard-Delete é obrigatório com valor CONFIRMAR", http.StatusBadRequest)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "Invalid ficha ID", http.StatusBadRequest)
		return
	}

	err = h.repo.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Ficha não encontrada", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "success",
		"message": "Ficha e seus links públicos removidos permanentemente.",
	})
}

type GerarFichaPeriodizadaRequest struct {
	AlunoID     int64  `json:"aluno_id"`
	Frequencia  int    `json:"frequencia"`
	Objetivo    string `json:"objetivo"`
	Nivel       string `json:"nivel"`
	Restricoes  string `json:"restricoes"`
	Observacoes string `json:"observacoes"`
}

func (h *FichaTreinoHandler) GerarPeriodizada(w http.ResponseWriter, r *http.Request) {
	var req GerarFichaPeriodizadaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.AlunoID <= 0 {
		writeJSONError(w, "aluno_id is required", http.StatusBadRequest)
		return
	}
	if req.Frequencia < 2 || req.Frequencia > 6 {
		writeJSONError(w, "Frequência deve estar entre 2 e 6 treinos", http.StatusBadRequest)
		return
	}

	aluno, err := h.alunoRepo.GetByID(r.Context(), req.AlunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !aluno.Ativo {
		writeJSONError(w, "Aluno inativo", http.StatusBadRequest)
		return
	}

	divisoes := map[int][]string{
		2: {"A - Treino Completo Superior", "B - Treino Completo Inferior"},
		3: {"A - Push (Peito, Ombros, Tríceps)", "B - Pull (Costas, Bíceps)", "C - Legs (Pernas, Glúteos)"},
		4: {"A - Peito e Tríceps", "B - Costas e Bíceps", "C - Ombros e Abdômen", "D - Pernas e Glúteos"},
		5: {"A - Peito", "B - Costas", "C - Pernas", "D - Ombros e Braços", "E - Glúteos e Abdômen"},
		6: {"A - Peito Superior", "B - Costas Largura", "C - Pernas Anterior", "D - Peito Inferior", "E - Costas Espessura", "F - Pernas Posterior"},
	}

	divisaoEscolhida := divisoes[req.Frequencia]
	letras := []string{"A", "B", "C", "D", "E", "F"}

	var treinosGerados []map[string]any
	for idx, nomeDivisao := range divisaoEscolhida {
		letra := letras[idx]
		grupo := "Geral"
		if strings.Contains(nomeDivisao, "-") {
			parts := strings.Split(nomeDivisao, "-")
			grupo = strings.TrimSpace(parts[1])
		}

		treino := map[string]any{
			"letra": letra,
			"nome":  nomeDivisao,
			"exercicios": []map[string]any{
				{
					"ordem":          1,
					"nome":           "Exercício Principal 1",
					"grupo_muscular": grupo,
					"series":         3,
					"repeticoes":     "10-12",
					"cadencia":       "4010",
					"descanso":       60,
					"observacoes":    "Executar com técnica correta",
					"video_url":      "",
				},
				{
					"ordem":          2,
					"nome":           "Exercício Principal 2",
					"grupo_muscular": grupo,
					"series":         3,
					"repeticoes":     "10-12",
					"cadencia":       "4010",
					"descanso":       60,
					"observacoes":    "Manter boa postura",
					"video_url":      "",
				},
				{
					"ordem":          3,
					"nome":           "Exercício Acessório 1",
					"grupo_muscular": grupo,
					"series":         3,
					"repeticoes":     "12-15",
					"cadencia":       "3010",
					"descanso":       45,
					"observacoes":    "Foco na contração muscular",
					"video_url":      "",
				},
				{
					"ordem":          4,
					"nome":           "Exercício Acessório 2",
					"grupo_muscular": grupo,
					"series":         3,
					"repeticoes":     "12-15",
					"cadencia":       "3010",
					"descanso":       45,
					"observacoes":    "Controlar a velocidade",
					"video_url":      "",
				},
				{
					"ordem":          5,
					"nome":           "Exercício Final",
					"grupo_muscular": grupo,
					"series":         2,
					"repeticoes":     "15-20",
					"cadencia":       "2010",
					"descanso":       30,
					"observacoes":    "Finalização do treino",
					"video_url":      "",
				},
			},
		}
		treinosGerados = append(treinosGerados, treino)
	}

	fichaPeriodizadaJSON := map[string]any{
		"tipo":        "periodizada",
		"frequencia":  req.Frequencia,
		"objetivo":    req.Objetivo,
		"nivel":       req.Nivel,
		"observacoes": req.Observacoes,
		"treinos":     treinosGerados,
	}

	if h.trainingAI != nil {
		pipelineStart := time.Now()
		contexto, err := h.loadTrainingContext(r.Context(), aluno.ID, req)
		if err != nil {
			writeJSONError(w, "Failed to load training context: "+err.Error(), http.StatusInternalServerError)
			return
		}
		result, err := h.trainingAI.Resolve(r.Context(), &services.GenerationRequest{
			Aluno:       aluno,
			Frequencia:  req.Frequencia,
			Objetivo:    req.Objetivo,
			Nivel:       req.Nivel,
			Restricoes:  req.Restricoes,
			Observacoes: req.Observacoes,
			LocalFicha:  fichaPeriodizadaJSON,
			Contexto:    contexto,
		})
		if err != nil {
			writeJSONError(w, "Failed to generate AI-assisted training: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		fichaPeriodizadaJSON = result.Ficha
		h.recordEvidencePipelineTelemetry(r.Context(), aluno.ID, contexto, result, time.Since(pipelineStart).Milliseconds())
	}

	fichaJSONBytes, err := json.Marshal(fichaPeriodizadaJSON)
	if err != nil {
		writeJSONError(w, "Failed to serialize periodized JSON", http.StatusInternalServerError)
		return
	}

	hashLink, err := generateToken()
	if err != nil {
		writeJSONError(w, "Failed to generate token hash", http.StatusInternalServerError)
		return
	}

	svedExs := extractSVEDExercisesFromFichaMap(fichaPeriodizadaJSON)
	metricas := services.CalcularMetricasSVED(svedExs)

	var firstSeries string
	var firstRIR int = 2
	var firstCadencia string = "4010"
	var firstRestSeconds int = 60
	if len(svedExs) > 0 {
		firstSeries = strconv.Itoa(svedExs[0].Series.(int))
		firstRIR = 2
		firstCadencia = svedExs[0].Cadencia
		firstRestSeconds = svedExs[0].Descanso.(int)
	}

	f := &domain.FichaTreinoWeb{
		Aluno:             aluno.Nome,
		Idade:             aluno.Idade,
		Sexo:              aluno.Sexo,
		Objetivo:          req.Objetivo,
		Modalidade:        "Musculação",
		Nivel:             req.Nivel,
		FrequenciaSemanal: req.Frequencia,
		DuracaoTreino:     60,
		Restricoes:        req.Restricoes,
		Feedback:          "Ficha gerada periodizada",
		Turma:             "Ficha Periodizada " + strconv.Itoa(req.Frequencia) + "x",
		ListaExercicios:   "exercicios_com_grupos",
		DataCriacao:       time.Now(),
		FichaJSON:         string(fichaJSONBytes),
		TipoFicha:         "periodizada",
		NumTreinos:        req.Frequencia,
		Versao:            1,
		IesScore:          metricas.IesMedioJoules,
		VolumeSved:        metricas.VolumeSved,
		Densidade:         metricas.DensidadeMedia,
		TutTotal:          metricas.TutTotal,
		Series:            firstSeries,
		RIR:               firstRIR,
		Cadencia:          firstCadencia,
		RestSeconds:       firstRestSeconds,
	}

	expiraEmStr, err := h.repo.CreatePeriodizadaWithArchiveAndLink(r.Context(), f, hashLink, 90, req.AlunoID)
	if err != nil {
		writeJSONError(w, "Failed to persist periodized sheet and link: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "success",
		"ficha_id":  f.ID,
		"hash_link": hashLink,
		"expira_em": expiraEmStr,
		"data":      fichaPeriodizadaJSON,
	})
}

func extractSVEDExercisesFromFichaMap(ficha map[string]any) []services.ExercicioJSON {
	treinos, ok := ficha["treinos"]
	if !ok {
		return nil
	}

	var svedExs []services.ExercicioJSON
	visitTreino := func(treino map[string]any) {
		exercicios, ok := treino["exercicios"]
		if !ok {
			return
		}
		for _, item := range anySlice(exercicios) {
			exercicio, ok := item.(map[string]any)
			if !ok {
				continue
			}
			series := anyInt(exercicio["series"], 3)
			descanso := anyInt(exercicio["descanso"], 60)
			rir := anyInt(exercicio["rir"], 2)
			svedExs = append(svedExs, services.ExercicioJSON{
				Nome:          anyString(exercicio["nome"]),
				GrupoMuscular: anyString(exercicio["grupo_muscular"]),
				Series:        series,
				Repeticoes:    anyStringDefault(exercicio["repeticoes"], "10-12"),
				Descanso:      descanso,
				Cadencia:      anyStringDefault(exercicio["cadencia"], "4010"),
				RIR:           rir,
				Bloco:         "principal",
			})
		}
	}

	for _, item := range anySlice(treinos) {
		if treino, ok := item.(map[string]any); ok {
			visitTreino(treino)
		}
	}
	return svedExs
}

func anySlice(v any) []any {
	switch typed := v.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func anyStringDefault(v any, fallback string) string {
	if s := anyString(v); s != "" {
		return s
	}
	return fallback
}

func anyInt(v any, fallback int) int {
	switch typed := v.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		i, err := typed.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		i, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(typed), "s"))
		if err == nil {
			return i
		}
	}
	return fallback
}

type MetodoInfo struct {
	Nome           string `json:"nome"`
	Descricao      string `json:"descricao"`
	IndicadoPara   string `json:"indicado_para"`
	Exemplo        string `json:"exemplo"`
	Intensidade    string `json:"intensidade"`
	BaseCientifica string `json:"base_cientifica"`
}

func (h *FichaTreinoHandler) GetMetodoInfo(w http.ResponseWriter, r *http.Request) {
	metodo := chi.URLParam(r, "metodo")
	if metodo == "" {
		writeJSONError(w, "Method name is required", http.StatusBadRequest)
		return
	}

	nomeParaChave := map[string]string{
		"Nenhum":        "tradicional",
		"Drop-set":      "drop_set",
		"Bi-set":        "super_set",
		"Tri-set":       "tradicional",
		"Rest-Pause":    "rest_pause",
		"Super-série":   "super_set",
		"Série Gigante": "tradicional",
		"Biplex":        "tradicional",
		"Triplex":       "tradicional",
	}

	metodosInfo := map[string]MetodoInfo{
		"tradicional": {
			Nome:           "Método Tradicional (Séries Diretas)",
			Descricao:      "Método clássico de treino resistido com séries completas executadas com descanso fixo entre elas. É o alicerce do treinamento de força e hipertrofia.",
			IndicadoPara:   "Todos os níveis (iniciante a avançado). Ideal para aprender técnica correta, desenvolver força base e construir massa muscular de forma progressiva.",
			Exemplo:        "Supino Reto: 4 séries de 10 repetições com 60 segundos de descanso. Executar todas as 10 reps, descansar, repetir.",
			Intensidade:    "Moderada a Alta (depende da carga). Permite controle preciso da intensidade via % 1RM ou RIR.",
			BaseCientifica: "Schoenfeld (2010) demonstrou que séries tradicionais de 6-12 reps com 60-90s de descanso são ótimas para hipertrofia. Haff & Triplett (2016) consolidaram este método como padrão ouro para desenvolvimento de força.",
		},
		"drop_set": {
			Nome:           "Drop-set (Série Descendente)",
			Descricao:      "Técnica avançada onde se reduz a carga imediatamente após atingir a falha muscular, continuando a série sem descanso. Prolonga o tempo sob tensão e maximiza o recrutamento de unidades motoras.",
			IndicadoPara:   "Alunos intermediários e avançados buscando hipertrofia máxima. Requer boa técnica de execução e controle. Ideal para exercícios com ajuste rápido de carga (máquinas, halteres).",
			Exemplo:        "Rosca Direta: 10 reps com 20kg → (sem pausa) → 8 reps com 15kg → (sem pausa) → 6 reps com 10kg. Total: 3 drops em uma única série.",
			Intensidade:    "Alta (RPE 9-10). Causa fadiga significativa e requer recuperação adequada. Usar 1-2x por semana por grupo muscular.",
			BaseCientifica: "Schoenfeld et al. (2017) demonstraram que drop-sets aumentam o estresse metabólico e tempo sob tensão, fatores-chave para hipertrofia. Fink et al. (2018) encontraram ganhos de força similares ao método tradicional com menor volume total.",
		},
		"rest_pause": {
			Nome:           "Rest-Pause",
			Descricao:      "Mini-descansos (10-20 segundos) dentro da mesma série para continuar executando repetições após falha inicial. Maximiza tensão mecânica e volume efetivo em curto período.",
			IndicadoPara:   "Alunos avançados com ótima técnica e alta tolerância à intensidade. Funciona melhor em exercícios compostos (agachamento, supino, desenvolvimento).",
			Exemplo:        "Supino: 8 reps até falha → 15s pausa (barra no rack) → 3 reps → 15s pausa → 2 reps. Total: 13 reps com 2 pausas.",
			Intensidade:    "Muito Alta (RPE 10). Todas as mini-séries vão até a falha. Usar com cautela, máximo 1x por semana por exercício.",
			BaseCientifica: "Haff & Triplett (2016) destacam que rest-pause aumenta o volume total na falha. Robbins et al. (2010) mostraram que prolonga tempo sob tensão sem comprometer carga, ideal para hipertrofia.",
		},
		"super_set": {
			Nome:           "Super-série (Agonista-Antagonista)",
			Descricao:      "Dois exercícios para grupos musculares antagonistas (opostos) executados consecutivamente sem descanso. Exemplo clássico: bíceps + tríceps, peito + costas.",
			IndicadoPara:   "Todos os níveis. Eficiente para economizar tempo sem perder performance. Permite pré-fadiga de um grupo enquanto outro descansa ativamente.",
			Exemplo:        "Supino Reto (peito) 10 reps → imediatamente → Remada Curvada (costas) 10 reps → descanso 90s → repetir.",
			Intensidade:    "Moderada a Alta. A intensidade individual de cada exercício não é comprometida devido ao trabalho de músculos diferentes.",
			BaseCientifica: "Robbins et al. (2010) mostraram que super-séries agonista-antagonista não reduzem força máxima e economizam ~50% do tempo de treino. Fleck & Kraemer (2014) recomendam para hipertrofia e condicionamento.",
		},
		"piramidal": {
			Nome:           "Pirâmide (Progressão de Carga)",
			Descricao:      "Progressão sistemática de carga ao longo das séries. Pode ser crescente (aumenta carga, diminui reps), decrescente (diminui carga, aumenta reps) ou diamante (sobe e desce).",
			IndicadoPara:   "Intermediários e avançados. Excelente para treinos de força onde se busca trabalhar múltiplas faixas de intensidade na mesma sessão.",
			Exemplo:        "Agachamento Pirâmide Crescente: 60kg x 12 reps → 70kg x 10 reps → 80kg x 8 reps → 90kg x 6 reps → 100kg x 4 reps.",
			Intensidade:    "Alta. As últimas séries atingem cargas próximas ao 1RM. Requer aquecimento adequado e progressão controlada.",
			BaseCientifica: "Fleck & Kraemer (2014) demonstram que pirâmides permitem trabalhar tanto hipertrofia (séries iniciais 8-12 reps) quanto força (séries finais 4-6 reps) na mesma sessão. Haff & Triplett (2016) recomendam para periodização não-linear.",
		},
		"fst7": {
			Nome:           "FST-7 (Fascia Stretch Training 7)",
			Descricao:      "7 séries de 8-12 repetições com descansos muito curtos (30-45 segundos) no último exercício do grupo muscular. Foco em pump extremo e expansão da fáscia muscular.",
			IndicadoPara:   "Avançados buscando hipertrofia máxima. Requer excelente condicionamento e recuperação. Aplicar apenas 1-2x por semana no músculo-alvo.",
			Exemplo:        "Leg Press (último exercício de pernas): 7 séries de 10 reps com 30 segundos de descanso. Pump muscular intenso nas séries finais.",
			Intensidade:    "Muito Alta. O acúmulo de metabólitos (lactato, íons H+) é extremo. Sensação de \"bomba\" e congestão muscular intensa.",
			BaseCientifica: "Técnica desenvolvida por Hany Rambod (2008), treinador de fisiculturistas profissionais. Baseada na teoria de que expansão da fáscia permite maior crescimento muscular. Schoenfeld (2013) sugere que o estresse metabólico do FST-7 contribui para hipertrofia via sinalização celular.",
		},
		"daniels": {
			Nome:           "Daniels' Running Formula (Periodização de Corrida)",
			Descricao:      "Sistema de periodização de corrida baseado em VDOT (VO2max estimado) e 5 zonas de treino: E (Easy), M (Marathon), T (Threshold), I (Interval), R (Repetition). Cada zona tem objetivo fisiológico específico.",
			IndicadoPara:   "Corredores de todos os níveis, de 5km a maratona. Método cientificamente validado usado por atletas olímpicos e recreacionais.",
			Exemplo:        "Treino T (Limiar): 3x1600m no ritmo de limiar anaeróbico (ex: 6:30/km para VDOT 45) com 90s de recuperação. Melhora clearance de lactato.",
			Intensidade:    "Variável. E = 59-74% VO2max (leve), M = 75-84% (moderada), T = 83-88% (limiar), I = 95-100% (VO2max), R = >100% (anaeróbica).",
			BaseCientifica: "Desenvolvido por Jack Daniels (PhD), fisiologista do exercício. Publicado em \"Daniels' Running Formula\" (3ª ed. 2014). Validado em centenas de atletas de elite. Baseado em curvas de VDOT e zonas fisiológicas precisas.",
		},
		"sved": {
			Nome:           "SVED (Sistema de Volume Efetivo por Densidade)",
			Descricao:      "Sistema proprietário que mede volume de treino através de RIR (Reps in Reserve), cadência de execução (tempo sob tensão) e densidade de descanso. Independe de carga absoluta, ideal para consultoria online.",
			IndicadoPara:   "Consultoria online (todos os níveis). Perfeito quando o PT não controla equipamentos disponíveis. Permite progressão mensurável via RIR, TUT e densidade.",
			Exemplo:        "3 séries de 10 reps, RIR 2 (poderia fazer 2 a mais), cadência 4012 (4s descida, 0s pausa, 1s subida, 2s pico), 60s rest → IES 46.7/100",
			Intensidade:    "Controlada pelo PT via RIR (0=falha, 4=leve). Permite ajuste fino sem depender de % 1RM. Progressão por redução de RIR, aumento de TUT ou densidade.",
			BaseCientifica: "Baseado em RPE (Rating of Perceived Exertion) de Zourdos et al. (2016) e tempo sob tensão de Burd et al. (2012). Integra conceitos de densidade de Thibaudeau (2006) e cadência de Poliquin (1997). Sistema validado internamente com 50+ alunos online.",
		},
	}

	metodoLower := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(metodo, "-", "_"), " ", "_"))

	if val, ok := nomeParaChave[metodo]; ok {
		metodoLower = val
	}

	if info, ok := metodosInfo[metodoLower]; ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"metodo":  info,
		})
	} else {
		writeJSONError(w, "Método de treino não encontrado", http.StatusNotFound)
	}
}
