package http

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/config"
	"staff_app/internal/corrida/blocos"
	"staff_app/internal/daniels"
	"staff_app/internal/domain"
	"staff_app/internal/repositories"
	"staff_app/internal/services"

	"github.com/go-chi/chi/v5"
)

type PeriodizacaoCorridaHandler struct {
	repo         repositories.PeriodizacaoCorridaRepository
	alunoRepo    repositories.AlunoRepository
	garminRepo   repositories.GarminRepository
	anamneseRepo repositories.AnamneseRepository
	cfg          *config.Config
	templatesPath string
	blocksAI      services.BlocksAIProvider
}

func NewPeriodizacaoCorridaHandler(
	repo repositories.PeriodizacaoCorridaRepository,
	aluno repositories.AlunoRepository,
	garmin repositories.GarminRepository,
	anamnese repositories.AnamneseRepository,
	cfg *config.Config,
) *PeriodizacaoCorridaHandler {
	return &PeriodizacaoCorridaHandler{
		repo:          repo,
		alunoRepo:     aluno,
		garminRepo:    garmin,
		anamneseRepo:  anamnese,
		cfg:           cfg,
		templatesPath: resolveStaffDataPath("json", "templates_daniels_blocos.json"),
		// Default assistive path: local enricher only (no remote API keys).
		blocksAI: services.LocalBlocksAIProvider{},
	}
}

func (h *PeriodizacaoCorridaHandler) WithBlocksAIProvider(p services.BlocksAIProvider) *PeriodizacaoCorridaHandler {
	h.blocksAI = p
	return h
}

func (h *PeriodizacaoCorridaHandler) hasBlocksAIProvider() bool {
	return h.blocksAI != nil && h.blocksAI.Available()
}

// GenerateRequest matches the payload of POST /api/v1/corrida/gerar
type GenerateRequest struct {
	AlunoID        int64   `json:"aluno_id"`
	DistanciaProva string  `json:"distancia_prova"` // "5K", "10K", "21K", "42K"
	DataProva      string  `json:"data_prova"`      // YYYY-MM-DD
	DataInicio     string  `json:"data_inicio"`     // YYYY-MM-DD, optional (defaults to today)
	Nivel          string  `json:"nivel"`           // "iniciante", "intermediario", "avancado", "elite"
	PaceBase       string  `json:"pace_base"`       // MM:SS
	VolumeSemanal  float64 `json:"volume_semanal"`
	DiasSemana     []int   `json:"dias_semana"`
	UsarBlocos     bool    `json:"usar_blocos"`
	ModoGeracao    string  `json:"modo_geracao"`
}

// EditTreinoRequest matches the payload of PUT /api/v1/corrida/{id}/editar-treino
type EditTreinoRequest struct {
	Semana    int     `json:"semana"`
	Dia       int     `json:"dia"`
	Tipo      string  `json:"tipo"`
	Distancia float64 `json:"distancia"`
	Zona      string  `json:"zona"`
	PaceAlvo  string  `json:"pace_alvo"`
	Descricao string  `json:"descricao"`
}

// UpdateSemanaRequest matches the payload of PUT /api/v1/corrida/{id}/semana-atual
type UpdateSemanaRequest struct {
	SemanaAtual int `json:"semana_atual"`
}

// ConcluirTreinoRequest matches the payload of POST /api/v1/corrida/publica/{hash}/concluir
type ConcluirTreinoRequest struct {
	Semana    int  `json:"semana"`
	Dia       int  `json:"dia"`
	Concluido bool `json:"concluido"`
}

