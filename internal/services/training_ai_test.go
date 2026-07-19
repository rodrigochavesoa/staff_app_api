package services

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"staff_app/internal/domain"
)

type fakeTrainingProvider struct {
	name        string
	model       string
	raw         string
	fixturePath string
	err         error
}

func (p fakeTrainingProvider) Name() string  { return p.name }
func (p fakeTrainingProvider) Model() string { return p.model }
func (p fakeTrainingProvider) Generate(context.Context, *GenerationRequest) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	if p.fixturePath != "" {
		data, err := os.ReadFile(p.fixturePath)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return p.raw, nil
}

func TestTrainingProviderChainFallbackToSecondProvider(t *testing.T) {
	req := testGenerationRequest("sem restrições")
	chain := NewTrainingProviderChain(
		AITrainingModeAssistive,
		0,
		[]TrainingProvider{
			fakeTrainingProvider{name: "gemini", model: "fake-gemini", err: errors.New("timeout")},
			fakeTrainingProvider{name: "openai", model: "fake-openai", raw: validTrainingJSON("Supino Reto")},
		},
		nil,
		nil,
		nil,
	)

	result, err := chain.Resolve(t.Context(), req)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if !result.Metadata.AIUsed {
		t.Fatal("expected AI provider to be used")
	}
	if result.Metadata.Provider != "openai" {
		t.Fatalf("expected openai provider, got %s", result.Metadata.Provider)
	}
	if result.Metadata.FallbackUsed {
		t.Fatal("fallback_used should be false when second AI provider succeeds")
	}
	if _, ok := result.Ficha["ai_metadata"]; !ok {
		t.Fatal("expected ai_metadata in generated ficha")
	}
}

func TestTrainingProviderChainAcceptsAnonymizedFixture(t *testing.T) {
	req := testGenerationRequest("sem restrições")
	req.Contexto = testTrainingContext()
	chain := NewTrainingProviderChain(
		AITrainingModeAssistive,
		0,
		[]TrainingProvider{
			fakeTrainingProvider{
				name:        "gemini",
				model:       "fixture-gemini",
				fixturePath: "testdata/ai/gemini_periodized_good.json",
			},
		},
		nil,
		nil,
		nil,
	)

	result, err := chain.Resolve(t.Context(), req)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if !result.Metadata.AIUsed {
		t.Fatal("expected fixture provider to be accepted")
	}
	if !result.Metadata.ContextUsed {
		t.Fatal("expected context_used=true when context is provided")
	}
	if result.Metadata.EvidenceCount != 1 {
		t.Fatalf("expected evidence_count=1, got %d", result.Metadata.EvidenceCount)
	}
	if result.Metadata.Complexity != "moderado" {
		t.Fatalf("expected complexity moderado, got %q", result.Metadata.Complexity)
	}
	if len(result.Metadata.Sources) != 1 || result.Metadata.Sources[0] != "Fixture" {
		t.Fatalf("expected sources=[Fixture], got %v", result.Metadata.Sources)
	}
	if result.Metadata.EvidenceFallback {
		t.Fatal("evidence_fallback_used should be false when evidence present")
	}
	if result.Metadata.ConfidenceScore <= 0 {
		t.Fatalf("expected confidence_score > 0, got %v", result.Metadata.ConfidenceScore)
	}
	if result.Ficha["tipo"] != "periodizada" {
		t.Fatalf("expected periodizada ficha, got %+v", result.Ficha["tipo"])
	}
}

func TestTrainingProviderChainSafetyFallbackToLocal(t *testing.T) {
	req := testGenerationRequest("dor lombar e hérnia de disco")
	chain := NewTrainingProviderChain(
		AITrainingModeAssistive,
		0,
		[]TrainingProvider{
			fakeTrainingProvider{name: "gemini", model: "fake-gemini", fixturePath: "testdata/ai/gemini_periodized_lumbar_unsafe.json"},
			LocalTrainingProvider{},
		},
		nil,
		nil,
		nil,
	)

	result, err := chain.Resolve(t.Context(), req)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if result.Metadata.AIUsed {
		t.Fatal("expected local deterministic generation after safety rejection")
	}
	if result.Metadata.Provider != "local" {
		t.Fatalf("expected local provider, got %s", result.Metadata.Provider)
	}
	if !result.Metadata.FallbackUsed {
		t.Fatal("expected fallback_used after safety rejection")
	}
	if !strings.Contains(result.Metadata.FallbackReason, "restrição lombar") {
		t.Fatalf("expected safety reason in fallback_reason, got %q", result.Metadata.FallbackReason)
	}
}

