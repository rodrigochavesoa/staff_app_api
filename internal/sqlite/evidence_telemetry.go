package sqlite

import (
	"context"

	"staff_app/internal/platform/logger"
	"staff_app/internal/services"
)

// EvidencePipelineTelemetryRecorder persists EvidencePipelineEvent rows locally.
type EvidencePipelineTelemetryRecorder struct {
	db *DB
}

func NewEvidencePipelineTelemetryRecorder(db *DB) *EvidencePipelineTelemetryRecorder {
	return &EvidencePipelineTelemetryRecorder{db: db}
}

func (r *EvidencePipelineTelemetryRecorder) Record(ctx context.Context, ev services.EvidencePipelineEvent) error {
	if r == nil || r.db == nil {
		return nil
	}

	logger.Info("training_pipeline_event",
		"aluno_id", ev.AlunoID,
		"endpoint", ev.Endpoint,
		"complexity", ev.Complexity,
		"evidence_requested", ev.EvidenceRequested,
		"evidence_count", ev.EvidenceCount,
		"evidence_fallback_used", ev.EvidenceFallbackUsed,
		"safety_rejected", ev.SafetyRejected,
		"quality_warnings", ev.QualityWarnings,
		"ai_fallback_used", ev.AIFallbackUsed,
		"provider", ev.Provider,
		"duration_ms", ev.DurationMs,
	)

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO training_pipeline_events (
			aluno_id, endpoint, complexity,
			evidence_requested, evidence_count, evidence_fallback_used,
			safety_rejected, quality_warnings, ai_fallback_used,
			provider, duration_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		ev.AlunoID,
		ev.Endpoint,
		ev.Complexity,
		boolToInt(ev.EvidenceRequested),
		ev.EvidenceCount,
		boolToInt(ev.EvidenceFallbackUsed),
		boolToInt(ev.SafetyRejected),
		ev.QualityWarnings,
		boolToInt(ev.AIFallbackUsed),
		ev.Provider,
		ev.DurationMs,
	)
	return err
}
