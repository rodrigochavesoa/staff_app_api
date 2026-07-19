package services

import (
	"testing"
)

func TestComputeConfidenceScoreDeterministic(t *testing.T) {
	tests := []struct {
		name    string
		meta    AIMetadata
		avgRel  float64
		want    float64
	}{
		{
			name: "simples with safety and quality",
			meta: AIMetadata{
				Complexity:       "simples",
				SafetyValidated:  true,
				QualityValidated: true,
			},
			want: 0.9, // 0.5+0.2+0.1+0.1
		},
		{
			name: "moderado with high-relevance evidence",
			meta: AIMetadata{
				Complexity:       "moderado",
				EvidenceCount:    1,
				SafetyValidated:  true,
				QualityValidated: true,
			},
			avgRel: 0.8,
			want:   0.9, // 0.5+0.2+0.1+0.1
		},
		{
			name: "complexo zero evidence penalty",
			meta: AIMetadata{
				Complexity:      "complexo",
				EvidenceCount:   0,
				SafetyValidated: true,
			},
			want: 0.45, // 0.5-0.15+0.1
		},
		{
			name: "ai fallback penalty",
			meta: AIMetadata{
				Complexity:       "simples",
				SafetyValidated:  true,
				QualityValidated: true,
				FallbackUsed:     true,
			},
			want: 0.8, // 0.9-0.1
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeConfidenceScore(tt.meta, tt.avgRel)
			if got != tt.want {
				t.Fatalf("ComputeConfidenceScore=%v want %v", got, tt.want)
			}
		})
	}
}

func TestEnrichMetadataSimpleSkipsSearchReason(t *testing.T) {
	meta := AIMetadata{
		SafetyValidated:  true,
		QualityValidated: true,
	}
	EnrichMetadata(t.Context(), &meta, &AthleteTrainingContext{Complexidade: "simples"})

	if meta.EvidenceCount != 0 {
		t.Fatalf("EvidenceCount=%d want 0", meta.EvidenceCount)
	}
	if meta.EvidenceFallback {
		t.Fatal("EvidenceFallback should be false for simples")
	}
	if len(meta.EvidenceReasons) != 1 || meta.EvidenceReasons[0] != "complexidade_simples: busca não acionada" {
		t.Fatalf("EvidenceReasons=%v", meta.EvidenceReasons)
	}
	if meta.ConfidenceScore != 0.9 {
		t.Fatalf("ConfidenceScore=%v want 0.9", meta.ConfidenceScore)
	}
	if len(meta.Validations) != 2 || meta.Validations[0] != "safety:passed" || meta.Validations[1] != "quality:passed" {
		t.Fatalf("Validations=%v", meta.Validations)
	}
}

func TestEnrichMetadataModeradoWithEvidence(t *testing.T) {
	meta := AIMetadata{SafetyValidated: true, QualityValidated: true}
	EnrichMetadata(t.Context(), &meta, &AthleteTrainingContext{
		Complexidade: "moderado",
		Evidencias: []KnowledgeEvidence{{
			Fonte:      "guia-lombalgia",
			Conteudo:   "estabilizacao",
			Relevancia: 0.85,
		}},
	})

	if meta.EvidenceCount != 1 {
		t.Fatalf("EvidenceCount=%d want 1", meta.EvidenceCount)
	}
	if meta.EvidenceFallback {
		t.Fatal("EvidenceFallback should be false when docs found")
	}
	if len(meta.Sources) != 1 || meta.Sources[0] != "guia-lombalgia" {
		t.Fatalf("Sources=%v", meta.Sources)
	}
	if len(meta.EvidenceReasons) != 1 || meta.EvidenceReasons[0] != "busca_acionada: 1 evidencias" {
		t.Fatalf("EvidenceReasons=%v", meta.EvidenceReasons)
	}
	if meta.ConfidenceScore != 0.9 {
		t.Fatalf("ConfidenceScore=%v want 0.9", meta.ConfidenceScore)
	}
}

func TestEnrichMetadataModeradoEmptyEvidenceFallback(t *testing.T) {
	meta := AIMetadata{SafetyValidated: true}
	EnrichMetadata(t.Context(), &meta, &AthleteTrainingContext{Complexidade: "moderado"})

	if !meta.EvidenceFallback {
		t.Fatal("EvidenceFallback should be true when search yields nothing")
	}
	if len(meta.EvidenceReasons) != 1 || meta.EvidenceReasons[0] != "busca_acionada: nenhum documento" {
		t.Fatalf("EvidenceReasons=%v", meta.EvidenceReasons)
	}
}