func TestTrainingProviderChainClinicalSafetyRulesFallbackToLocal(t *testing.T) {
	tests := []struct {
		name          string
		req           *GenerationRequest
		fixture       string
		wantReason    string
		contextRisk   float64
		contextFields *AnamneseTrainingHint
	}{
		{
			name:       "knee restriction",
			req:        testGenerationRequest("condromalácia patelar no joelho direito"),
			fixture:    "testdata/ai/gemini_periodized_knee_unsafe.json",
			wantReason: "restrição de joelho",
		},
		{
			name:    "shoulder restriction from anamnese",
			req:     testGenerationRequest("sem restrições livres"),
			fixture: "testdata/ai/gemini_periodized_shoulder_unsafe.json",
			contextFields: &AnamneseTrainingHint{
				StatusAprovacao: "aprovada",
				LesoesAtuais:    "tendinite no manguito do ombro",
				RiskScore:       1,
			},
			wantReason: "restrição de ombro",
		},
		{
			name:    "high risk score blocks metabolic intensity",
			req:     testGenerationRequest("sem restrições livres"),
			fixture: "testdata/ai/gemini_periodized_cardio_unsafe.json",
			contextFields: &AnamneseTrainingHint{
				StatusAprovacao: "aprovada",
				RiskScore:       3,
			},
			wantReason: "restrição cardiorrespiratória",
		},
		{
			name:       "cervical restriction",
			req:        testGenerationRequest("dor cervical crônica"),
			fixture:    "testdata/ai/gemini_periodized_cervical_unsafe.json",
			wantReason: "restrição cervical",
		},
		{
			name:       "wrist elbow restriction",
			req:        testGenerationRequest("epicondilite lateral e dor no punho"),
			fixture:    "testdata/ai/gemini_periodized_wrist_elbow_unsafe.json",
			wantReason: "restrição de punho/cotovelo",
		},
		{
			name:       "ankle restriction",
			req:        testGenerationRequest("entorse de tornozelo recente"),
			fixture:    "testdata/ai/gemini_periodized_ankle_unsafe.json",
			wantReason: "restrição de tornozelo",
		},
		{
			name:       "pregnancy restriction",
			req:        testGenerationRequest("aluna gestante segundo trimestre"),
			fixture:    "testdata/ai/gemini_periodized_pregnancy_unsafe.json",
			wantReason: "restrição gestacional",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.contextFields != nil {
				tt.req.Contexto = &AthleteTrainingContext{Anamnese: tt.contextFields}
			}
			chain := NewTrainingProviderChain(
				AITrainingModeAssistive,
				0,
				[]TrainingProvider{
					fakeTrainingProvider{name: "gemini", model: "fixture-gemini", fixturePath: tt.fixture},
					LocalTrainingProvider{},
				},
				nil,
				nil,
				nil,
			)

			result, err := chain.Resolve(t.Context(), tt.req)
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			if result.Metadata.AIUsed {
				t.Fatal("expected unsafe fixture to fallback to local")
			}
			if !result.Metadata.FallbackUsed {
				t.Fatal("expected fallback_used after clinical safety rejection")
			}
			if !strings.Contains(result.Metadata.FallbackReason, tt.wantReason) {
				t.Fatalf("expected fallback reason to contain %q, got %q", tt.wantReason, result.Metadata.FallbackReason)
			}
		})
	}
}

func TestTrainingProviderChainMissingFieldsFixtureReturnsQualityWarnings(t *testing.T) {
	req := testGenerationRequest("sem restrições")
	chain := NewTrainingProviderChain(
		AITrainingModeAssistive,
		0,
		[]TrainingProvider{
			fakeTrainingProvider{name: "gemini", model: "fixture-gemini", fixturePath: "testdata/ai/gemini_periodized_missing_fields.json"},
		},
		nil,
		nil,
		nil,
	)

	result, err := chain.Resolve(t.Context(), req)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if !result.Metadata.AIUsed {
		t.Fatal("expected structurally valid fixture to be accepted")
	}
	if result.Metadata.QualityValidated {
		t.Fatal("expected fixture with missing fields to be accepted but not quality validated")
	}
	if len(result.Metadata.Warnings) == 0 {
		t.Fatal("expected quality warnings for missing exercise fields")
	}
	if !strings.Contains(strings.Join(result.Metadata.Warnings, " "), "grupo_muscular") {
		t.Fatalf("expected warning about missing grupo_muscular, got %+v", result.Metadata.Warnings)
	}
}