func (h *PeriodizacaoCorridaHandler) Gerar(w http.ResponseWriter, r *http.Request) {
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// 1. Input validations
	if req.AlunoID <= 0 {
		returnError(w, http.StatusBadRequest, "Aluno ID deve ser maior que zero")
		return
	}
	if req.DistanciaProva != "5K" && req.DistanciaProva != "10K" && req.DistanciaProva != "21K" && req.DistanciaProva != "42K" {
		returnError(w, http.StatusBadRequest, "Distância de prova inválida. Deve ser 5K, 10K, 21K ou 42K")
		return
	}
	if req.Nivel != "iniciante" && req.Nivel != "intermediario" && req.Nivel != "avancado" && req.Nivel != "elite" {
		returnError(w, http.StatusBadRequest, "Nível inválido. Deve ser iniciante, intermediario, avancado ou elite")
		return
	}
	paceRegex := regexp.MustCompile(`^\d{2}:\d{2}$`)
	if !paceRegex.MatchString(req.PaceBase) {
		returnError(w, http.StatusBadRequest, "Pace base deve estar no formato MM:SS (ex: 05:30)")
		return
	}
	paceSec, err := ParsePace(req.PaceBase)
	if err != nil || paceSec < 120 || paceSec > 900 {
		returnError(w, http.StatusBadRequest, "Pace base inválido ou fora dos limites permitidos (02:00 a 15:00)")
		return
	}
	if err := validateVolume(req.DistanciaProva, req.Nivel, req.VolumeSemanal); err != nil {
		returnError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.DiasSemana) < 2 || len(req.DiasSemana) > 7 {
		returnError(w, http.StatusBadRequest, "Quantidade de dias de treino por semana deve ser de 2 a 7 dias")
		return
	}

	seen := make(map[int]bool)
	for _, d := range req.DiasSemana {
		if d < 1 || d > 7 {
			returnError(w, http.StatusBadRequest, "Dia de treino deve estar entre 1 (segunda) e 7 (domingo)")
			return
		}
		if seen[d] {
			returnError(w, http.StatusBadRequest, "Dias de treino selecionados não podem conter duplicados")
			return
		}
		seen[d] = true
	}

	if req.DataInicio == "" {
		req.DataInicio = time.Now().Format("2006-01-02")
	}
	dtInicio, err := time.Parse("2006-01-02", req.DataInicio)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Data de início deve estar no formato YYYY-MM-DD")
		return
	}
	dtProva, err := time.Parse("2006-01-02", req.DataProva)
	if err != nil {
		returnError(w, http.StatusBadRequest, "Data da prova deve estar no formato YYYY-MM-DD")
		return
	}
	if !dtProva.After(dtInicio) {
		returnError(w, http.StatusBadRequest, "Data da prova deve ser estritamente posterior à data de início")
		return
	}

	// 2. Duration calculation with Monday alignment
	segundaInicio := mondayOfWeek(dtInicio)
	segundaProva := mondayOfWeek(dtProva)
	duracaoSemanas := int(segundaProva.Sub(segundaInicio).Hours()/(24*7)) + 1

	if duracaoSemanas < 4 || duracaoSemanas > 24 {
		returnError(w, http.StatusBadRequest, fmt.Sprintf("Duração do plano de corrida deve ser entre 4 e 24 semanas. Calculado: %d semanas.", duracaoSemanas))
		return
	}

	// 3. Check Aluno exists and is active
	aluno, err := h.alunoRepo.GetByID(r.Context(), req.AlunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, fmt.Sprintf("Aluno com ID %d não encontrado", req.AlunoID))
			return
		}
		returnError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	if !aluno.Ativo {
		returnError(w, http.StatusBadRequest, "Aluno inativo")
		return
	}

	// 4. Prepare timestamps for archiving/generation
	nowTimeStr := time.Now().Format("2006-01-02 15:04:05")

	// 5. Estimate VDOT from race distance + pace (sec/km), then target zones
	distKM, ok := raceDistanceKM(req.DistanciaProva)
	if !ok {
		returnError(w, http.StatusBadRequest, "Distância de prova inválida")
		return
	}
	raceTimeSec := int(float64(paceSec)*distKM + 0.5)
	vdot, err := daniels.EstimateVDOTByRace(raceTimeSec, distKM)
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Error estimating VDOT")
		return
	}
	if req.Nivel == "iniciante" && vdot > 45.0 {
		vdot = 45.0
	}

	danielsZones := daniels.CalculateZones(vdot)
	zonasMap := make(map[string]domain.ZoneDetails)
	for k, z := range danielsZones {
		zonasMap[k] = domain.ZoneDetails{
			PaceAlvo:  FormatPace(z.PaceAlvo),
			Descricao: z.Descricao,
		}
	}
	zonasMap["RACE"] = domain.ZoneDetails{
		PaceAlvo:  req.PaceBase,
		Descricao: "Ritmo alvo da prova",
	}

	modoGeracao, err := blocos.NormalizeModoGeracao(req.UsarBlocos, req.ModoGeracao)
	if err != nil {
		returnError(w, http.StatusBadRequest, err.Error())
		return
	}

	var planoDetalhado domain.PlanoDetalhado
	if modoGeracao == blocos.ModoBlocosCompleta || modoGeracao == blocos.ModoSemanaASemana {
		templates, err := blocos.LoadTemplates(h.templatesPath)
		if err != nil {
			returnError(w, http.StatusInternalServerError, "Falha ao carregar templates de blocos")
			return
		}
		planoDetalhado, err = blocos.GeneratePlano(templates, vdot, req.DistanciaProva, distKM, duracaoSemanas, req.Nivel, req.DiasSemana, modoGeracao, zonasMap)
		if err != nil {
			returnError(w, http.StatusInternalServerError, fmt.Sprintf("Falha ao gerar plano com blocos: %v", err))
			return
		}
	} else {
		// Flat mode: generate detailed training weeks deterministically.
		semanas := make([]domain.SemanaJSON, duracaoSemanas)
		for i := 1; i <= duracaoSemanas; i++ {
			fase := GetFaseForWeek(i, duracaoSemanas)

			var weekVol float64
			w1 := duracaoSemanas / 4
			if w1 < 1 {
				w1 = 1
			}
			w2 := duracaoSemanas / 4
			if w2 < 1 {
				w2 = 1
			}
			w3 := duracaoSemanas / 4
			if w3 < 1 {
				w3 = 1
			}
			w4 := duracaoSemanas - w1 - w2 - w3

			switch fase {
			case "Base":
				ratio := 0.6
				if w1 > 1 {
					ratio = 0.6 + 0.4*float64(i-1)/float64(w1-1)
				} else {
					ratio = 1.0
				}
				weekVol = req.VolumeSemanal * ratio
			case "Build", "Intensidade":
				weekVol = req.VolumeSemanal
			default: // Taper
				k := i - (w1 + w2 + w3)
				ratio := 0.8
				if w4 > 1 {
					ratio = 0.8 - 0.3*float64(k-1)/float64(w4-1)
				} else {
					ratio = 0.5
				}
				weekVol = req.VolumeSemanal * ratio
			}

			isRecovery := false
			if duracaoSemanas >= 12 && i%4 == 0 && fase != "Taper" {
				weekVol = weekVol * 0.70
				isRecovery = true
			}

			weekVol = math.Round(weekVol*10) / 10

			numDays := len(req.DiasSemana)
			treinos := make([]domain.TreinoJSON, numDays)

			weights := make([]float64, numDays)
			switch numDays {
			case 2:
				weights[0] = 0.4
				weights[1] = 0.6
			case 3:
				weights[0] = 0.3
				weights[1] = 0.3
				weights[2] = 0.4
			case 4:
				weights[0] = 0.2
				weights[1] = 0.25
				weights[2] = 0.2
				weights[3] = 0.35
			case 5:
				weights[0] = 0.15
				weights[1] = 0.2
				weights[2] = 0.15
				weights[3] = 0.2
				weights[4] = 0.3
			default:
				for d := 0; d < numDays-1; d++ {
					weights[d] = 0.75 / float64(numDays-1)
				}
				weights[numDays-1] = 0.25
			}

			for d := 0; d < numDays; d++ {
				diaSemana := req.DiasSemana[d]
				distRaw := weekVol * weights[d]
				dist := math.Round(distRaw*10) / 10

				isLastDay := (d == numDays-1)
				isQualityDay := (d == 1 && numDays >= 4) || (d == 0 && numDays == 3)

				var tipo, zona, paceAlvo, descricao string

				if isRecovery {
					if isLastDay {
						tipo = "Long Run"
						zona = "E"
						paceAlvo = zonasMap["E"].PaceAlvo
						descricao = "Corrida longa em ritmo confortável e regenerativo."
					} else {
						tipo = "Corrida Fácil"
						zona = "E"
						paceAlvo = zonasMap["E"].PaceAlvo
						descricao = "Corrida fácil regenerativa de recuperação."
					}
				} else {
					switch fase {
					case "Base":
						if isLastDay {
							tipo = "Long Run"
							zona = "E"
							paceAlvo = zonasMap["E"].PaceAlvo
							descricao = "Corrida longa em ritmo fácil para desenvolver resistência aeróbica."
						} else {
							tipo = "Corrida Fácil"
							zona = "E"
							paceAlvo = zonasMap["E"].PaceAlvo
							descricao = "Corrida contínua em ritmo fácil e confortável."
						}
					case "Build":
						if isLastDay {
							tipo = "Long Run"
							zona = "E"
							paceAlvo = zonasMap["E"].PaceAlvo
							descricao = "Corrida longa em ritmo fácil."
						} else if isQualityDay && dist >= 6.0 {
							tipo = "Tempo Run"
							zona = "T"
							paceAlvo = zonasMap["T"].PaceAlvo
							distT := math.Round((dist-4.0)*10) / 10
							if distT < 1.0 {
								distT = 1.0
							}
							descricao = fmt.Sprintf("Aquecimento 2km E + %.1fkm em ritmo de Limiar (zona T) + Desaquecimento 2km E.", distT)
						} else {
							tipo = "Corrida Fácil"
							zona = "E"
							paceAlvo = zonasMap["E"].PaceAlvo
							descricao = "Corrida contínua em ritmo fácil."
						}
					case "Intensidade":
						if isLastDay {
							tipo = "Long Run"
							zona = "E"
							paceAlvo = zonasMap["E"].PaceAlvo
							descricao = "Corrida longa em ritmo fácil."
						} else if isQualityDay && dist >= 6.0 {
							tipo = "Intervalados"
							zona = "I"
							paceAlvo = zonasMap["I"].PaceAlvo
							tiros := int(math.Floor(dist - 4.0))
							if tiros < 1 {
								tiros = 1
							}
							descricao = fmt.Sprintf("Aquecimento 2km E + %d tiros de 1km na zona I (recuperação 3min trote) + Desaquecimento 2km E.", tiros)
						} else {
							tipo = "Corrida Fácil"
							zona = "E"
							paceAlvo = zonasMap["E"].PaceAlvo
							descricao = "Corrida contínua em ritmo fácil."
						}
					case "Taper":
						if i == duracaoSemanas && isLastDay {
							tipo = "Corrida de Prova"
							zona = "RACE"
							dist = distKM
							paceAlvo = req.PaceBase
							descricao = "Dia da Prova! Mantenha o ritmo planejado e aproveite a corrida."
						} else if isLastDay {
							tipo = "Long Run"
							zona = "E"
							paceAlvo = zonasMap["E"].PaceAlvo
							descricao = "Corrida longa em ritmo fácil com volume reduzido."
						} else if isQualityDay && i != duracaoSemanas && dist >= 6.0 {
							tipo = "Tempo Run"
							zona = "T"
							paceAlvo = zonasMap["T"].PaceAlvo
							distT := math.Round((dist-4.0)*0.5*10) / 10
							if distT < 1.0 {
								distT = 1.0
							}
							descricao = fmt.Sprintf("Aquecimento 2km E + %.1fkm na zona T + Desaquecimento 2km E.", distT)
						} else {
							tipo = "Corrida Fácil"
							zona = "E"
							paceAlvo = zonasMap["E"].PaceAlvo
							descricao = "Corrida curta e fácil de polimento."
						}
					}
				}

				treinos[d] = domain.TreinoJSON{
					Dia:       diaSemana,
					Tipo:      tipo,
					Distancia: dist,
					Zona:      zona,
					PaceAlvo:  paceAlvo,
					Descricao: descricao,
					Concluido: false,
				}
			}

			suplDesc := "Fortalecimento geral de core (15-20 min)."
			if req.Nivel == "avancado" || req.Nivel == "elite" {
				switch fase {
				case "Base", "Build":
					suplDesc = "Fortalecimento de membros inferiores e core (30 min)."
				case "Intensidade":
					suplDesc = "Exercícios de mobilidade e estabilidade (20 min)."
				}
			}

			semanas[i-1] = domain.SemanaJSON{
				Numero:      i,
				Fase:        fase,
				VolumeTotal: weekVol,
				Treinos:     treinos,
				TreinamentoSuplementar: map[string]any{
					"descricao": suplDesc,
				},
			}
		}

		planoDetalhado = domain.PlanoDetalhado{
			VDOT:                   vdot,
			DistanciaProva:         distKM,
			DuracaoSemanas:         duracaoSemanas,
			DiasSemanaSelecionados: req.DiasSemana,
			Zonas:                  zonasMap,
			Semanas:                semanas,
		}
	}

	planoJSONBytes, err := json.Marshal(planoDetalhado)
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to serialize plan details")
		return
	}

	diasSemanaBytes, _ := json.Marshal(req.DiasSemana)

	pc := domain.PeriodizacaoCorrida{
		AlunoID:                req.AlunoID,
		DataInicio:             req.DataInicio,
		DuracaoSemanas:         duracaoSemanas,
		Modo:                   fmt.Sprintf("%s_%s", req.DistanciaProva, req.Nivel),
		SemanaAtual:            1,
		Status:                 "ativo",
		DistanciaProva:         distKM,
		Nivel:                  req.Nivel,
		VDOT:                   vdot,
		PaceBase:               paceSec,
		VolumeSemanal:          req.VolumeSemanal,
		DiasDisponiveis:        len(req.DiasSemana),
		PlanoJSON:              string(planoJSONBytes),
		ModoGeracao:            modoGeracao,
		DataUltimaGeracao:      nowTimeStr,
		DiasSemanaSelecionados: string(diasSemanaBytes),
		Versao:                 1,
	}

	if err := h.repo.CreateWithArchiveActive(r.Context(), &pc, nowTimeStr); err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to persist new periodization")
		return
	}

	returnSuccess(w, http.StatusCreated, pc, "Plano de corrida gerado com sucesso")
}

