package services

import (
	"context"
	"strings"
)

// CaseComplexityClassifier estima quanto contexto estruturado o caso de treino
// precisa.
type CaseComplexityClassifier interface {
	Classify(ctx context.Context, in ClassificationInput) ClassificationResult
}

type ClassificationInput struct {
	Context     *AthleteTrainingContext
	Frequencia  int
	Restricoes  string
	Observacoes string
}

// Reasons serve a testes/telemetria; não exposto no HTTP nesta versão.
type ClassificationResult struct {
	Complexity string
	Score      int
	Reasons    []string
}

// LegacyCaseComplexityClassifier é porta 1:1 das regras de
// classifyTrainingComplexity anteriores ao pipeline.
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

// DeterministicCaseComplexityClassifier aplica as regras de pontuação da spec §4.2.
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
