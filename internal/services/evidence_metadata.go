package services

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// EnrichMetadata fills evidence-related AIMetadata fields from the pipeline result (spec §7).
// Safe with nil meta or nil pipelineResult (only validations/confidence from meta flags).
func EnrichMetadata(_ context.Context, meta *AIMetadata, pipelineResult *AthleteTrainingContext) {
	if meta == nil {
		return
	}

	avgRel := 0.0
	if pipelineResult != nil {
		meta.ContextUsed = true
		meta.Complexity = pipelineResult.Complexidade
		meta.EvidenceCount = len(pipelineResult.Evidencias)
		meta.Sources = uniqueEvidenceSources(pipelineResult.Evidencias)
		avgRel = averageEvidenceRelevance(pipelineResult.Evidencias)

		complexity := strings.ToLower(strings.TrimSpace(pipelineResult.Complexidade))
		switch {
		case complexity == "simples":
			meta.EvidenceFallback = false
			meta.EvidenceReasons = []string{"complexidade_simples: busca não acionada"}
		case complexity == "moderado" || complexity == "complexo":
			if len(pipelineResult.Evidencias) == 0 {
				meta.EvidenceFallback = true
				meta.EvidenceReasons = []string{"busca_acionada: nenhum documento"}
				meta.Warnings = appendUniqueWarning(meta.Warnings, "nenhuma evidência local encontrada")
			} else {
				meta.EvidenceFallback = false
				meta.EvidenceReasons = []string{
					fmt.Sprintf("busca_acionada: %d evidencias", len(pipelineResult.Evidencias)),
				}
			}
		}
	}

	meta.Validations = buildValidations(*meta)
	meta.ConfidenceScore = ComputeConfidenceScore(*meta, avgRel)
}

// ComputeConfidenceScore applies the deterministic v1 rules from spec §7.2.
func ComputeConfidenceScore(meta AIMetadata, avgEvidenceRelevance float64) float64 {
	score := 0.5
	complexity := strings.ToLower(strings.TrimSpace(meta.Complexity))

	if complexity == "simples" {
		score += 0.2
	}
	if meta.EvidenceCount >= 1 && avgEvidenceRelevance >= 0.7 {
		score += 0.2
	}
	if meta.EvidenceCount == 0 && (complexity == "moderado" || complexity == "complexo") {
		score -= 0.15
	}
	if meta.SafetyValidated {
		score += 0.1
	}
	if meta.QualityValidated {
		score += 0.1
	}
	if meta.FallbackUsed {
		score -= 0.1
	}

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return math.Round(score*100) / 100
}

func uniqueEvidenceSources(evidencias []KnowledgeEvidence) []string {
	if len(evidencias) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(evidencias))
	out := make([]string, 0, len(evidencias))
	for _, ev := range evidencias {
		fonte := strings.TrimSpace(ev.Fonte)
		if fonte == "" {
			continue
		}
		if _, ok := seen[fonte]; ok {
			continue
		}
		seen[fonte] = struct{}{}
		out = append(out, fonte)
	}
	return out
}

func averageEvidenceRelevance(evidencias []KnowledgeEvidence) float64 {
	if len(evidencias) == 0 {
		return 0
	}
	var sum float64
	for _, ev := range evidencias {
		sum += ev.Relevancia
	}
	return sum / float64(len(evidencias))
}

func buildValidations(meta AIMetadata) []string {
	out := make([]string, 0, 2)
	if meta.SafetyValidated {
		out = append(out, "safety:passed")
	} else {
		out = append(out, "safety:rejected")
	}
	if meta.QualityValidated {
		out = append(out, "quality:passed")
	} else if len(meta.Warnings) > 0 {
		out = append(out, "quality:warnings")
	}
	return out
}

func appendUniqueWarning(warnings []string, msg string) []string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return warnings
	}
	for _, w := range warnings {
		if w == msg {
			return warnings
		}
	}
	return append(warnings, msg)
}