func (h *PeriodizacaoCorridaHandler) ListByAluno(w http.ResponseWriter, r *http.Request) {
	alunoIDStr := chi.URLParam(r, "aluno_id")
	if alunoIDStr == "" {
		alunoIDStr = chi.URLParam(r, "id")
	}
	alunoID, err := strconv.ParseInt(alunoIDStr, 10, 64)
	if err != nil || alunoID <= 0 {
		returnError(w, http.StatusBadRequest, "Invalid Aluno ID")
		return
	}

	list, err := h.repo.ListByAlunoID(r.Context(), alunoID)
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to retrieve plans list")
		return
	}

	returnSuccess(w, http.StatusOK, list, "")
}

func (h *PeriodizacaoCorridaHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		returnError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	pc, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Plano de corrida não encontrado")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to retrieve plan details")
		return
	}

	var pd domain.PlanoDetalhado
	if pc.PlanoJSON != "" {
		if err := json.Unmarshal([]byte(pc.PlanoJSON), &pd); err == nil {
			// Nest detailed content inside response
			type detailResp struct {
				domain.PeriodizacaoCorrida
				PlanoDetalhado domain.PlanoDetalhado `json:"plano_detalhado"`
			}
			resp := detailResp{
				PeriodizacaoCorrida: *pc,
				PlanoDetalhado:      pd,
			}
			returnSuccess(w, http.StatusOK, resp, "")
			return
		}
	}

	returnSuccess(w, http.StatusOK, pc, "")
}

