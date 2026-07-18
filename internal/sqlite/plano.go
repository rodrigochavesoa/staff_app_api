package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"staff_app/internal/domain"
)

type PlanoRepository struct {
	db *DB
}

func NewPlanoRepository(db *DB) *PlanoRepository {
	return &PlanoRepository{db: db}
}

func (r *PlanoRepository) List(ctx context.Context, includeInactive bool) ([]*domain.Plano, error) {
	query := "SELECT id, nome, preco_default, COALESCE(descricao, ''), ativo FROM planos"
	if !includeInactive {
		query += " WHERE ativo = 1"
	}
	query += " ORDER BY preco_default ASC, nome ASC"

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list plans: %w", err)
	}
	defer rows.Close()

	var planos []*domain.Plano
	for rows.Next() {
		p, err := scanPlano(rows)
		if err != nil {
			return nil, err
		}
		planos = append(planos, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during plan rows iteration: %w", err)
	}
	return planos, nil
}

func (r *PlanoRepository) Create(ctx context.Context, p *domain.Plano) error {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO planos (nome, preco_default, descricao, ativo)
		VALUES (?, ?, ?, ?)
	`, p.Nome, p.PrecoDefault, p.Descricao, boolToInt(p.Ativo))
	if err != nil {
		return fmt.Errorf("failed to insert plan: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to retrieve plan id: %w", err)
	}
	p.ID = id
	return nil
}

func (r *PlanoRepository) Update(ctx context.Context, p *domain.Plano) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE planos
		SET nome = ?, preco_default = ?, descricao = ?, ativo = ?
		WHERE id = ?
	`, p.Nome, p.PrecoDefault, p.Descricao, boolToInt(p.Ativo), p.ID)
	if err != nil {
		return fmt.Errorf("failed to update plan: %w", err)
	}
	return requireAffected(res, sql.ErrNoRows)
}

func (r *PlanoRepository) Deactivate(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "UPDATE planos SET ativo = 0 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to deactivate plan: %w", err)
	}
	return requireAffected(res, sql.ErrNoRows)
}

func (r *PlanoRepository) GetByID(ctx context.Context, id int64) (*domain.Plano, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, nome, preco_default, COALESCE(descricao, ''), ativo FROM planos WHERE id = ?", id)
	return scanPlano(row)
}

func scanPlano(row rowScanner) (*domain.Plano, error) {
	var p domain.Plano
	var ativoInt int
	if err := row.Scan(&p.ID, &p.Nome, &p.PrecoDefault, &p.Descricao, &ativoInt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to scan plan: %w", err)
	}
	p.Ativo = ativoInt == 1
	return &p, nil
}
