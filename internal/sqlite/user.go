package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"staff_app/internal/domain"
)

type UserRepository struct {
	db *DB
}

func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}
	return count, nil
}

func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	query := `
		INSERT INTO users (username, email, password_hash, nome_completo, is_admin, ativo, aprovado)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	res, err := r.db.ExecContext(ctx, query,
		u.Username,
		u.Email,
		u.PasswordHash,
		u.NomeCompleto,
		boolToInt(u.IsAdmin),
		boolToInt(u.Ativo),
		boolToInt(u.Aprovado),
	)
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to retrieve user id: %w", err)
	}
	u.ID = id
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	return r.getOne(ctx, `
		SELECT id, username, email, password_hash, COALESCE(nome_completo, ''), is_admin,
		       criado_em, ultimo_login, ativo, COALESCE(aprovado, 0), foto_perfil
		FROM users
		WHERE id = ?
	`, id)
}

func (r *UserRepository) GetByLogin(ctx context.Context, login string) (*domain.User, error) {
	return r.getOne(ctx, `
		SELECT id, username, email, password_hash, COALESCE(nome_completo, ''), is_admin,
		       criado_em, ultimo_login, ativo, COALESCE(aprovado, 0), foto_perfil
		FROM users
		WHERE username = ? OR email = ?
	`, login, login)
}

func (r *UserRepository) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, email, password_hash, COALESCE(nome_completo, ''), is_admin,
		       criado_em, ultimo_login, ativo, COALESCE(aprovado, 0), foto_perfil
		FROM users
		ORDER BY aprovado ASC, criado_em DESC, username ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during user rows iteration: %w", err)
	}
	return users, nil
}

func (r *UserRepository) UpdateLastLogin(ctx context.Context, id int64, t time.Time) error {
	if _, err := r.db.ExecContext(ctx, "UPDATE users SET ultimo_login = ? WHERE id = ?", t.UTC().Format(time.RFC3339), id); err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	res, err := r.db.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, id)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	return requireAffected(res, sql.ErrNoRows)
}

func (r *UserRepository) Approve(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "UPDATE users SET aprovado = 1, ativo = 1 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to approve user: %w", err)
	}
	return requireAffected(res, sql.ErrNoRows)
}

func (r *UserRepository) RejectPending(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM users WHERE id = ? AND COALESCE(aprovado, 0) = 0", id)
	if err != nil {
		return fmt.Errorf("failed to reject pending user: %w", err)
	}
	return requireAffected(res, sql.ErrNoRows)
}

func (r *UserRepository) ToggleActive(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "UPDATE users SET ativo = CASE ativo WHEN 1 THEN 0 ELSE 1 END WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to toggle user active status: %w", err)
	}
	return requireAffected(res, sql.ErrNoRows)
}

func (r *UserRepository) getOne(ctx context.Context, query string, args ...any) (*domain.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, query, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return u, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*domain.User, error) {
	var u domain.User
	var isAdminInt, ativoInt, aprovadoInt int
	var criadoEmRaw string
	var ultimoLogin sql.NullString
	var fotoPerfil sql.NullString

	if err := row.Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.PasswordHash,
		&u.NomeCompleto,
		&isAdminInt,
		&criadoEmRaw,
		&ultimoLogin,
		&ativoInt,
		&aprovadoInt,
		&fotoPerfil,
	); err != nil {
		return nil, err
	}

	u.IsAdmin = isAdminInt == 1
	u.Ativo = ativoInt == 1
	u.Aprovado = aprovadoInt == 1
	if fotoPerfil.Valid {
		u.FotoPerfil = &fotoPerfil.String
	}
	u.CriadoEm = parseSQLiteTime(criadoEmRaw)
	if ultimoLogin.Valid {
		t := parseSQLiteTime(ultimoLogin.String)
		if !t.IsZero() {
			u.UltimoLogin = &t
		}
	}

	return &u, nil
}

func parseSQLiteTime(value string) time.Time {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func requireAffected(res sql.Result, notFound error) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect affected rows: %w", err)
	}
	if affected == 0 {
		return notFound
	}
	return nil
}
