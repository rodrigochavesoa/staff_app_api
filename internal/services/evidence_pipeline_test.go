package services

import (
	"context"
	"testing"

	"staff_app/internal/domain"
)

type panicSearcher struct{}

func (panicSearcher) Search(context.Context, EvidenceSearchRequest) ([]KnowledgeEvidence, error) {
	panic("search must not run for simple cases")
}

func TestEvidencePipelineSimpleSkipsEvidenceSearch(t *testing.T) {
	docs := &fakeDocSearcher{
		docs: []domain.KnowledgeDocument{{
			Fonte:      "fixture",
			Conteudo:   "Nao deve ser usado em caso simples.",
			Relevancia: 0.9,
		}},
	}
	pipeline := &EvidencePipeline{
		Structured: NewSQLStructuredContextLoader(newTestContextDB(t), fakeAnamneseFinder{}),
		Classifier: DeterministicCaseComplexityClassifier{},
		Searcher:   NewHybridKnowledgeEvidenceSearcher(docs),
	}

	ctxOut, err := pipeline.Build(t.Context(), 1, GenerationRequest{
		Frequencia: 3,
		Objetivo:   "Condicionamento",
		Nivel:      "iniciante",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if docs.calls != 0 {
		t.Fatalf("expected no evidence search for simple case, calls=%d", docs.calls)
	}
	if ctxOut.Complexidade != "simples" {
		t.Fatalf("complexity=%q want simples", ctxOut.Complexidade)
	}
	if len(ctxOut.Evidencias) != 0 {
		t.Fatalf("expected no evidencias, got %d", len(ctxOut.Evidencias))
	}
}

func TestEvidencePipelineSimplePanicsIfSearcherCalled(t *testing.T) {
	pipeline := &EvidencePipeline{
		Structured: NewSQLStructuredContextLoader(newTestContextDB(t), fakeAnamneseFinder{}),
		Classifier: DeterministicCaseComplexityClassifier{},
		Searcher:   panicSearcher{},
	}
	_, err := pipeline.Build(t.Context(), 1, GenerationRequest{
		Frequencia: 3,
		Objetivo:   "Condicionamento",
		Nivel:      "iniciante",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
}

func TestEvidencePipelineModeradoSearchesEvidence(t *testing.T) {
	anam := &domain.Anamnese{
		StatusAprovacao: "aprovada",
		Patologias:      "lombalgia",
		RiskScoreCached: 1,
	}
	docs := &fakeDocSearcher{
		docs: []domain.KnowledgeDocument{{
			Fonte:      "fixture",
			Conteudo:   "Evidencia de teste sobre lombalgia e estabilizacao.",
			Tags:       []string{"lombalgia"},
			Relevancia: 0.9,
		}},
	}
	pipeline := &EvidencePipeline{
		Structured: NewSQLStructuredContextLoader(newTestContextDB(t), fakeAnamneseFinder{anamnese: anam}),
		Classifier: DeterministicCaseComplexityClassifier{},
		Searcher:   NewHybridKnowledgeEvidenceSearcher(docs),
	}

	ctxOut, err := pipeline.Build(t.Context(), 1, GenerationRequest{
		Frequencia: 3,
		Objetivo:   "Hipertrofia",
		Nivel:      "intermediario",
		Restricoes: "dor lombar",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if docs.calls == 0 {
		t.Fatal("expected evidence search for moderado case")
	}
	if ctxOut.Complexidade != "moderado" {
		t.Fatalf("complexity=%q want moderado", ctxOut.Complexidade)
	}
	if len(ctxOut.Evidencias) == 0 {
		t.Fatal("expected evidencias from fake searcher")
	}
	if !containsString(ctxOut.DadosUsados, "base_conhecimento") {
		t.Fatalf("expected base_conhecimento in DadosUsados, got %v", ctxOut.DadosUsados)
	}
}
