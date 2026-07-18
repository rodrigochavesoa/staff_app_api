package daniels

import (
	"sort"
)

// ZoneDetails describes a training zone with its name, target pace, and description.
type ZoneDetails struct {
	Nome      string `json:"nome"`
	PaceAlvo  int    `json:"pace_alvo"`
	Descricao string `json:"descricao"`
}

// danielsTable maps VDOT keys to their target paces (seconds/km) for different zones.
var danielsTable = map[float64]map[string]int{
	30: {"E": 362, "M": 340, "T": 310, "I": 285, "R": 268},
	35: {"E": 332, "M": 312, "T": 286, "I": 262, "R": 246},
	40: {"E": 307, "M": 288, "T": 262, "I": 240, "R": 225},
	45: {"E": 287, "M": 268, "T": 243, "I": 222, "R": 208},
	50: {"E": 270, "M": 252, "T": 228, "I": 208, "R": 194},
	55: {"E": 255, "M": 238, "T": 215, "I": 196, "R": 183},
	60: {"E": 242, "M": 225, "T": 203, "I": 185, "R": 172},
	65: {"E": 230, "M": 214, "T": 193, "I": 175, "R": 163},
	70: {"E": 220, "M": 204, "T": 184, "I": 167, "R": 155},
}

// CalculateZones returns a map of training zones based on VDOT using linear interpolation.
func CalculateZones(vdot float64) map[string]ZoneDetails {
	// Clamp VDOT to valid range
	if vdot < 20.0 {
		vdot = 20.0
	} else if vdot > 85.0 {
		vdot = 85.0
	}

	interpolate := func(vdotVal float64, zoneKey string) int {
		// Get sorted known VDOTs
		vdots := make([]float64, 0, len(danielsTable))
		for k := range danielsTable {
			vdots = append(vdots, k)
		}
		sort.Float64s(vdots)

		// Edge cases
		if vdotVal <= vdots[0] {
			return danielsTable[vdots[0]][zoneKey]
		}
		if vdotVal >= vdots[len(vdots)-1] {
			return danielsTable[vdots[len(vdots)-1]][zoneKey]
		}

		// Interpolation
		for i := 0; i < len(vdots)-1; i++ {
			v1 := vdots[i]
			v2 := vdots[i+1]

			if vdotVal >= v1 && vdotVal <= v2 {
				pace1 := danielsTable[v1][zoneKey]
				pace2 := danielsTable[v2][zoneKey]
				fator := (vdotVal - v1) / (v2 - v1)
				return int(float64(pace1) + float64(pace2-pace1)*fator)
			}
		}

		return danielsTable[40][zoneKey] // Fallback
	}

	return map[string]ZoneDetails{
		"E": {
			Nome:      "Easy (Fácil)",
			PaceAlvo:  interpolate(vdot, "E"),
			Descricao: "Corrida confortável, conversação possível",
		},
		"M": {
			Nome:      "Marathon (Maratona)",
			PaceAlvo:  interpolate(vdot, "M"),
			Descricao: "Ritmo de prova longa",
		},
		"T": {
			Nome:      "Threshold (Limiar)",
			PaceAlvo:  interpolate(vdot, "T"),
			Descricao: "Ritmo forte mas sustentável",
		},
		"I": {
			Nome:      "Interval (Intervalado)",
			PaceAlvo:  interpolate(vdot, "I"),
			Descricao: "Tiros 3-5min",
		},
		"R": {
			Nome:      "Repetition (Tiros)",
			PaceAlvo:  interpolate(vdot, "R"),
			Descricao: "Tiros curtos máxima velocidade",
		},
	}
}
