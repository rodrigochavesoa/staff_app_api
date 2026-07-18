package blocos

import (
	"math"
	"time"

	"staff_app/internal/domain"
)

// HistoricoStats aggregates running history for an athlete.
type HistoricoStats struct {
	TemHistorico          bool     `json:"tem_historico"`
	AlunoID               int64    `json:"aluno_id"`
	Dias                  int      `json:"dias"`
	VolumeMedioSemanalKM  float64  `json:"volume_medio_semanal_km"`
	TreinosPorSemana      float64  `json:"treinos_por_semana"`
	TaxaConclusao         float64  `json:"taxa_conclusao"`
	IntensidadePreferida  string   `json:"intensidade_preferida"`
	Fontes                []string `json:"fontes"`
}

// GarminSample is a minimal activity sample for stats.
type GarminSample struct {
	StartTime      time.Time
	DistanceMeters float64
}

// PlanoConclusao captures concluded vs total sessions in plan JSON.
type PlanoConclusao struct {
	Total     int
	Concluidos int
	Zonas     map[string]int
}

// ComputeHistoricoStats merges Garmin and plan completion signals.
func ComputeHistoricoStats(alunoID int64, dias int, garmin []GarminSample, planos []PlanoConclusao, since time.Time) HistoricoStats {
	if dias < 1 {
		dias = 30
	}
	stats := HistoricoStats{
		AlunoID:              alunoID,
		Dias:                 dias,
		IntensidadePreferida: "E",
		Fontes:               []string{},
	}

	var garminKM float64
	garminCount := 0
	for _, g := range garmin {
		if g.StartTime.Before(since) {
			continue
		}
		garminCount++
		garminKM += g.DistanceMeters / 1000
	}

	totalSessoes := 0
	concluidas := 0
	zonaCounts := map[string]int{}
	for _, p := range planos {
		totalSessoes += p.Total
		concluidas += p.Concluidos
		for z, c := range p.Zonas {
			zonaCounts[z] += c
		}
	}

	fontes := make([]string, 0, 2)
	weeks := float64(dias) / 7.0
	if weeks < 1 {
		weeks = 1
	}

	if garminCount > 0 {
		fontes = append(fontes, "garmin")
		stats.VolumeMedioSemanalKM = round2(garminKM / weeks)
		stats.TreinosPorSemana = round2(float64(garminCount) / weeks)
	}
	if totalSessoes > 0 {
		fontes = append(fontes, "plano_json")
		stats.TaxaConclusao = round2(float64(concluidas) / float64(totalSessoes))
		if garminCount == 0 {
			stats.TreinosPorSemana = round2(float64(concluidas) / weeks)
		}
	}

	bestZona := "E"
	bestCount := -1
	for z, c := range zonaCounts {
		if c > bestCount {
			bestCount = c
			bestZona = z
		}
	}
	if bestCount >= 0 {
		stats.IntensidadePreferida = bestZona
	}

	stats.Fontes = fontes
	stats.TemHistorico = len(fontes) > 0
	return stats
}

// SummarizePlanoConclusao extracts completion stats from a detailed plan.
func SummarizePlanoConclusao(pd domain.PlanoDetalhado) PlanoConclusao {
	out := PlanoConclusao{Zonas: map[string]int{}}
	for _, s := range pd.Semanas {
		for _, t := range s.Treinos {
			out.Total++
			if t.Concluido {
				out.Concluidos++
			}
			zona := t.Zona
			if zona == "" {
				zona = t.ZonaPrincipal
			}
			if zona == "" {
				zona = "E"
			}
			out.Zonas[zona]++
		}
	}
	return out
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
