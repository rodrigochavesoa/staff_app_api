package http

import (
	"context"

	"staff_app/internal/platform/logger"
	"staff_app/internal/services"
)

func (h *FichaTreinoHandler) loadTrainingContext(ctx context.Context, alunoID int64, req GerarFichaPeriodizadaRequest) (*services.AthleteTrainingContext, error) {
	pipeline := h.evidencePipeline
	if pipeline == nil {
		pipeline = services.NewEvidencePipeline(h.db, h.anamRepo, h.ragRepo)
	}
	return pipeline.Build(ctx, alunoID, services.GenerationRequest{
		Frequencia:  req.Frequencia,
		Objetivo:    req.Objetivo,
		Nivel:       req.Nivel,
		Restricoes:  req.Restricoes,
		Observacoes: req.Observacoes,
	})
}

func (h *FichaTreinoHandler) recordEvidencePipelineTelemetry(
	ctx context.Context,
	alunoID int64,
	pipelineCtx *services.AthleteTrainingContext,
	result *services.GenerationResult,
	durationMs int64,
) {
	if h == nil || h.evidenceTelemetry == nil || result == nil {
		return
	}
	ev := services.NewEvidencePipelineEvent(alunoID, "gerar-periodizada", pipelineCtx, result, durationMs)
	if err := h.evidenceTelemetry.Record(ctx, ev); err != nil {
		logger.Warn("failed to record training pipeline telemetry", "error", err.Error(), "aluno_id", alunoID)
	}
}
