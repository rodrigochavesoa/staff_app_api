package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/services"
)

func TestSearchLocalDocumentCandidatesTokenOR(t *testing.T) {
	logger.Setup("development", false)
	repo := newTestRAGRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	compound := "Hipertrofia intermediario musculacao dor lombar"

	narrow, err := repo.SearchLocalDocuments(ctx, compound, "musculacao", 20)
	if err != nil {
		t.Fatalf("SearchLocalDocuments: %v", err)
	}
	if len(narrow) != 0 {
		t.Fatalf("expected compound substring LIKE to miss seed docs, got %d hits", len(narrow))
	}

	cands, err := repo.SearchLocalDocumentCandidates(ctx, compound, "musculacao", 20)
	if err != nil {
		t.Fatalf("SearchLocalDocumentCandidates: %v", err)
	}
	if len(cands) == 0 {
		t.Fatal("expected token-OR candidates from migration 0014 seed")
	}

	foundLombalgia := false
	for _, doc := range cands {
		blob := strings.ToLower(doc.Titulo + " " + doc.Conteudo + " " + strings.Join(doc.Tags, " "))
		if strings.Contains(blob, "lombalgia") {
			foundLombalgia = true
			break
		}
	}
	if !foundLombalgia {
		t.Fatalf("expected lombalgia seed among candidates, got fontes=%v", candidateFontes(cands))
	}
}

func TestHybridSearchViaSQLiteCandidates(t *testing.T) {
	logger.Setup("development", false)
	repo := newTestRAGRepo(t)
	searcher := services.NewHybridKnowledgeEvidenceSearcher(repo)

	got, err := searcher.Search(context.Background(), services.EvidenceSearchRequest{
		Generation: services.GenerationRequest{
			Objetivo:   "Hipertrofia",
			Nivel:      "intermediario",
			Restricoes: "dor lombar",
		},
		Anamnese:   &services.AnamneseTrainingHint{Patologias: "lombalgia"},
		Complexity: "moderado",
		Modalidade: "musculacao",
		TopK:       3,
	})
	if err != nil {
		t.Fatalf("hybrid Search: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected ≥1 evidence via SQLite candidates + rerank")
	}

	top := strings.ToLower(got[0].Fonte + " " + got[0].Conteudo + " " + strings.Join(got[0].Tags, " "))
	if !strings.Contains(top, "lombalgia") && !strings.Contains(top, "lombar") {
		t.Fatalf("expected clinical lombalgia signal near top, got fonte=%q tags=%v", got[0].Fonte, got[0].Tags)
	}
}

func TestSearchLocalDocumentsAdminContractUnchanged(t *testing.T) {
	logger.Setup("development", false)
	repo := newTestRAGRepo(t)
	ctx := context.Background()

	// Short admin-style query still works with substring LIKE.
	hits, err := repo.SearchLocalDocuments(ctx, "lombalgia", "Musculação", 5)
	if err != nil {
		t.Fatalf("SearchLocalDocuments: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected admin short query to hit seed lombalgia doc")
	}
}

func newTestRAGRepo(t *testing.T) *RAGRepository {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "sqlite-rag-evidence-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	dbPath := filepath.Join(tempDir, "test_fichas_treino.db")
	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRAGRepository(db)
}

func candidateFontes(docs []domain.KnowledgeDocument) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.Fonte
	}
	return out
}
