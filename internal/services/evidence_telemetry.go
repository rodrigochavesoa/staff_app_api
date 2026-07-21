package services

import (
	"context"
	"strings"
)

// EvidencePipelineEvent é telemetria agregada, sem texto clínico (spec §8.1).
//
// Notas v1:
//   - Emitido só quando TrainingProviderChain roda (trainingAI != nil) e Resolve ok.
//   - Falhas de Resolve e caminho sem IA não gravam linhas (backlog: eventos de erro).
//   - SafetyRejected infere-se de SafetyValidated / FallbackReason com "safety".
type EvidencePipelineEvent struct {
	AlunoID              int64
	Endpoint             string
	Complexity           string
	EvidenceRequested    bool
	EvidenceCount        int
	EvidenceFallbackUsed bool
	SafetyRejected       bool
	QualityWarnings      int
	AIFallbackUsed       bool
	Provider             string
	DurationMs           int64
}

// EvidencePipelineTelemetryRecorder persiste eventos sem texto clínico livre.
type EvidencePipelineTelemetryRecorder interface {
	Record(ctx context.Context, ev EvidencePipelineEvent) error
}

// NoopEvidencePipelineTelemetryRecorder descarta eventos (testes / desligar opcional).
type NoopEvidencePipelineTelemetryRecorder struct{}

func (NoopEvidencePipelineTelemetryRecorder) Record(context.Context, EvidencePipelineEvent) error {
	return nil
}

// NewEvidencePipelineEvent monta o evento a partir do resultado da geração.
// Só IDs, enums e contadores — nunca texto livre de patologia/restrição.
func NewEvidencePipelineEvent(
	alunoID int64,
	endpoint string,
	pipelineCtx *AthleteTrainingContext,
	result *GenerationResult,
	durationMs int64,
) EvidencePipelineEvent {
	ev := EvidencePipelineEvent{
		AlunoID:    alunoID,
		Endpoint:   strings.TrimSpace(endpoint),
		DurationMs: durationMs,
	}
	if ev.Endpoint == "" {
		ev.Endpoint = "gerar-periodizada"
	}

	if pipelineCtx != nil {
		ev.Complexity = strings.TrimSpace(pipelineCtx.Complexidade)
		ev.EvidenceCount = len(pipelineCtx.Evidencias)
	}
	if ev.Complexity == "" && result != nil {
		ev.Complexity = strings.TrimSpace(result.Metadata.Complexity)
		ev.EvidenceCount = result.Metadata.EvidenceCount
	}
	ev.EvidenceRequested = ev.Complexity != "" && !strings.EqualFold(ev.Complexity, "simples")

	if result != nil {
		meta := result.Metadata
		ev.EvidenceFallbackUsed = meta.EvidenceFallback
		ev.AIFallbackUsed = meta.FallbackUsed
		ev.Provider = strings.TrimSpace(meta.Provider)
		ev.QualityWarnings = len(meta.Warnings)
		ev.SafetyRejected = evidenceSafetyRejected(meta)
		if ev.Complexity == "" {
			ev.Complexity = "desconhecido"
		}
	} else if ev.Complexity == "" {
		ev.Complexity = "desconhecido"
	}

	return ev
}

func evidenceSafetyRejected(meta AIMetadata) bool {
	if !meta.SafetyValidated {
		return true
	}
	reason := strings.ToLower(meta.FallbackReason)
	return strings.Contains(reason, "safety")
}
