package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"staff_app/internal/domain"
)

type catalogDocSearcher struct {
	docs  []domain.KnowledgeDocument
	calls int
}

func (c *catalogDocSearcher) SearchLocalDocuments(_ context.Context, query, _ string, limit int) ([]domain.KnowledgeDocument, error) {
	c.calls++
	if len(c.docs) == 0 {
		return nil, nil
	}
	q := tokenizeEvidence(query)
	out := make([]domain.KnowledgeDocument, 0, len(c.docs))
	for _, doc := range c.docs {
		blob := strings.ToLower(strings.Join(append([]string{doc.Titulo, doc.Conteudo, doc.Fonte}, doc.Tags...), " "))
		keep := len(q) == 0
		for _, tok := range q {
			if strings.Contains(blob, tok) {
				keep = true
				break
			}
		}
		if keep {
			out = append(out, doc)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func loadLombalgiaDocs(t *testing.T) []domain.KnowledgeDocument {
	t.Helper()
	path := filepath.Join("testdata", "evidence", "docs_lombalgia.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var docs []domain.KnowledgeDocument
	if err := json.Unmarshal(raw, &docs); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return docs
}

func TestHybridLombalgiaRanksAboveGeneric(t *testing.T) {
	docs := &catalogDocSearcher{docs: loadLombalgiaDocs(t)}
	searcher := NewHybridKnowledgeEvidenceSearcher(docs)

	got, err := searcher.Search(t.Context(), EvidenceSearchRequest{
		Generation: GenerationRequest{
			Objetivo:   "Hipertrofia",
			Nivel:      "intermediario",
			Restricoes: "dor lombar",
		},
		Anamnese:   &AnamneseTrainingHint{Patologias: "lombalgia"},
		Complexity: "moderado",
		Modalidade: "musculacao",
		TopK:       3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected evidence results")
	}
	if got[0].Fonte != "guia-lombalgia-estabilizacao" {
		t.Fatalf("rank[0]=%q want guia-lombalgia-estabilizacao (got %v)", got[0].Fonte, fontes(got))
	}
}

func TestHybridEmptyCatalogNoError(t *testing.T) {
	searcher := NewHybridKnowledgeEvidenceSearcher(&catalogDocSearcher{})
	got, err := searcher.Search(t.Context(), EvidenceSearchRequest{
		Generation: GenerationRequest{Objetivo: "Força", Restricoes: "joelho"},
		Complexity: "moderado",
		TopK:       3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty evidence, got %d", len(got))
	}
}

func TestHybridLexicalPlusVectorRerankOrder(t *testing.T) {
	docs := &catalogDocSearcher{docs: []domain.KnowledgeDocument{{
		Fonte:      "lexical-only",
		Titulo:     "Progressao de hipertrofia",
		Conteudo:   "Volume e progressao para hipertrofia sem foco clinico.",
		Tags:       []string{"hipertrofia"},
		Modalidade: "musculacao",
	}}}
	searcher := &HybridKnowledgeEvidenceSearcher{
		Docs:  docs,
		Embed: fakeEmbedder{},
		Store: fakeVectorStore{docs: []domain.KnowledgeDocument{{
			Fonte:      "vector-lombalgia",
			Titulo:     "Vetorial lombalgia",
			Conteudo:   "Documento recuperado por similaridade vetorial sobre lombalgia e estabilizacao.",
			Tags:       []string{"lombalgia"},
			Modalidade: "musculacao",
			Relevancia: 0.95,
		}}},
		Reranker: DeterministicEvidenceReranker{},
	}

	got, err := searcher.Search(t.Context(), EvidenceSearchRequest{
		Generation: GenerationRequest{
			Objetivo:   "Hipertrofia",
			Restricoes: "lombalgia",
		},
		Complexity: "moderado",
		Modalidade: "musculacao",
		TopK:       3,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("expected merged hybrid results, got %d (%v)", len(got), fontes(got))
	}
	if got[0].Fonte != "vector-lombalgia" {
		t.Fatalf("rank[0]=%q want vector-lombalgia after rerank (got %v)", got[0].Fonte, fontes(got))
	}
}

func TestEvidenceTopKByComplexity(t *testing.T) {
	if EvidenceTopK("moderado") != 3 {
		t.Fatalf("moderado TopK=%d want 3", EvidenceTopK("moderado"))
	}
	if EvidenceTopK("complexo") != 5 {
		t.Fatalf("complexo TopK=%d want 5", EvidenceTopK("complexo"))
	}
}

func TestHybridComplexoUsesMultipleQueries(t *testing.T) {
	docs := &catalogDocSearcher{docs: loadLombalgiaDocs(t)}
	searcher := NewHybridKnowledgeEvidenceSearcher(docs)
	_, err := searcher.Search(t.Context(), EvidenceSearchRequest{
		Generation: GenerationRequest{
			Objetivo:   "Reabilitacao",
			Nivel:      "intermediario",
			Restricoes: "dor lombar",
		},
		Anamnese: &AnamneseTrainingHint{
			Patologias:   "lombalgia",
			LesoesAtuais: "disco",
		},
		Complexity: "complexo",
		Modalidade: "musculacao",
		TopK:       5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if docs.calls < 2 {
		t.Fatalf("expected multiple sub-queries for complexo, calls=%d", docs.calls)
	}
}

type fakeEmbedder struct{}

func (fakeEmbedder) GenerateEmbeddings(context.Context, string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

type fakeVectorStore struct {
	docs []domain.KnowledgeDocument
}

func (f fakeVectorStore) SearchSimilar(context.Context, []float32, int) ([]domain.KnowledgeDocument, error) {
	return f.docs, nil
}

func fontes(ev []KnowledgeEvidence) []string {
	out := make([]string, len(ev))
	for i, e := range ev {
		out[i] = e.Fonte
	}
	return out
}