func (h *PeriodizacaoCorridaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		returnError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	confirm := r.Header.Get("X-Confirm-Hard-Delete")
	if confirm != "CONFIRMAR" {
		returnError(w, http.StatusBadRequest, "Cabeçalho X-Confirm-Hard-Delete inválido ou ausente")
		return
	}

	err = h.repo.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Plano de corrida não encontrado")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to delete plan")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Plano de corrida excluído com sucesso",
	})
}

func (h *PeriodizacaoCorridaHandler) EditarTreino(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		returnError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var req EditTreinoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	var updateErr error
	var finalVersao int

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

		var pd domain.PlanoDetalhado
		if err := json.Unmarshal([]byte(pc.PlanoJSON), &pd); err != nil {
			returnError(w, http.StatusInternalServerError, "Failed to parse plan_json")
			return
		}

		// Find and update the training session
		found := false
		for sIdx := range pd.Semanas {
			if pd.Semanas[sIdx].Numero == req.Semana {
				for tIdx := range pd.Semanas[sIdx].Treinos {
					if pd.Semanas[sIdx].Treinos[tIdx].Dia == req.Dia {
						pd.Semanas[sIdx].Treinos[tIdx].Tipo = req.Tipo
						pd.Semanas[sIdx].Treinos[tIdx].Distancia = req.Distancia
						pd.Semanas[sIdx].Treinos[tIdx].Zona = req.Zona
						pd.Semanas[sIdx].Treinos[tIdx].PaceAlvo = req.PaceAlvo
						pd.Semanas[sIdx].Treinos[tIdx].Descricao = req.Descricao
						found = true
						break
					}
				}
				break
			}
		}

		if !found {
			returnError(w, http.StatusBadRequest, fmt.Sprintf("Treino para semana %d e dia %d não encontrado no plano", req.Semana, req.Dia))
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
			break // unexpected database error
		}

		// Backoff before retry
		time.Sleep(time.Duration(10+i*20) * time.Millisecond)
	}

	if updateErr != nil {
		if errors.Is(updateErr, sql.ErrNoRows) {
			returnError(w, http.StatusConflict, "Conflito de concorrência ao atualizar o plano. O plano foi modificado por outro usuário.")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to update training session")
		return
	}

	returnSuccess(w, http.StatusOK, map[string]int{"versao": finalVersao}, "Treino atualizado com sucesso")
}

