package sqlite

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
)

func TestFeedbackRepositoryCRUD(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-feedback-test-*")
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

	// Insert test dependencies
	_, _ = db.ExecContext(ctx, "INSERT INTO alunos (id, nome, idade, sexo, email) VALUES (1, 'Carlos Aluno', 25, 'M', 'carlos@test.com')")
	_, _ = db.ExecContext(ctx, "INSERT INTO fichas_treino_web (id, aluno, ficha_json) VALUES (10, 'Carlos Aluno', '{}')")
	// Insert legacy monolithic training plan row
	_, _ = db.ExecContext(ctx, "INSERT INTO fichas (id, aluno_id, feedback_rating) VALUES (10, 1, 0)")

	// Create active public link
	expiration := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_web (hash, ficha_id, aluno_id, conteudo_json, expira_em, ativo, acessos)
		VALUES ('hash123', 10, 1, '{}', ?, 1, 0)
	`, expiration)
	if err != nil {
		t.Fatalf("failed to insert public link: %v", err)
	}

	repo := NewFeedbackRepository(db)

	// 1. CreateFeedback - Success
	comment := "Muito bom o treino!"
	fb := &domain.FeedbackFicha{
		HashFicha:  "hash123",
		Rating:     5,
		Comentario: &comment,
	}

	feedbackID, err := repo.CreateFeedback(ctx, fb)
	if err != nil {
		t.Fatalf("failed to create feedback: %v", err)
	}
	if feedbackID == 0 {
		t.Errorf("expected non-zero feedback ID")
	}

	// Verify legacy monolithic training plan update
	var legacyRating int
	err = db.QueryRowContext(ctx, "SELECT feedback_rating FROM fichas WHERE id = 10").Scan(&legacyRating)
	if err != nil {
		t.Fatalf("failed to select legacy rating: %v", err)
	}
	if legacyRating != 5 {
		t.Errorf("expected legacy rating to be 5, got %d", legacyRating)
	}

	// Verify access count incremented in public link
	var accesses int
	err = db.QueryRowContext(ctx, "SELECT acessos FROM fichas_web WHERE hash = 'hash123'").Scan(&accesses)
	if err != nil {
		t.Fatalf("failed to select accesses: %v", err)
	}
	if accesses != 1 {
		t.Errorf("expected accesses to be 1, got %d", accesses)
	}

	// 2. GetFeedbackByHash - Success
	fetched, err := repo.GetFeedbackByHash(ctx, "hash123")
	if err != nil {
		t.Fatalf("failed to fetch feedback: %v", err)
	}
	if fetched.Rating != 5 || *fetched.Comentario != comment || fetched.AlunoID != 1 {
		t.Errorf("fetched feedback details mismatch: %+v", fetched)
	}

	// 3. Unique Constraint - Duplicate feedback for same hash should fail
	fb2 := &domain.FeedbackFicha{
		HashFicha: "hash123",
		Rating:    4,
	}
	_, err = repo.CreateFeedback(ctx, fb2)
	if err == nil {
		t.Errorf("expected error due to unique constraint on hash_ficha, got nil")
	}

	// 4. ListPendingFeedbacks
	pending, err := repo.ListPendingFeedbacks(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list pending feedbacks: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending notification, got %d", len(pending))
	}
	if pending[0].AlunoNome != "Carlos Aluno" || pending[0].NotificacaoID == 0 {
		t.Errorf("unexpected pending details: %+v", pending[0])
	}

	// 5. MarkNotificationLida - Success
	err = repo.MarkNotificationLida(ctx, pending[0].NotificacaoID)
	if err != nil {
		t.Fatalf("failed to mark notification as read: %v", err)
	}

	// Verify lido = 1 and count becomes 0
	pending2, err := repo.ListPendingFeedbacks(ctx, nil)
	if err != nil {
		t.Fatalf("failed to list pending: %v", err)
	}
	if len(pending2) != 0 {
		t.Errorf("expected 0 pending notifications after read marking, got %d", len(pending2))
	}

	// MarkNotificationLida - Non-existent ID returns sql.ErrNoRows
	err = repo.MarkNotificationLida(ctx, 99999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows for non-existent notification ID, got: %v", err)
	}
}
