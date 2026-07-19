package services

import "testing"

func TestNewEvidencePipelineEventSimple(t *testing.T) {
	ev := NewEvidencePipelineEvent(7, "gerar-periodizada", &AthleteTrainingContext{
		Complexidade: "simples",
	}, &GenerationResult{
		Metadata: AIMetadata{
			Provider:         "gemini",
			SafetyValidated:  true,
			EvidenceFallback: false,
		},
	}, 42)

	if ev.AlunoID != 7 {
		t.Fatalf("AlunoID=%d", ev.AlunoID)
	}
	if ev.EvidenceRequested {
		t.Fatal("simples must not request evidence")
	}
	if ev.EvidenceCount != 0 {
		t.Fatalf("EvidenceCount=%d", ev.EvidenceCount)
	}
	if ev.SafetyRejected {
		t.Fatal("SafetyRejected should be false")
	}
	if ev.DurationMs != 42 || ev.Provider != "gemini" {
		t.Fatalf("got %+v", ev)
	}
}

func TestNewEvidencePipelineEventModeradoFallback(t *testing.T) {
	ev := NewEvidencePipelineEvent(1, "", &AthleteTrainingContext{
		Complexidade: "moderado",
	}, &GenerationResult{
		Metadata: AIMetadata{
			Provider:         "local",
			FallbackUsed:     true,
			FallbackReason:   "provider gemini rejected by safety: blocked",
			SafetyValidated:  true,
			EvidenceFallback: true,
			Warnings:         []string{"nenhuma evidência local encontrada"},
		},
	}, 10)

	if !ev.EvidenceRequested {
		t.Fatal("moderado should request evidence")
	}
	if !ev.EvidenceFallbackUsed {
		t.Fatal("EvidenceFallbackUsed")
	}
	if !ev.AIFallbackUsed {
		t.Fatal("AIFallbackUsed")
	}
	if !ev.SafetyRejected {
		t.Fatal("SafetyRejected expected from fallback reason")
	}
	if ev.QualityWarnings != 1 {
		t.Fatalf("QualityWarnings=%d", ev.QualityWarnings)
	}
	if ev.Endpoint != "gerar-periodizada" {
		t.Fatalf("Endpoint=%q", ev.Endpoint)
	}
}

func TestNoopEvidencePipelineTelemetryRecorder(t *testing.T) {
	var r EvidencePipelineTelemetryRecorder = NoopEvidencePipelineTelemetryRecorder{}
	if err := r.Record(t.Context(), EvidencePipelineEvent{AlunoID: 1}); err != nil {
		t.Fatalf("noop Record: %v", err)
	}
}