func (h *PeriodizacaoCorridaHandler) UpdateSemana(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		returnError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	var req UpdateSemanaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	var updateErr error

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

		if req.SemanaAtual < 1 || req.SemanaAtual > pc.DuracaoSemanas {
			returnError(w, http.StatusBadRequest, fmt.Sprintf("Semana atual deve estar entre 1 e %d", pc.DuracaoSemanas))
			return
		}

		pc.SemanaAtual = req.SemanaAtual
		updateErr = h.repo.Update(r.Context(), pc)
		if updateErr == nil {
			break
		}
		if !errors.Is(updateErr, sql.ErrNoRows) {
			break
		}

		time.Sleep(time.Duration(10+i*20) * time.Millisecond)
	}

	if updateErr != nil {
		if errors.Is(updateErr, sql.ErrNoRows) {
			returnError(w, http.StatusConflict, "Conflito de concorrência ao atualizar a semana atual")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to update current week")
		return
	}

	returnSuccess(w, http.StatusOK, nil, fmt.Sprintf("Semana atualizada para %d", req.SemanaAtual))
}

func (h *PeriodizacaoCorridaHandler) GerarLink(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		returnError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	pc, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Plano de corrida não encontrado")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to retrieve plan details")
		return
	}

	hash, err := generateHexHash()
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to generate public hash")
		return
	}

	// Deactivate any existing active public links for this periodization
	existing, err := h.repo.GetPublicLinkByPeriodizacaoID(r.Context(), id)
	if err == nil && existing != nil {
		existing.Ativo = 0
		_ = h.repo.UpdatePublicLink(r.Context(), existing)
	}

	var coachID *int64
	if user, ok := UserFromContext(r.Context()); ok {
		val := user.ID
		coachID = &val
	}

	now := time.Now()
	expiraEm := now.AddDate(0, 0, 90) // 90 days validity

	link := domain.PeriodizacaoCorridaWeb{
		Hash:           hash,
		PeriodizacaoID: id,
		AlunoID:        pc.AlunoID,
		UserID:         coachID,
		CriadoEm:       now,
		ExpiraEm:       expiraEm,
		Acessos:        0,
		Ativo:          1,
	}

	if err := h.repo.CreatePublicLink(r.Context(), &link); err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to persist public link")
		return
	}

	type linkResp struct {
		Hash       string    `json:"hash"`
		URLPublica string    `json:"url_publica"`
		ExpiraEm   time.Time `json:"expira_em"`
	}

	resp := linkResp{
		Hash:       hash,
		URLPublica: fmt.Sprintf("/api/v1/corrida/publica/%s", hash),
		ExpiraEm:   expiraEm,
	}

	returnSuccess(w, http.StatusCreated, resp, "Link público gerado com sucesso")
}

