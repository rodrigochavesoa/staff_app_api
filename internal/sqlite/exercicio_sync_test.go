package sqlite

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/exercicios/csvsync"
	"staff_app/internal/platform/logger"
)

func TestCatalogSyncSQLite(t *testing.T) {
	logger.Setup("development", false)
	db, cleanup := openExercicioSyncDB(t)
	defer cleanup()
	ctx := t.Context()
	repo := NewExercicioRepository(db)

	seedSQL := `
		INSERT INTO exercicios_reabilitacao (
			codigo, nome, categoria, grupo_muscular, nivel_prioridade, url, criado_por, criado_em, status
		) VALUES (?, ?, 'normal', ?, 2, '', ?, '2026-07-20 00:00:00', 'ativo')`
	for _, row := range []struct {
		codigo int
		nome   string
		grupo  string
		owner  string
	}{
		{5000, "Personalizado Keep", "Outro", "admin"},
		{300, "Nome Em Uso", "Costas", "admin"},
	} {
		if _, err := db.ExecContext(ctx, seedSQL, row.codigo, row.nome, row.grupo, row.owner); err != nil {
			t.Fatalf("seed %d: %v", row.codigo, err)
		}
	}

	ex := &domain.ExercicioReabilitacao{
		Codigo: 100, Nome: "Supino Reto", Categoria: "normal", GrupoMuscular: "Peito",
		NivelPrioridade: 2, Url: "https://example.com/100", CriadoPor: csvsync.CatalogMarker,
		CriadoEm: time.Now().UTC(), Status: "ativo",
	}
	inserted, err := repo.UpsertCatalogExercise(ctx, ex)
	if err != nil || !inserted {
		t.Fatalf("upsert insert: %v inserted=%v", err, inserted)
	}
	ex.GrupoMuscular = "Peitoral"
	inserted, err = repo.UpsertCatalogExercise(ctx, ex)
	if err != nil || inserted {
		t.Fatalf("upsert update: %v inserted=%v", err, inserted)
	}

	res, err := csvsync.Sync(ctx, repo, []csvsync.Record{
		{Codigo: 110, Nome: "Catalogo A", GrupoMuscular: "Peito", URL: "https://example.com/a"},
		{Codigo: 200, Nome: "Nome Em Uso", GrupoMuscular: "Peito"},
	})
	if err != nil || res.Inserted != 1 || res.SkippedNameConflict != 1 {
		t.Fatalf("sync=%+v err=%v", res, err)
	}

	res2, err := csvsync.Sync(ctx, repo, []csvsync.Record{
		{Codigo: 110, Nome: "Catalogo A", GrupoMuscular: "Peito", URL: "https://example.com/a"},
	})
	if err != nil || res2.Inserted != 0 || res2.Updated != 0 {
		t.Fatalf("idempotent=%+v err=%v", res2, err)
	}

	res3, err := csvsync.Sync(ctx, repo, []csvsync.Record{
		{Codigo: 110, Nome: "Catalogo A", GrupoMuscular: "Peitoral", URL: "https://example.com/a"},
	})
	if err != nil || res3.Updated != 1 {
		t.Fatalf("update=%+v err=%v", res3, err)
	}

	if _, err := repo.UpsertCatalogExercise(ctx, &domain.ExercicioReabilitacao{
		Codigo: 300, Nome: "From CSV", CriadoPor: csvsync.CatalogMarker, CriadoEm: time.Now().UTC(), Status: "ativo",
	}); err == nil {
		t.Fatal("expected owner conflict on upsert")
	}
	personalizado, _ := repo.GetByCodigo(ctx, 5000)
	if personalizado == nil || personalizado.Nome != "Personalizado Keep" {
		t.Fatalf("personalizado=%+v", personalizado)
	}
}

func openExercicioSyncDB(t *testing.T) (*DB, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "sqlite-exercicio-sync-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	db, err := Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("connect: %v", err)
	}
	return db, func() { db.Close(); os.RemoveAll(tempDir) }
}
