package http

import (
	"context"

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
