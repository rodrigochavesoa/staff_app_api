package services

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type SVEDHistoricoItem struct {
	FichaID     int64   `json:"ficha_id"`
	Data        string  `json:"data"`
	Series      int     `json:"series"`
	Reps        int     `json:"reps"`
	RIR         int     `json:"rir"`
	Cadencia    string  `json:"cadencia"`
	RestSeconds int     `json:"rest_seconds"`
	TutTotal    int     `json:"tut_total"`
	Densidade   float64 `json:"densidade"`
	IesScore    float64 `json:"ies_score"`
}

type ExercicioJSON struct {
	Ordem         int    `json:"ordem,omitempty"`
	Nome          string `json:"nome"`
	GrupoMuscular string `json:"grupo_muscular"`
	Bloco         string `json:"bloco,omitempty"`
	Series        any    `json:"series"`
	Repeticoes    any    `json:"repeticoes"`
	Carga         string `json:"carga,omitempty"`
	Descanso      any    `json:"descanso"`
	Observacoes   string `json:"observacoes,omitempty"`
	Cadencia      string `json:"cadencia,omitempty"`
	RIR           any    `json:"rir,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
}

type SVEDMetricas struct {
	TutMedioS       int     `json:"tut_medio_s"`
	DensidadeMedia  float64 `json:"densidade_media"`
	IesMedioJoules  float64 `json:"ies_medio_joules"`
	VolumeSved      int     `json:"volume_sved"`
	TotalExercicios int     `json:"total_exercicios"`
	TutTotal        int     `json:"tut_total"`
}

func ParseInt(val any, defaultVal int) int {
	if val == nil {
		return defaultVal
	}
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		parts := strings.Split(v, "-")
		firstPart := strings.Split(parts[0], "x")[0]
		firstPart = strings.TrimSpace(firstPart)
		reg := regexp.MustCompile(`[^0-9]`)
		clean := reg.ReplaceAllString(firstPart, "")
		if clean == "" {
			return defaultVal
		}
		if res, err := strconv.Atoi(clean); err == nil {
			return res
		}
	}
	return defaultVal
}

func ParseDescanso(val any) int {
	if val == nil {
		return 60
	}
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		vLower := strings.ToLower(v)
		parts := strings.Split(vLower, "-")
		firstPart := strings.Split(parts[0], " ")[0]
		firstPart = strings.TrimSpace(firstPart)
		reg := regexp.MustCompile(`[^0-9]`)
		numPart := reg.ReplaceAllString(firstPart, "")
		if numPart == "" {
			return 60
		}
		num, err := strconv.Atoi(numPart)
		if err != nil {
			return 60
		}
		if strings.Contains(vLower, "min") {
			return num * 60
		}
		return num
	}
	return 60
}

func ParseCadencia(cadenciaStr string) map[string]int {
	if len(cadenciaStr) != 4 {
		cadenciaStr = "4010"
	}
	res := make(map[string]int)
	eccentric, err1 := strconv.Atoi(string(cadenciaStr[0]))
	pause, err2 := strconv.Atoi(string(cadenciaStr[1]))
	concentric, err3 := strconv.Atoi(string(cadenciaStr[2]))
	isometric, err4 := strconv.Atoi(string(cadenciaStr[3]))

	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return map[string]int{
			"excentrica":  4,
			"pausa":       0,
			"concentrica": 1,
			"pico":        0,
		}
	}

	res["excentrica"] = eccentric
	res["pausa"] = pause
	res["concentrica"] = concentric
	res["pico"] = isometric
	return res
}

func CalcularDensidade(tutTotal float64, restSeconds int) float64 {
	if restSeconds <= 0 {
		return 0.0
	}
	val := tutTotal / float64(restSeconds)
	return math.Round(val*1000) / 1000
}

func CalcularIES(series int, reps int, cadenciaStr string, restSeconds int, rir int) float64 {
	if restSeconds <= 0 {
		return 0.0
	}
	cad := ParseCadencia(cadenciaStr)
	tempoPorRep := cad["excentrica"] + cad["pausa"] + cad["concentrica"] + cad["pico"]
	
	var tutTotal float64
	if tempoPorRep > 0 {
		tutTotal = float64(series * reps * tempoPorRep)
	} else {
		tutTotal = float64(series * reps * 4) // fallback
	}

	densidade := tutTotal / float64(restSeconds)
	ies := densidade * float64(10-rir) * 2.5

	if ies > 100.0 {
		ies = 100.0
	} else if ies < 0.0 {
		ies = 0.0
	}
	return math.Round(ies*10) / 10
}

func CalcularMetricasSVED(exercicios []ExercicioJSON) SVEDMetricas {
	if len(exercicios) == 0 {
		return SVEDMetricas{}
	}

	var totalTut float64
	var totalDensidade float64
	var totalIes float64
	var exerciciosValidos int

	var exerciciosPrincipais []ExercicioJSON
	for _, ex := range exercicios {
		bloco := "principal"
		if ex.Bloco != "" {
			bloco = ex.Bloco
		}
		if bloco == "principal" {
			exerciciosPrincipais = append(exerciciosPrincipais, ex)
		}
	}
	if len(exerciciosPrincipais) == 0 {
		exerciciosPrincipais = exercicios
	}

	for _, ex := range exerciciosPrincipais {
		series := ParseInt(ex.Series, 1)
		reps := ParseInt(ex.Repeticoes, 10)
		cadencia := ex.Cadencia
		if cadencia == "" {
			cadencia = "4010"
		}
		restSeconds := ParseDescanso(ex.Descanso)
		rir := ParseInt(ex.RIR, 2)

		var tutTotalEx float64
		if len(cadencia) == 4 {
			cadVals := ParseCadencia(cadencia)
			tempoPorRep := cadVals["excentrica"] + cadVals["pausa"] + cadVals["concentrica"] + cadVals["pico"]
			tutPorSerie := float64(reps * tempoPorRep)
			tutTotalEx = tutPorSerie * float64(series)
		} else {
			tutTotalEx = float64(reps * series * 4)
		}

		var densidade float64
		if restSeconds > 0 {
			densidade = tutTotalEx / float64(restSeconds)
		} else {
			densidade = tutTotalEx / 60.0
		}

		var ies float64
		if rir < 10 {
			ies = CalcularIES(series, reps, cadencia, restSeconds, rir)
		}

		totalTut += tutTotalEx
		totalDensidade += densidade
		totalIes += ies
		exerciciosValidos++
	}

	if exerciciosValidos == 0 {
		return SVEDMetricas{}
	}

	return SVEDMetricas{
		TutMedioS:       int(math.Round(totalTut / float64(exerciciosValidos))),
		DensidadeMedia:  math.Round((totalDensidade/float64(exerciciosValidos))*100) / 100,
		IesMedioJoules:  math.Round((totalIes/float64(exerciciosValidos))*10) / 10,
		VolumeSved:      int(math.Round(totalTut * 0.85)),
		TotalExercicios: exerciciosValidos,
		TutTotal:        int(totalTut),
	}
}

func SugerirProgressaoSVED(exercicioNome string, historico []SVEDHistoricoItem) map[string]any {
	if len(historico) < 2 {
		return map[string]any{
			"tipo":      "manter",
			"mensagem":  "Continue com os mesmos parâmetros por mais 1-2 semanas",
			"prioridade": "baixa",
		}
	}

	ultimo := historico[0]

	if ultimo.RIR == 0 {
		return map[string]any{
			"tipo":       "aumentar_reps",
			"mensagem":   "RIR 0 alcançado! Aumente para " + strconv.Itoa(ultimo.Reps+1) + "-" + strconv.Itoa(ultimo.Reps+2) + " reps e volte RIR para 2",
			"parametros": map[string]any{"reps": ultimo.Reps + 2, "rir": 2},
			"prioridade": "alta",
		}
	} else if ultimo.RIR >= 3 {
		return map[string]any{
			"tipo":       "reduzir_rir",
			"mensagem":   "Reduza RIR de " + strconv.Itoa(ultimo.RIR) + " para " + strconv.Itoa(ultimo.RIR-1) + " (chegar mais perto da falha)",
			"parametros": map[string]any{"rir": ultimo.RIR - 1},
			"prioridade": "media",
		}
	} else if ultimo.Densidade < 0.5 && ultimo.RestSeconds > 45 {
		novoRest := ultimo.RestSeconds - 10
		if novoRest < 30 {
			novoRest = 30
		}
		densStr := strconv.FormatFloat(ultimo.Densidade, 'f', 2, 64)
		return map[string]any{
			"tipo":       "aumentar_densidade",
			"mensagem":   "Densidade baixa (" + densStr + "). Reduza rest de " + strconv.Itoa(ultimo.RestSeconds) + "s para " + strconv.Itoa(novoRest) + "s",
			"parametros": map[string]any{"rest_seconds": novoRest},
			"prioridade": "media",
		}
	} else if ultimo.Cadencia == "4010" {
		return map[string]any{
			"tipo":       "aumentar_tut",
			"mensagem":   "Tente cadência 4012 (adicionar 2s de contração no pico)",
			"parametros": map[string]any{"cadencia": "4012"},
			"prioridade": "baixa",
		}
	}

	return map[string]any{
		"tipo":      "manter",
		"mensagem":  "Progressão adequada! Mantenha parâmetros por mais 1 semana",
		"prioridade": "baixa",
	}
}