func (h *PeriodizacaoCorridaHandler) RenovarLink(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		returnError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	link, err := h.repo.GetPublicLinkByPeriodizacaoID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Link público ativo não encontrado para este plano")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to retrieve public link")
		return
	}

	// Extend validity by 30 days
	link.ExpiraEm = link.ExpiraEm.AddDate(0, 0, 30)
	if err := h.repo.UpdatePublicLink(r.Context(), link); err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to renew public link")
		return
	}

	returnSuccess(w, http.StatusOK, map[string]time.Time{"expira_em": link.ExpiraEm}, "Link público renovado com sucesso")
}

func (h *PeriodizacaoCorridaHandler) DesativarLink(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		returnError(w, http.StatusBadRequest, "Invalid ID")
		return
	}

	link, err := h.repo.GetPublicLinkByPeriodizacaoID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Link público ativo não encontrado para este plano")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to retrieve public link")
		return
	}

	link.Ativo = 0
	if err := h.repo.UpdatePublicLink(r.Context(), link); err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to deactivate public link")
		return
	}

	returnSuccess(w, http.StatusOK, nil, "Link público desativado com sucesso")
}

func (h *PeriodizacaoCorridaHandler) GetPublicPlano(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		returnError(w, http.StatusBadRequest, "Hash is required")
		return
	}

	link, err := h.repo.GetPublicLinkByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Link público não encontrado")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to check public link")
		return
	}

	if link.Ativo == 0 || link.ExpiraEm.Before(time.Now()) {
		returnError(w, http.StatusForbidden, "Este link público expirou ou foi desativado")
		return
	}

	// Increment access count
	_ = h.repo.IncrementPublicLinkAccess(r.Context(), hash)

	pc, err := h.repo.GetByID(r.Context(), link.PeriodizacaoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Plano de corrida não encontrado")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to retrieve plan details")
		return
	}

	var pd domain.PlanoDetalhado
	if err := json.Unmarshal([]byte(pc.PlanoJSON), &pd); err != nil {
		returnError(w, http.StatusInternalServerError, "Failed to parse plan details")
		return
	}

	// Resumed/Structured public payload
	type publicPlanoResp struct {
		ID             int64                 `json:"id"`
		AlunoNome      string                `json:"aluno_nome"`
		DataInicio     string                `json:"data_inicio"`
		DuracaoSemanas int                   `json:"duracao_semanas"`
		SemanaAtual    int                   `json:"semana_atual"`
		DistanciaProva float64               `json:"distancia_prova"`
		Nivel          string                `json:"nivel"`
		PlanoDetalhado domain.PlanoDetalhado `json:"plano_detalhado"`
	}

	resp := publicPlanoResp{
		ID:             pc.ID,
		AlunoNome:      pc.AlunoNome,
		DataInicio:     pc.DataInicio,
		DuracaoSemanas: pc.DuracaoSemanas,
		SemanaAtual:    pc.SemanaAtual,
		DistanciaProva: pc.DistanciaProva,
		Nivel:          pc.Nivel,
		PlanoDetalhado: pd,
	}

	returnSuccess(w, http.StatusOK, resp, "")
}

