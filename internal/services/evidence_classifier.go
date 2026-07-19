package services

import (
	"context"
	"strings"
)

// CaseComplexityClassifier scores how much structured context a training
// generation case needs.
type CaseComplexityClassifier interface {
	Classify(ctx context.Context, in ClassificationInput) ClassificationResult
}

// ClassificationInput carries request fields and already-loaded athlete context.
type ClassificationInput struct {
	Context     *AthleteTrainingContext
	Frequencia  int
	Restricoes  string
	Observacoes string
}

// ClassificationResult is the classifier output.
// Reasons is for unit tests / future telemetry; not exposed on HTTP in PR1.
type ClassificationResult struct {
	Complexity string
	Score      int
	Reasons    []string
}

// LegacyCaseComplexityClassifier is a 1:1 port of the pre-pipeline
// classifyTrainingComplexity scoring rules.
type LegacyCaseComplexityClassifier struct{}

func (LegacyCaseComplexityClassifier) Classify(_ context.Context, in ClassificationInput) ClassificationResult {
	score := 0
	reasons := make([]string, 0, 6)

	if in.Frequencia >= 5 {
		score++
		reasons = append(reasons, "frequencia>=5")
	}
	if strings.TrimSpace(in.Restricoes) != "" || strings.TrimSpace(in.Observacoes) != "" {
		score++
		reasons = append(reasons, "restricoes_or_observacoes")
	}
	if in.Context != nil && in.Context.Anamnese != nil {
		if in.Context.Anamnese.RiskScore >= 2 {
			score += 2
			reasons = append(reasons, "anamnese_risk>=2")
		}
		if strings.TrimSpace(in.Context.Anamnese.Patologias+in.Context.Anamnese.LesoesAtuais+in.Context.Anamnese.DoresCronicas) != "" {
			score++
			reasons = append(reasons, "anamnese_clinical_text")
		}
	}
	if in.Context != nil && in.Context.SVED != nil && in.Context.SVED.IesMedio > 0 {
		score++
		reasons = append(reasons, "sved_ies_medio")
	}

	complexity := "simples"
	if score >= 4 {
		complexity = "complexo"
	} else if score >= 2 {
		complexity = "moderado"
	}

	return ClassificationResult{
		Complexity: complexity,
		Score:      score,
		Reasons:    reasons,
	}
}

// DeterministicCaseComplexityClassifier applies spec §4.2 scoring rules.
type DeterministicCaseComplexityClassifier struct{}

func (DeterministicCaseComplexityClassifier) Classify(_ context.Context, in ClassificationInput) ClassificationResult {
	score := 0
	reasons := make([]string, 0, 8)

	if in.Frequencia >= 5 {
		score++
		reasons = append(reasons, "frequencia_alta")
	}
	if strings.TrimSpace(in.Restricoes) != "" || strings.TrimSpace(in.Observacoes) != "" {
		score++
		reasons = append(reasons, "restricoes_ou_observacoes")
	}
	if in.Context != nil && in.Context.Anamnese != nil {
		if in.Context.Anamnese.RiskScore >= 2 {
			score += 2
			reasons = append(reasons, "anamnese_risk_score")
		}
		if strings.TrimSpace(in.Context.Anamnese.Patologias+in.Context.Anamnese.LesoesAtuais+in.Context.Anamnese.DoresCronicas) != "" {
			score++
			reasons = append(reasons, "anamnese_clinica")
		}
		if in.Context.Anamnese.RiskScore >= 3 {
			score++
			reasons = append(reasons, "anamnese_risco_alto")
		}
	}
	if in.Context != nil && len(in.Context.Historico) > 0 {
		score++
		reasons = append(reasons, "historico_fichas")
	}
	if in.Context != nil && in.Context.SVED != nil && in.Context.SVED.IesMedio > 0 {
		score++
		reasons = append(reasons, "sved_disponivel")
	}

	complexity := "simples"
	switch {
	case score >= 4:
		complexity = "complexo"
	case score >= 2:
		complexity = "moderado"
	}

	return ClassificationResult{
		Complexity: complexity,
		Score:      score,
		Reasons:    reasons,
	}
}
