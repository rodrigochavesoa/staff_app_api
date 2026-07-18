package http

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"staff_app/internal/domain"
	"staff_app/internal/services"
)

func (h *FichaTreinoHandler) loadTrainingContext(ctx context.Context, alunoID int64, req GerarFichaPeriodizadaRequest) (*services.AthleteTrainingContext, error) {
	result := &services.AthleteTrainingContext{
		Complexidade: "simples",
		DadosUsados:  []string{"aluno", "ficha_local"},
	}

	anamnese, err := h.anamRepo.FindActiveByAlunoID(ctx, alunoID)
	if err != nil {
		return nil, fmt.Errorf("loading active anamnese: %w", err)
	}
	if anamnese != nil {
		result.Anamnese = anamneseHint(anamnese)
		result.DadosUsados = append(result.DadosUsados, "anamnese")
	}

	historico, err := h.loadTrainingHistory(ctx, alunoID)
	if err != nil {
		return nil, err
	}
	if len(historico) > 0 {
		result.Historico = historico
		result.DadosUsados = append(result.DadosUsados, "historico_fichas")
	}

	sved, err := h.loadSVEDSummary(ctx, alunoID)
	if err != nil {
		return nil, err
	}
	if sved != nil {
		result.SVED = sved
		result.DadosUsados = append(result.DadosUsados, "sved")
	}

	evidencias, err := h.searchTrainingEvidence(ctx, req, result.Anamnese)
	if err != nil {
		return nil, err
	}
	if len(evidencias) > 0 {
		result.Evidencias = evidencias
		result.DadosUsados = append(result.DadosUsados, "base_conhecimento")
	}

	result.Complexidade = classifyTrainingComplexity(result, req)
	result.ResumoEstrutural = map[string]interface{}{
		"tem_anamnese":      result.Anamnese != nil,
		"historico_count":   len(result.Historico),
		"evidencias_count":  len(result.Evidencias),
		"frequencia":        req.Frequencia,
		"restricoes_input":  strings.TrimSpace(req.Restricoes) != "",
		"observacoes_input": strings.TrimSpace(req.Observacoes) != "",
	}
	return result, nil
}

func anamneseHint(a *domain.Anamnese) *services.AnamneseTrainingHint {
	return &services.AnamneseTrainingHint{
		StatusAprovacao: a.StatusAprovacao,
		Patologias:      a.Patologias,
		LesoesAtuais:    a.LesoesAtuais,
		DoresCronicas:   a.DoresCronicas,
		Medicamentos:    a.Medicamentos,
		RiskScore:       a.RiskScoreCached,
		Experiencia:     a.ExperienciaTreino,
		Objetivo:        a.ObjetivoPrincipal,
	}
}

func (h *FichaTreinoHandler) loadTrainingHistory(ctx context.Context, alunoID int64) ([]services.TrainingHistoryHint, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT id, COALESCE(tipo_ficha, ''), COALESCE(objetivo, ''), COALESCE(nivel, ''),
		       COALESCE(ies_score, 0), COALESCE(volume_sved, 0), COALESCE(data_criacao, '')
		FROM fichas_treino_web
		WHERE aluno IN (SELECT nome FROM alunos WHERE id = ?)
		ORDER BY id DESC
		LIMIT 3
	`, alunoID)
	if err != nil {
		return nil, fmt.Errorf("loading training history: %w", err)
	}
	defer rows.Close() // #nosec G104

	historico := make([]services.TrainingHistoryHint, 0, 3)
	for rows.Next() {
		var item services.TrainingHistoryHint
		if err := rows.Scan(&item.ID, &item.TipoFicha, &item.Objetivo, &item.Nivel, &item.IesScore, &item.Volume, &item.Data); err != nil {
			return nil, fmt.Errorf("scanning training history: %w", err)
		}
		item.Status = "registrada"
		historico = append(historico, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating training history: %w", err)
	}
	return historico, nil
}

func (h *FichaTreinoHandler) loadSVEDSummary(ctx context.Context, alunoID int64) (*services.SVEDTrainingHint, error) {
	var summary services.SVEDTrainingHint
	err := h.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(ies_score), 0), COALESCE(AVG(densidade), 0),
		       COALESCE(AVG(volume_sved), 0), COUNT(*)
		FROM fichas_treino_web
		WHERE aluno IN (SELECT nome FROM alunos WHERE id = ?)
		  AND COALESCE(volume_sved, 0) > 0
	`, alunoID).Scan(&summary.IesMedio, &summary.DensidadeMedia, &summary.VolumeMedio, &summary.Fichas)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("loading sved summary: %w", err)
	}
	if summary.Fichas == 0 {
		return nil, nil
	}
	return &summary, nil
}

func (h *FichaTreinoHandler) searchTrainingEvidence(ctx context.Context, req GerarFichaPeriodizadaRequest, anam *services.AnamneseTrainingHint) ([]services.KnowledgeEvidence, error) {
	queryParts := []string{req.Restricoes, req.Objetivo, req.Nivel, "musculação"}
	if anam != nil {
		queryParts = append([]string{anam.Patologias, anam.LesoesAtuais, anam.DoresCronicas}, queryParts...)
	}
	candidates := nonEmpty(queryParts)
	candidates = append(candidates, "força", "segurança", "progressão")

	var docs []domain.KnowledgeDocument
	for _, query := range candidates {
		found, err := h.ragRepo.SearchLocalDocuments(ctx, query, "musculacao", 3)
		if err != nil {
			return nil, fmt.Errorf("searching training evidence: %w", err)
		}
		if len(found) > 0 {
			docs = found
			break
		}
	}
	evidencias := make([]services.KnowledgeEvidence, 0, len(docs))
	for _, doc := range docs {
		evidencias = append(evidencias, services.KnowledgeEvidence{
			Fonte:      doc.Fonte,
			Conteudo:   truncateEvidence(doc.Conteudo, 900),
			Tags:       doc.Tags,
			Relevancia: doc.Relevancia,
		})
	}
	return evidencias, nil
}

func classifyTrainingComplexity(ctx *services.AthleteTrainingContext, req GerarFichaPeriodizadaRequest) string {
	score := 0
	if req.Frequencia >= 5 {
		score++
	}
	if strings.TrimSpace(req.Restricoes) != "" || strings.TrimSpace(req.Observacoes) != "" {
		score++
	}
	if ctx.Anamnese != nil {
		if ctx.Anamnese.RiskScore >= 2 {
			score += 2
		}
		if strings.TrimSpace(ctx.Anamnese.Patologias+ctx.Anamnese.LesoesAtuais+ctx.Anamnese.DoresCronicas) != "" {
			score++
		}
	}
	if ctx.SVED != nil && ctx.SVED.IesMedio > 0 {
		score++
	}
	if score >= 4 {
		return "complexo"
	}
	if score >= 2 {
		return "moderado"
	}
	return "simples"
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func truncateEvidence(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}