func (h *PeriodizacaoCorridaHandler) ConcluirTreinoPublico(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		returnError(w, http.StatusBadRequest, "Hash is required")
		return
	}

	var req ConcluirTreinoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		returnError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	link, err := h.repo.GetPublicLinkByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Link público não encontrado")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to check public link")
		return
	}

	if link.Ativo == 0 || link.ExpiraEm.Before(time.Now()) {
		returnError(w, http.StatusForbidden, "Este link público expirou ou foi desativado")
		return
	}

	var updateErr error
	for i := 0; i < 10; i++ {
		pc, err := h.repo.GetByID(r.Context(), link.PeriodizacaoID)
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
			returnError(w, http.StatusInternalServerError, "Failed to parse plan details")
			return
		}

		found := false
		for sIdx := range pd.Semanas {
			if pd.Semanas[sIdx].Numero == req.Semana {
				for tIdx := range pd.Semanas[sIdx].Treinos {
					if pd.Semanas[sIdx].Treinos[tIdx].Dia == req.Dia {
						pd.Semanas[sIdx].Treinos[tIdx].Concluido = req.Concluido
						found = true
						break
					}
				}
				break
			}
		}

		if !found {
			returnError(w, http.StatusBadRequest, fmt.Sprintf("Treino para semana %d e dia %d não encontrado no plano", req.Semana, req.Dia))
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
			break
		}
		if !errors.Is(updateErr, sql.ErrNoRows) {
			break
		}

		time.Sleep(time.Duration(10+i*20) * time.Millisecond)
	}

	if updateErr != nil {
		if errors.Is(updateErr, sql.ErrNoRows) {
			returnError(w, http.StatusConflict, "Conflito de concorrência ao atualizar o progresso do treino. Por favor, tente novamente.")
			return
		}
		returnError(w, http.StatusInternalServerError, "Failed to update training status")
		return
	}

	returnSuccess(w, http.StatusOK, nil, "Status do treino atualizado com sucesso")
}

// Helper functions for parsing and calculations
func ParsePace(s string) (int, error) {
	var m, sec int
	_, err := fmt.Sscanf(s, "%d:%d", &m, &sec)
	if err != nil || sec < 0 || sec >= 60 || m < 0 {
		return 0, fmt.Errorf("invalid pace format: %s", s)
	}
	return m*60 + sec, nil
}

func FormatPace(seconds int) string {
	m := seconds / 60
	s := seconds % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

func mondayOfWeek(t time.Time) time.Time {
	wd := int(t.Weekday())
	daysToSubtract := wd - 1
	if wd == 0 {
		daysToSubtract = 6
	}
	year, month, day := t.Date()
	tZero := time.Date(year, month, day, 0, 0, 0, 0, t.Location())
	return tZero.AddDate(0, 0, -daysToSubtract)
}

func GetFaseForWeek(weekNum, totalWeeks int) string {
	w1 := totalWeeks / 4
	if w1 < 1 {
		w1 = 1
	}
	w2 := totalWeeks / 4
	if w2 < 1 {
		w2 = 1
	}
	w3 := totalWeeks / 4
	if w3 < 1 {
		w3 = 1
	}

	if weekNum <= w1 {
		return "Base"
	} else if weekNum <= w1+w2 {
		return "Build"
	} else if weekNum <= w1+w2+w3 {
		return "Intensidade"
	}
	return "Taper"
}

func generateHexHash() (string, error) {
	b := make([]byte, 12) // 12 bytes = 24 hex characters
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// raceDistanceKM maps distancia_prova labels to kilometers for Daniels VDOT.
func raceDistanceKM(distanciaProva string) (float64, bool) {
	switch strings.TrimSpace(distanciaProva) {
	case "5K":
		return 5.0, true
	case "10K":
		return 10.0, true
	case "21K":
		return 21.1, true
	case "42K":
		return 42.2, true
	default:
		return 0, false
	}
}

func validateVolume(dist string, level string, vol float64) error {
	var minVol, maxVol float64
	switch dist {
	case "5K":
		switch level {
		case "iniciante":
			minVol, maxVol = 10, 20
		case "intermediario":
			minVol, maxVol = 20, 35
		case "avancado":
			minVol, maxVol = 30, 50
		case "elite":
			minVol, maxVol = 50, 80
		default:
			return fmt.Errorf("nível inválido: %s", level)
		}
	case "10K":
		switch level {
		case "iniciante":
			minVol, maxVol = 15, 30
		case "intermediario":
			minVol, maxVol = 30, 50
		case "avancado":
			minVol, maxVol = 45, 70
		case "elite":
			minVol, maxVol = 70, 110
		default:
			return fmt.Errorf("nível inválido: %s", level)
		}
	case "21K":
		switch level {
		case "iniciante":
			minVol, maxVol = 25, 45
		case "intermediario":
			minVol, maxVol = 45, 75
		case "avancado":
			minVol, maxVol = 65, 100
		case "elite":
			minVol, maxVol = 95, 140
		default:
			return fmt.Errorf("nível inválido: %s", level)
		}
	case "42K":
		switch level {
		case "iniciante":
			minVol, maxVol = 30, 55
		case "intermediario":
			minVol, maxVol = 55, 95
		case "avancado":
			minVol, maxVol = 80, 130
		case "elite":
			minVol, maxVol = 110, 180
		default:
			return fmt.Errorf("nível inválido: %s", level)
		}
	default:
		return fmt.Errorf("distância de prova inválida: %s", dist)
	}

	if vol < minVol || vol > maxVol {
		return fmt.Errorf("volume semanal de %.1fkm inválido para %s %s, deve estar entre %.1fkm e %.1fkm", vol, dist, level, minVol, maxVol)
	}
	return nil
}

func returnError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "error",
		"message": msg,
	})
}

