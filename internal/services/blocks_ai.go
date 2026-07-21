package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"staff_app/internal/corrida/blocos"
	"staff_app/internal/domain"
)

type BlocksEnrichRequest struct {
	Days           []blocos.PreviewDay
	VDOT           float64
	DistanciaProva string
	Nivel          string
	Objetivo       string
	Limitacoes     string
	HighCardioRisk bool
}

type BlocksEnrichResult struct {
	Days     []blocos.PreviewDay
	Warnings []string
}

// BlocksAIProvider enriquece opcionalmente prévias de blocos de corrida.
// Padrão de produção: LocalBlocksAIProvider (sem chaves de API). Provedores externos
// podem ser injetados em testes ou homologação autorizada.
type BlocksAIProvider interface {
	Name() string
	Model() string
	Available() bool
	Enrich(ctx context.Context, req *BlocksEnrichRequest) (*BlocksEnrichResult, error)
}

// LocalBlocksAIProvider acrescenta notas determinísticas sem chamar APIs remotas.
type LocalBlocksAIProvider struct{}

func (LocalBlocksAIProvider) Name() string  { return "local" }
func (LocalBlocksAIProvider) Model() string { return "local-blocks-enricher" }
func (LocalBlocksAIProvider) Available() bool {
	return true
}

func (LocalBlocksAIProvider) Enrich(_ context.Context, req *BlocksEnrichRequest) (*BlocksEnrichResult, error) {
	if req == nil {
		return nil, errors.New("blocks enrich request is required")
	}
	if len(req.Days) == 0 {
		return nil, errors.New("preview days are required")
	}

	objetivo := strings.TrimSpace(strings.ToLower(req.Objetivo))
	if objetivo == "" {
		objetivo = "performance"
	}

	out := make([]blocos.PreviewDay, len(req.Days))
	warnings := make([]string, 0)
	for i, day := range req.Days {
		cp := day
		cp.Blocos = enrichBlocksNotes(day.Blocos, objetivo, req.HighCardioRisk)
		if req.HighCardioRisk {
			cp.Blocos = blocos.ApplyPaces(blocos.DowngradeHardIntensities(cp.Blocos), req.VDOT)
			cp.DuracaoMinutos = blocos.DurationMinutes(cp.Blocos)
		}
		out[i] = cp
	}
	if req.HighCardioRisk {
		warnings = append(warnings, "intensidades I/R reduzidas por risco cardiorrespiratório (enricher local)")
	}

	return &BlocksEnrichResult{Days: out, Warnings: warnings}, nil
}

func enrichBlocksNotes(in []domain.BlocoCorrida, objetivo string, highRisk bool) []domain.BlocoCorrida {
	out := make([]domain.BlocoCorrida, len(in))
	for i, b := range in {
		cp := b
		if cp.Type == "atomic" {
			note := intensityNote(cp.Intensity, objetivo, highRisk)
			if note != "" {
				if strings.TrimSpace(cp.Notas) == "" {
					cp.Notas = note
				} else if !strings.Contains(cp.Notas, note) {
					cp.Notas = cp.Notas + " | " + note
				}
			}
		}
		if len(cp.Content) > 0 {
			cp.Content = enrichBlocksNotes(cp.Content, objetivo, highRisk)
		}
		out[i] = cp
	}
	return out
}

func intensityNote(intensity, objetivo string, highRisk bool) string {
	if highRisk && (intensity == "I" || intensity == "R") {
		return "evitar pico de intensidade sob risco clínico"
	}
	switch intensity {
	case "E":
		return "manter conversação confortável (zona E)"
	case "M":
		return "ritmo maratona controlado (zona M)"
	case "T":
		if objetivo == "saude" || objetivo == "saúde" {
			return "limiar suave; priorizar técnica"
		}
		return "limiar sustentado (zona T)"
	case "I":
		return "intervalos curtos com recuperação completa (zona I)"
	case "R":
		return "repetições rápidas; técnica antes de volume (zona R)"
	case "Rest":
		return "recuperação ativa ou parado conforme prescrito"
	default:
		return ""
	}
}

// ValidateBlocksSafety rejeita enriquecimento que reintroduz intervalos duros com alto risco cardio.
func ValidateBlocksSafety(days []blocos.PreviewDay, highCardioRisk bool) error {
	if !highCardioRisk {
		return nil
	}
	for _, day := range days {
		if hasHardIntensity(day.Blocos) {
			return fmt.Errorf("blocos com intensidade I/R rejeitados sob risco cardiorrespiratório alto")
		}
	}
	return nil
}

func hasHardIntensity(in []domain.BlocoCorrida) bool {
	for _, b := range in {
		if b.Type == "atomic" && (b.Intensity == "I" || b.Intensity == "R") {
			return true
		}
		if hasHardIntensity(b.Content) {
			return true
		}
	}
	return false
}

// HighCardioRiskFromText indica se o texto livre de limitações implica alto risco cardio.
func HighCardioRiskFromText(limitacoes string) bool {
	text := strings.ToLower(strings.TrimSpace(limitacoes))
	if text == "" {
		return false
	}
	triggers := []string{
		"cardiopatia", "cardíaco", "cardiaco", "hipertensão", "hipertensao",
		"arritmia", "dor no peito", "dispneia", "risco cardiorrespirat",
	}
	return containsAny(text, triggers)
}
