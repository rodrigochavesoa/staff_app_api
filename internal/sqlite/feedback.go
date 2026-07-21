package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"staff_app/internal/domain"
)

type FeedbackRepository struct {
	db *DB
}

func NewFeedbackRepository(db *DB) *FeedbackRepository {
	return &FeedbackRepository{db: db}
}

// CreateFeedback grava o feedback e, na mesma transação, atualiza nota legada,
// cria notificação (user_id NULL = todos) e estatísticas do link público.
func (r *FeedbackRepository) CreateFeedback(ctx context.Context, fb *domain.FeedbackFicha) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin feedback transaction: %w", err)
	}
	defer tx.Rollback()

	var alunoID, fichaID int64
	var ativoInt int
	err = tx.QueryRowContext(ctx, `
		SELECT aluno_id, ficha_id, ativo 
		FROM fichas_web 
		WHERE hash = ?
	`, fb.HashFicha).Scan(&alunoID, &fichaID, &ativoInt)

	if err != nil {
		return 0, fmt.Errorf("failed to find public link: %w", err)
	}

	if ativoInt != 1 {
		return 0, fmt.Errorf("deactivated link: public link is no longer active")
	}

	fb.AlunoID = alunoID

	var comentarioVal sql.NullString
	if fb.Comentario != nil {
		comentarioVal = sql.NullString{String: *fb.Comentario, Valid: true}
	}

	queryInsertFeedback := `
		INSERT INTO feedback_fichas (hash_ficha, aluno_id, rating, comentario, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	now := time.Now()
	fb.CreatedAt = now

	res, err := tx.ExecContext(ctx, queryInsertFeedback, fb.HashFicha, fb.AlunoID, fb.Rating, comentarioVal, now.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("failed to insert feedback record: %w", err)
	}

	feedbackID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve feedback last insert ID: %w", err)
	}
	fb.ID = feedbackID

	_, err = tx.ExecContext(ctx, `
		UPDATE fichas 
		SET feedback_rating = ?
		WHERE id = ?
	`, fb.Rating, fichaID)
	if err != nil {
		return 0, fmt.Errorf("failed to update legacy training plan rating: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO feedback_notificacoes (feedback_id, user_id, lido, criado_em)
		VALUES (?, NULL, 0, ?)
	`, feedbackID, now.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("failed to create feedback notification: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE fichas_web 
		SET ultimo_acesso = ?,
		    acessos = acessos + 1
		WHERE hash = ?
	`, now.Format(time.RFC3339), fb.HashFicha)
	if err != nil {
		return 0, fmt.Errorf("failed to update public link accesses: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit feedback transaction: %w", err)
	}

	return feedbackID, nil
}

func (r *FeedbackRepository) GetFeedbackByHash(ctx context.Context, hash string) (*domain.FeedbackFicha, error) {
	query := `
		SELECT id, hash_ficha, aluno_id, rating, comentario, created_at
		FROM feedback_fichas
		WHERE hash_ficha = ?
	`

	var fb domain.FeedbackFicha
	var comentarioVal sql.NullString
	var createdAtStr string

	err := r.db.QueryRowContext(ctx, query, hash).Scan(
		&fb.ID, &fb.HashFicha, &fb.AlunoID, &fb.Rating, &comentarioVal, &createdAtStr,
	)
	if err != nil {
		return nil, err
	}

	if comentarioVal.Valid {
		fb.Comentario = &comentarioVal.String
	}

	if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		fb.CreatedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
		fb.CreatedAt = t
	}

	return &fb, nil
}

func (r *FeedbackRepository) ListPendingFeedbacks(ctx context.Context, userID *int64) ([]*domain.FeedbackFicha, error) {
	query := `
		SELECT 
			f.id,
			f.hash_ficha,
			f.aluno_id,
			f.rating,
			f.comentario,
			f.created_at,
			a.nome as aluno_nome,
			fn.id as notificacao_id
		FROM feedback_fichas f
		JOIN feedback_notificacoes fn ON f.id = fn.feedback_id
		JOIN alunos a ON f.aluno_id = a.id
		WHERE fn.lido = 0
	`

	var args []any
	if userID != nil {
		query += " AND (fn.user_id = ? OR fn.user_id IS NULL)"
		args = append(args, *userID)
	} else {
		query += " AND fn.user_id IS NULL"
	}

	query += " ORDER BY f.created_at DESC LIMIT 50"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending feedbacks: %w", err)
	}
	defer rows.Close()

	var list []*domain.FeedbackFicha
	for rows.Next() {
		var fb domain.FeedbackFicha
		var comentarioVal sql.NullString
		var createdAtStr string

		err := rows.Scan(
			&fb.ID, &fb.HashFicha, &fb.AlunoID, &fb.Rating, &comentarioVal, &createdAtStr, &fb.AlunoNome, &fb.NotificacaoID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if comentarioVal.Valid {
			fb.Comentario = &comentarioVal.String
		}

		if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			fb.CreatedAt = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
			fb.CreatedAt = t
		}

		list = append(list, &fb)
	}

	return list, nil
}

func (r *FeedbackRepository) MarkNotificationLida(ctx context.Context, notificationID int64) error {
	query := `
		UPDATE feedback_notificacoes
		SET lido = 1, lido_em = ?
		WHERE id = ?
	`
	now := time.Now().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx, query, now, notificationID)
	if err != nil {
		return fmt.Errorf("failed to mark notification as read: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *FeedbackRepository) CreateNotification(ctx context.Context, feedbackID int64) error {
	query := `
		INSERT INTO feedback_notificacoes (feedback_id, user_id, lido, criado_em)
		VALUES (?, NULL, 0, ?)
	`
	now := time.Now().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, query, feedbackID, now)
	return err
}
