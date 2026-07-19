package services

import (
	"context"
	"strings"
)

// EvidencePipelineEvent is an aggregated, non-clinical telemetry record (spec §8.1).
//
// v1 notes:
//   - Emitted only when TrainingProviderChain runs (trainingAI != nil) and Resolve succeeds.
//   - Failures of Resolve and the AI-off path do not write rows (backlog: error events).
//   - SafetyRejected is inferred from SafetyValidated / FallbackReason containing "safety".
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

// EvidencePipelineTelemetryRecorder persists pipeline events without clinical free text.
type EvidencePipelineTelemetryRecorder interface {
	Record(ctx context.Context, ev EvidencePipelineEvent) error
}

// NoopEvidencePipelineTelemetryRecorder discards events (tests / optional disable).
type NoopEvidencePipelineTelemetryRecorder struct{}

func (NoopEvidencePipelineTelemetryRecorder) Record(context.Context, EvidencePipelineEvent) error {
	return nil
}

// NewEvidencePipelineEvent builds a telemetry event from generation outcomes.
// Only IDs, enums, and counters — never pathology/restriction free text.
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