func returnSuccess(w http.ResponseWriter, status int, data any, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]any{
		"status": "success",
	}
	if msg != "" {
		resp["message"] = msg
	}
	if data != nil {
		resp["data"] = data
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// GetCorridaTreinosDia handles authenticated GET /api/v1/alunos/{id}/corrida/treinos-dia
func (h *PeriodizacaoCorridaHandler) GetCorridaTreinosDia(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	alunoID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || alunoID <= 0 {
		returnError(w, http.StatusBadRequest, "ID do aluno inválido")
		return
	}

	// Verify student exists
	_, err = h.alunoRepo.GetByID(r.Context(), alunoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			returnError(w, http.StatusNotFound, "Aluno não encontrado")
			return
		}
		returnError(w, http.StatusInternalServerError, "Internal server error checking student")
		return
	}

	// Parse mes and ano query parameters
	now := time.Now()
	mesStr := r.URL.Query().Get("mes")
	mes := int(now.Month())
	if mesStr != "" {
		if val, err := strconv.Atoi(mesStr); err == nil && val >= 1 && val <= 12 {
			mes = val
		} else {
			returnError(w, http.StatusBadRequest, "Mês inválido")
			return
		}
	}

	anoStr := r.URL.Query().Get("ano")
	ano := now.Year()
	if anoStr != "" {
		if val, err := strconv.Atoi(anoStr); err == nil && val > 1900 && val < 2100 {
			ano = val
		} else {
			returnError(w, http.StatusBadRequest, "Ano inválido")
			return
		}
	}

	// Query periodization plans for this student
	plans, err := h.repo.ListByAlunoID(r.Context(), alunoID)
	if err != nil {
		returnError(w, http.StatusInternalServerError, "Internal server error fetching plans")
		return
	}

	type TreinoDia struct {
		Data      string  `json:"data"`
		Tipo      string  `json:"tipo"`
		Distancia float64 `json:"distancia"`
		Zona      string  `json:"zona"`
		PaceAlvo  string  `json:"pace_alvo"`
		Descricao string  `json:"descricao"`
		Concluido bool    `json:"concluido"`
		SemanaIdx int     `json:"semana_idx"`
		DiaIdx    int     `json:"dia_idx"`
		PlanoID   int64   `json:"plano_id"`
	}

	treinosPorDia := make([]TreinoDia, 0)

	for _, pc := range plans {
		if pc.Status != "ativo" && pc.Status != "arquivado" {
			continue
		}
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
					treinosPorDia = append(treinosPorDia, TreinoDia{
						Data:      tDate.Format("2006-01-02"),
						Tipo:      t.Tipo,
						Distancia: t.Distancia,
						Zona:      t.Zona,
						PaceAlvo:  t.PaceAlvo,
						Descricao: t.Descricao,
						Concluido: t.Concluido,
						SemanaIdx: semana.Numero - 1,
						DiaIdx:    t.Dia - 1,
						PlanoID:   pc.ID,
					})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"mes":         mes,
		"ano":         ano,
		"treinos_dia": treinosPorDia,
	})
}
