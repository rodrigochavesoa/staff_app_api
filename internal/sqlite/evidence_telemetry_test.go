package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"staff_app/internal/platform/logger"
	"staff_app/internal/services"
)

func TestEvidencePipelineTelemetryRecorderInsert(t *testing.T) {
	logger.Setup("development", false)
	tempDir, err := os.MkdirTemp("", "sqlite-evidence-telemetry-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	db, err := Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rec := NewEvidencePipelineTelemetryRecorder(db)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ev := services.EvidencePipelineEvent{
		AlunoID:              9,
		Endpoint:             "gerar-periodizada",
		Complexity:           "complexo",
		EvidenceRequested:    true,
		EvidenceCount:        0,
		EvidenceFallbackUsed: true,
		SafetyRejected:       false,
		QualityWarnings:      2,
		AIFallbackUsed:       true,
		Provider:             "local",
		DurationMs:           123,
	}
	if err := rec.Record(ctx, ev); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var (
		alunoID, evidenceRequested, evidenceCount, evidenceFallback int
		safetyRejected, qualityWarnings, aiFallback                 int
		endpoint, complexity, provider                              string
		durationMs                                                  int64
	)
	err = db.QueryRowContext(ctx, `
		SELECT aluno_id, endpoint, complexity,
		       evidence_requested, evidence_count, evidence_fallback_used,
		       safety_rejected, quality_warnings, ai_fallback_used,
		       provider, duration_ms
		FROM training_pipeline_events
		ORDER BY id DESC LIMIT 1
	`).Scan(
		&alunoID, &endpoint, &complexity,
		&evidenceRequested, &evidenceCount, &evidenceFallback,
		&safetyRejected, &qualityWarnings, &aiFallback,
		&provider, &durationMs,
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if alunoID != 9 || endpoint != "gerar-periodizada" || complexity != "complexo" {
		t.Fatalf("row identity mismatch: %d %s %s", alunoID, endpoint, complexity)
	}
	if evidenceRequested != 1 || evidenceCount != 0 || evidenceFallback != 1 {
		t.Fatalf("evidence fields: req=%d count=%d fallback=%d", evidenceRequested, evidenceCount, evidenceFallback)
	}
	if safetyRejected != 0 || qualityWarnings != 2 || aiFallback != 1 {
		t.Fatalf("flags mismatch")
	}
	if provider != "local" || durationMs != 123 {
		t.Fatalf("provider/duration: %s %d", provider, durationMs)
	}
}

func TestTrainingPipelineEventsHasNoClinicalColumns(t *testing.T) {
	logger.Setup("development", false)
	tempDir := t.TempDir()
	db, err := Connect(filepath.Join(tempDir, "schema.db"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.Query(`PRAGMA table_info(training_pipeline_events)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()

	forbidden := map[string]bool{
		"patologias": true, "restricoes": true, "query": true,
		"conteudo": true, "observacoes": true, "lesoes": true,
	}
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		cols = append(cols, name)
		if forbidden[name] {
			t.Fatalf("clinical/free-text column not allowed: %s", name)
		}
	}
	if len(cols) == 0 {
		t.Fatal("expected training_pipeline_events columns")
	}
}
