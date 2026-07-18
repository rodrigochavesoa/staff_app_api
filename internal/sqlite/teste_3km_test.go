package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
)

func TestTeste3kmRepositoryCRUD(t *testing.T) {
	logger.Setup("development", false)

	// Create a temp database path
	tempDir, err := os.MkdirTemp("", "sqlite-repo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test_fichas_treino.db")

	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	var ctx context.Context = t.Context()

	// 1. Create a dummy Aluno (required due to Foreign Key constraint)
	_, err = db.ExecContext(ctx, `
		INSERT INTO alunos (id, nome, idade, sexo, email) 
		VALUES (1, 'Carlos Silva', 30, 'M', 'carlos@test.com')
	`)
	if err != nil {
		t.Fatalf("failed to insert dummy aluno: %v", err)
	}

	repo := NewTeste3kmRepository(db)

	// 2. Insert test data
	pse := 8
	conf := 5
	test1 := &domain.Teste3km{
		AlunoID:         1,
		DataTeste:       time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
		TempoSegundos:   720, // 12min
		PSE:             &pse,
		Fonte:           "garmin",
		VDOT:            47.9,
		FTPPaceSegundos: 254,
		PaceZ1Min:       328,
		PaceZ1Max:       363,
		PaceZ2Min:       290,
		PaceZ2Max:       325,
		PaceZ3Min:       254,
		PaceZ3Max:       287,
		PaceZ4Min:       224,
		PaceZ4Max:       251,
		PaceZ5Min:       191,
		PaceZ5Max:       221,
		IndiceConfianca: &conf,
		Observacoes:     "Forte no final",
	}

	err = repo.Create(ctx, test1)
	if err != nil {
		t.Fatalf("failed to create 3k test: %v", err)
	}

	if test1.ID == 0 {
		t.Error("expected populated ID after insert")
	}

	// 3. List tests for Aluno
	tests, err := repo.ListByAlunoID(ctx, 1)
	if err != nil {
		t.Fatalf("failed to list 3k tests: %v", err)
	}

	if len(tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(tests))
	}

	got := tests[0]
	if got.ID != test1.ID || got.TempoSegundos != 720 || got.Fonte != "garmin" || got.VDOT != 47.9 {
		t.Errorf("retrieved test mismatch: %+v", got)
	}
	if got.PSE == nil || *got.PSE != 8 {
		t.Errorf("expected PSE to be 8, got %+v", got.PSE)
	}
	if got.DataTeste.Format("2006-01-02") != "2026-07-16" {
		t.Errorf("expected date 2026-07-16, got %s", got.DataTeste.Format("2006-01-02"))
	}

	// 4. Test cross-student deletion prevention (deleting using wrong student ID)
	err = repo.Delete(ctx, test1.ID, 2) // student ID 2 instead of 1
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows when attempting to delete with wrong student ID, got %v", err)
	}

	// Verify it was NOT deleted
	testsVerify, err := repo.ListByAlunoID(ctx, 1)
	if err != nil || len(testsVerify) != 1 {
		t.Fatalf("test should not have been deleted: err=%v, count=%d", err, len(testsVerify))
	}

	// 5. Delete test (correct student ID)
	err = repo.Delete(ctx, test1.ID, 1)
	if err != nil {
		t.Fatalf("failed to delete 3k test: %v", err)
	}

	// Verify it's deleted
	testsAfter, err := repo.ListByAlunoID(ctx, 1)
	if err != nil {
		t.Fatalf("failed to list 3k tests after deletion: %v", err)
	}
	if len(testsAfter) != 0 {
		t.Errorf("expected 0 tests after deletion, got %d", len(testsAfter))
	}

	// Verify delete of non-existing ID returns error
	err = repo.Delete(ctx, 99999, 1)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows for invalid delete, got %v", err)
	}
}
