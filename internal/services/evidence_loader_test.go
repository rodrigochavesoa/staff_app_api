package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"staff_app/internal/domain"

	_ "modernc.org/sqlite"
)

type fakeAnamneseFinder struct {
	anamnese *domain.Anamnese
	err      error
}

func (f fakeAnamneseFinder) FindActiveByAlunoID(context.Context, int64) (*domain.Anamnese, error) {
	return f.anamnese, f.err
}

type fakeDocSearcher struct {
	docs  []domain.KnowledgeDocument
	calls int
}

func (f *fakeDocSearcher) SearchLocalDocuments(context.Context, string, string, int) ([]domain.KnowledgeDocument, error) {
	f.calls++
	return f.docs, nil
}

type sqliteContextDB struct {
	db *sql.DB
}

func (s sqliteContextDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

func (s sqliteContextDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

func newTestContextDB(t *testing.T) sqliteContextDB {
	t.Helper()
	dsn := fmt.Sprintf(
		"file:evidence-loader-%s?mode=memory&cache=shared",
		strings.ReplaceAll(t.Name(), "/", "_"),
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS alunos (id INTEGER PRIMARY KEY, nome TEXT NOT NULL);
		INSERT OR IGNORE INTO alunos (id, nome) VALUES (1, 'Test Student');
		CREATE TABLE IF NOT EXISTS fichas_treino_web (
			id INTEGER PRIMARY KEY,
			aluno TEXT NOT NULL,
			tipo_ficha TEXT,
			objetivo TEXT,
			nivel TEXT,
			ies_score REAL,
			volume_sved INTEGER,
			densidade REAL,
			data_criacao TEXT
		);
	`); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return sqliteContextDB{db: db}
}

func TestSQLStructuredContextLoaderDadosUsados(t *testing.T) {
	anam := &domain.Anamnese{
		StatusAprovacao:   "aprovada",
		Patologias:        "lombalgia",
		RiskScoreCached:   1,
		ExperienciaTreino: "intermediario",
	}
	loader := NewSQLStructuredContextLoader(newTestContextDB(t), fakeAnamneseFinder{anamnese: anam})

	ctxOut, err := loader.Load(t.Context(), 1, GenerationRequest{
		Frequencia: 3,
		Objetivo:   "Hipertrofia",
		Nivel:      "intermediario",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	wantPrefix := []string{"aluno", "ficha_local", "anamnese"}
	for i, key := range wantPrefix {
		if i >= len(ctxOut.DadosUsados) || ctxOut.DadosUsados[i] != key {
			t.Fatalf("DadosUsados prefix=%v want start %v", ctxOut.DadosUsados, wantPrefix)
		}
	}
	if len(ctxOut.Evidencias) != 0 {
		t.Fatalf("structured loader must not attach evidencias, got %d", len(ctxOut.Evidencias))
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
