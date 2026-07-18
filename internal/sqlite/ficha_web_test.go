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

func TestFichaWebRepositoryCRUD(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-ficha-web-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	var ctx context.Context = t.Context()

	// Insert dummy student
	_, err = db.ExecContext(ctx, "INSERT INTO alunos (id, nome, idade, sexo, email) VALUES (1, 'Test Student', 20, 'M', 'stu@test.com')")
	if err != nil {
		t.Fatalf("failed to insert student: %v", err)
	}

	// Insert legacy ficha_treino_web
	_, err = db.ExecContext(ctx, "INSERT INTO fichas_treino_web (id, aluno, ficha_json) VALUES (10, 'Test Student', '{\"exercicios\": []}')")
	if err != nil {
		t.Fatalf("failed to insert legacy ficha: %v", err)
	}

	repo := NewFichaWebRepository(db)

	// 1. Test GetFichaJSON
	jsonStr, err := repo.GetFichaJSON(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get legacy ficha JSON: %v", err)
	}
	if jsonStr != `{"exercicios": []}` {
		t.Errorf("expected json content, got %q", jsonStr)
	}

	// 2. Test Create Link
	fw1 := &domain.FichaWeb{
		Hash:         "hash123",
		FichaID:      10,
		AlunoID:      1,
		ConteudoJSON: `{"exercicios": []}`,
		CriadoEm:     time.Now().Add(-1 * time.Hour),
		ExpiraEm:     time.Now().Add(24 * time.Hour),
	}

	err = repo.Create(ctx, fw1)
	if err != nil {
		t.Fatalf("failed to create public link: %v", err)
	}
	if fw1.ID == 0 {
		t.Error("expected last insert ID to be populated")
	}

	// Create another active link for the same ficha ID -> should deactivate fw1
	fw2 := &domain.FichaWeb{
		Hash:         "hash456",
		FichaID:      10,
		AlunoID:      1,
		ConteudoJSON: `{"exercicios": []}`,
		CriadoEm:     time.Now(),
		ExpiraEm:     time.Now().Add(48 * time.Hour),
	}

	err = repo.Create(ctx, fw2)
	if err != nil {
		t.Fatalf("failed to create second public link: %v", err)
	}

	// Verify fw1 is now inactive
	got1, err := repo.GetByHash(ctx, "hash123")
	if err != nil {
		t.Fatalf("failed to get fw1: %v", err)
	}
	if got1.Ativo {
		t.Error("expected fw1 to be soft-deactivated")
	}

	// Verify fw2 is active
	got2, err := repo.GetByHash(ctx, "hash456")
	if err != nil {
		t.Fatalf("failed to get fw2: %v", err)
	}
	if !got2.Ativo {
		t.Error("expected fw2 to be active")
	}

	// 3. Test IncrementAccessCount
	err = repo.IncrementAccessCount(ctx, "hash456", "Mozilla/5.0", "192.168.1.100")
	if err != nil {
		t.Fatalf("failed to increment access count: %v", err)
	}

	got2Updated, err := repo.GetByHash(ctx, "hash456")
	if err != nil {
		t.Fatalf("failed to get updated fw2: %v", err)
	}
	if got2Updated.Acessos != 1 {
		t.Errorf("expected 1 access, got %d", got2Updated.Acessos)
	}
	if got2Updated.UltimoAcesso == nil {
		t.Error("expected UltimoAcesso to be set")
	}

	// 4. Test GetStats
	stats, err := repo.GetStats(ctx, "hash456")
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.Acessos != 1 || len(stats.HistoricoAcessos) != 1 {
		t.Errorf("stats mismatch: %+v", stats)
	}
	if stats.HistoricoAcessos[0].UserAgent != "Mozilla/5.0" || stats.HistoricoAcessos[0].IPAddress != "192.168.1.100" {
		t.Errorf("access history details mismatch: %+v", stats.HistoricoAcessos[0])
	}

	// 5. Test Renew
	newExp := time.Now().Add(10 * 24 * time.Hour)
	newContent := `{"exercicios": ["new"]}`
	err = repo.Renew(ctx, "hash456", newExp, &newContent)
	if err != nil {
		t.Fatalf("failed to renew link: %v", err)
	}

	got2Renewed, _ := repo.GetByHash(ctx, "hash456")
	if got2Renewed.ConteudoJSON != newContent {
		t.Errorf("renewed content mismatch: %q", got2Renewed.ConteudoJSON)
	}

	// 6. Test Deactivate
	err = repo.Deactivate(ctx, "hash456")
	if err != nil {
		t.Fatalf("failed to deactivate link: %v", err)
	}

	got2Deactivated, _ := repo.GetByHash(ctx, "hash456")
	if got2Deactivated.Ativo {
		t.Error("expected link to be inactive after deactivation")
	}

	// Renewing inactive link should return sql.ErrNoRows
	err = repo.Renew(ctx, "hash456", newExp, nil)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows when renewing inactive link, got %v", err)
	}

	// 7. Test ListByAlunoID
	list, err := repo.ListByAlunoID(ctx, 1, true)
	if err != nil {
		t.Fatalf("failed to list by student ID: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 links total for student 1, got %d", len(list))
	}
}