func TestTrainingProviderChainMalformedFixtureFallsBackToLocal(t *testing.T) {
	req := testGenerationRequest("sem restrições")
	chain := NewTrainingProviderChain(
		AITrainingModeAssistive,
		0,
		[]TrainingProvider{
			fakeTrainingProvider{name: "gemini", model: "fake-gemini", fixturePath: "testdata/ai/gemini_periodized_malformed.txt"},
			LocalTrainingProvider{},
		},
		nil,
		nil,
		nil,
	)

	result, err := chain.Resolve(t.Context(), req)
	if err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if result.Metadata.AIUsed {
		t.Fatal("expected malformed fixture to fallback to local")
	}
	if !result.Metadata.FallbackUsed {
		t.Fatal("expected fallback_used=true for malformed fixture")
	}
	if !strings.Contains(result.Metadata.FallbackReason, "rejected by safety") || !strings.Contains(result.Metadata.FallbackReason, "invalid character") {
		t.Fatalf("expected fallback reason to mention safety rejection and invalid JSON, got %q", result.Metadata.FallbackReason)
	}
}

func TestTrainingProviderChainRequiredModeFailsWhenProvidersFail(t *testing.T) {
	req := testGenerationRequest("sem restrições")
	chain := NewTrainingProviderChain(
		AITrainingModeRequired,
		0,
		[]TrainingProvider{
			fakeTrainingProvider{name: "gemini", model: "fake-gemini", err: errors.New("quota exceeded")},
		},
		nil,
		nil,
		nil,
	)

	_, err := chain.Resolve(t.Context(), req)
	if err == nil {
		t.Fatal("expected required mode to fail")
	}
	if !strings.Contains(err.Error(), "all AI training providers failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildTrainingPromptIncludesContextAndEvidence(t *testing.T) {
	req := testGenerationRequest("dor lombar")
	req.Contexto = &AthleteTrainingContext{
		Complexidade: "complexo",
		DadosUsados:  []string{"aluno", "anamnese", "base_conhecimento"},
		Anamnese: &AnamneseTrainingHint{
			StatusAprovacao: "aprovada",
			Patologias:      "Lombalgia",
			DoresCronicas:   "dor lombar",
			RiskScore:       3,
		},
		Evidencias: []KnowledgeEvidence{
			{
				Fonte:    "Dutton",
				Conteudo: "Evitar compressão axial em quadros lombares sintomáticos.",
				Tags:     []string{"lombalgia", "reabilitacao"},
			},
		},
	}

	prompt := buildTrainingPrompt(req)
	for _, want := range []string{"Lombalgia", "compressão axial", "complexo", "base_conhecimento"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got: %s", want, prompt)
		}
	}
	for _, want := range []string{"Formato mínimo esperado", "Chain-of-Thought", `"treinos"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt contract to contain %q, got: %s", want, prompt)
		}
	}
}

func testGenerationRequest(restricoes string) *GenerationRequest {
	return &GenerationRequest{
		Aluno: &domain.Aluno{
			ID:   1,
			Nome: "Carlos Teste",
		},
		Frequencia: 3,
		Objetivo:   "Hipertrofia",
		Nivel:      "intermediário",
		Restricoes: restricoes,
		LocalFicha: map[string]any{
			"tipo":       "periodizada",
			"frequencia": 3,
			"treinos": []map[string]any{
				{
					"letra": "A",
					"nome":  "A - Peito",
					"exercicios": []map[string]any{
						{"nome": "Supino Reto", "grupo_muscular": "Peito", "series": 3, "repeticoes": "10-12", "descanso": 60, "cadencia": "4010"},
					},
				},
			},
		},
	}
}

func testTrainingContext() *AthleteTrainingContext {
	return &AthleteTrainingContext{
		Complexidade: "moderado",
		DadosUsados:  []string{"aluno", "anamnese", "base_conhecimento"},
		Anamnese: &AnamneseTrainingHint{
			StatusAprovacao: "aprovada",
			Patologias:      "Lombalgia leve",
			RiskScore:       1,
		},
		Evidencias: []KnowledgeEvidence{
			{
				Fonte:      "Fixture",
				Conteudo:   "Progressao de carga deve respeitar restricoes clinicas e sinais de dor.",
				Tags:       []string{"progressao", "seguranca"},
				Relevancia: 0.85,
			},
		},
	}
}

func validTrainingJSON(exerciseName string) string {
	return `{
		"tipo": "periodizada",
		"frequencia": 3,
		"objetivo": "Hipertrofia",
		"nivel": "intermediário",
		"treinos": [
			{
				"letra": "A",
				"nome": "A - Peito",
				"exercicios": [
					{
						"nome": "` + exerciseName + `",
						"grupo_muscular": "Peito",
						"series": 3,
						"repeticoes": "10-12",
						"descanso": 60,
						"cadencia": "4010"
					}
				]
			}
		]
	}`
}
