package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"staff_app/internal/platform/logger"
)

// TestFichaTreinoDeleteWithJourneyChildren covers the E2E cleanup failure mode:
// hard-delete of a manual ficha that still has treinos_realizados (and related
// fichas_web rows) after marcar/desmarcar in the journey.
func TestFichaTreinoDeleteWithJourneyChildren(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-ficha-treino-delete-*")
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

	ctx := t.Context()

	_, err = db.ExecContext(ctx, `
		INSERT INTO alunos (id, nome, idade, sexo, email)
		VALUES (1, 'Aluno E2E Delete', 30, 'M', 'e2e-delete@test.com')
	`)
	if err != nil {
		t.Fatalf("failed to seed aluno: %v", err)
	}

	const fichaID int64 = 1021
	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_treino_web (id, aluno, ficha_json, tipo_ficha, versao)
		VALUES (?, 'Aluno E2E Delete', '{"exercicios":[]}', 'manual', 1)
	`, fichaID)
	if err != nil {
		t.Fatalf("failed to seed ficha: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_web (hash, ficha_id, aluno_id, conteudo_json, criado_em, expira_em, acessos, ativo)
		VALUES ('hash-manual-e2e', ?, 1, '{}', '2026-07-19 00:00:00', '2026-08-19 00:00:00', 0, 1)
	`, fichaID)
	if err != nil {
		t.Fatalf("failed to seed fichas_web: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_web_acessos (hash, data_acesso, user_agent, ip_address)
		VALUES ('hash-manual-e2e', '2026-07-19 12:00:00', 'e2e', '127.0.0.1')
	`)
	if err != nil {
		t.Fatalf("failed to seed fichas_web_acessos: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO treinos_realizados (ficha_id, aluno_id, hash_ficha, data_treino, tipo_treino, tipo_ficha, observacao)
		VALUES (?, 1, 'hash-manual-e2e', '2026-07-19', 'A', 'musculacao', 'marcado no journey')
	`, fichaID)
	if err != nil {
		t.Fatalf("failed to seed treinos_realizados: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_web_feedback (ficha_id, rating, comentario)
		VALUES (?, 5, 'ok')
	`, fichaID)
	if err != nil {
		t.Fatalf("failed to seed fichas_web_feedback: %v", err)
	}

	repo := NewFichaTreinoRepository(db)
	if err := repo.Delete(ctx, fichaID); err != nil {
		t.Fatalf("Delete with journey children: %v", err)
	}

	checks := []struct {
		label string
		query string
		args  []any
	}{
		{"fichas_treino_web", `SELECT COUNT(*) FROM fichas_treino_web WHERE id = ?`, []any{fichaID}},
		{"fichas_web", `SELECT COUNT(*) FROM fichas_web WHERE ficha_id = ?`, []any{fichaID}},
		{"treinos_realizados", `SELECT COUNT(*) FROM treinos_realizados WHERE ficha_id = ?`, []any{fichaID}},
		{"fichas_web_feedback", `SELECT COUNT(*) FROM fichas_web_feedback WHERE ficha_id = ?`, []any{fichaID}},
		{"fichas_web_acessos", `SELECT COUNT(*) FROM fichas_web_acessos WHERE hash = ?`, []any{"hash-manual-e2e"}},
	}
	for _, c := range checks {
		var n int
		if err := db.QueryRowContext(ctx, c.query, c.args...).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", c.label, err)
		}
		if n != 0 {
			t.Errorf("expected 0 remaining %s rows, got %d", c.label, n)
		}
	}

	if err := repo.Delete(ctx, fichaID); err == nil {
		t.Fatal("expected sql.ErrNoRows on second delete, got nil")
	}
}
