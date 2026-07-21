package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"staff_app/internal/domain"
)

type FichaWebRepository struct {
	db *DB
}

func NewFichaWebRepository(db *DB) *FichaWebRepository {
	return &FichaWebRepository{db: db}
}

func (r *FichaWebRepository) GetFichaJSON(ctx context.Context, id int64) (string, error) {
	var jsonStr sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT ficha_json FROM fichas_treino_web WHERE id = ?", id).Scan(&jsonStr)
	if err != nil {
		return "", err
	}
	return jsonStr.String, nil
}

// Create cria o link público e desativa links ativos da mesma ficha.
func (r *FichaWebRepository) Create(ctx context.Context, fw *domain.FichaWeb) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, "UPDATE fichas_web SET ativo = 0 WHERE ficha_id = ? AND ativo = 1", fw.FichaID)
	if err != nil {
		return fmt.Errorf("failed to deactivate previous active links: %w", err)
	}

	query := `
		INSERT INTO fichas_web (
			hash, ficha_id, aluno_id, user_id, conteudo_json, criado_em, expira_em, acessos, ativo, renovado_de
		) VALUES (?, ?, ?, ?, ?, ?, ?, 0, 1, ?)
	`

	criadoEmStr := fw.CriadoEm.Format(time.RFC3339)
	expiraEmStr := fw.ExpiraEm.Format(time.RFC3339)

	res, err := tx.ExecContext(ctx, query,
		fw.Hash,
		fw.FichaID,
		fw.AlunoID,
		fw.UserID,
		fw.ConteudoJSON,
		criadoEmStr,
		expiraEmStr,
		fw.RenovadoDe,
	)
	if err != nil {
		return fmt.Errorf("failed to insert public link: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	fw.ID = id

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *FichaWebRepository) GetByHash(ctx context.Context, hash string) (*domain.FichaWeb, error) {
	query := `
		SELECT 
			id, hash, ficha_id, aluno_id, user_id, conteudo_json, criado_em, expira_em, acessos, ultimo_acesso, ativo, renovado_de
		FROM fichas_web
		WHERE hash = ?
	`

	var fw domain.FichaWeb
	var criadoEmStr, expiraEmStr string
	var ultimoAcessoStr sql.NullString
	var ativoInt int

	err := r.db.QueryRowContext(ctx, query, hash).Scan(
		&fw.ID, &fw.Hash, &fw.FichaID, &fw.AlunoID, &fw.UserID, &fw.ConteudoJSON,
		&criadoEmStr, &expiraEmStr, &fw.Acessos, &ultimoAcessoStr, &ativoInt, &fw.RenovadoDe,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to get public link: %w", err)
	}

	fw.Ativo = ativoInt == 1

	if t, err := time.Parse(time.RFC3339, criadoEmStr); err == nil {
		fw.CriadoEm = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", criadoEmStr); err == nil {
		fw.CriadoEm = t
	}

	if t, err := time.Parse(time.RFC3339, expiraEmStr); err == nil {
		fw.ExpiraEm = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", expiraEmStr); err == nil {
		fw.ExpiraEm = t
	}

	if ultimoAcessoStr.Valid {
		if t, err := time.Parse(time.RFC3339, ultimoAcessoStr.String); err == nil {
			fw.UltimoAcesso = &t
		} else if t, err := time.Parse("2006-01-02 15:04:05", ultimoAcessoStr.String); err == nil {
			fw.UltimoAcesso = &t
		}
	}

	return &fw, nil
}

func (r *FichaWebRepository) IncrementAccessCount(ctx context.Context, hash string, userAgent, ipAddress string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	nowStr := now.Format(time.RFC3339)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO fichas_web_acessos (hash, data_acesso, user_agent, ip_address)
		VALUES (?, ?, ?, ?)
	`, hash, nowStr, userAgent, ipAddress)
	if err != nil {
		return fmt.Errorf("failed to insert access record: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE fichas_web 
		SET acessos = acessos + 1, ultimo_acesso = ?
		WHERE hash = ?
	`, nowStr, hash)
	if err != nil {
		return fmt.Errorf("failed to update access counter: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *FichaWebRepository) GetStats(ctx context.Context, hash string) (*domain.FichaWebStats, error) {
	fw, err := r.GetByHash(ctx, hash)
	if err != nil {
		return nil, err
	}

	stats := &domain.FichaWebStats{
		Hash:             fw.Hash,
		CriadoEm:         fw.CriadoEm,
		ExpiraEm:         fw.ExpiraEm,
		Acessos:          fw.Acessos,
		Ativo:            fw.Ativo,
		HistoricoAcessos: []*domain.FichaWebAcesso{},
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, hash, data_acesso, user_agent, ip_address
		FROM fichas_web_acessos
		WHERE hash = ?
		ORDER BY data_acesso DESC
		LIMIT 20
	`, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to query accesses: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var a domain.FichaWebAcesso
		var dataStr string

		err := rows.Scan(&a.ID, &a.Hash, &dataStr, &a.UserAgent, &a.IPAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to scan access row: %w", err)
		}

		if t, err := time.Parse(time.RFC3339, dataStr); err == nil {
			a.DataAcesso = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", dataStr); err == nil {
			a.DataAcesso = t
		}

		stats.HistoricoAcessos = append(stats.HistoricoAcessos, &a)
	}

	return stats, nil
}

// Renew atualiza a validade e, se houver, o conteúdo JSON do link público.
func (r *FichaWebRepository) Renew(ctx context.Context, hash string, newExpiration time.Time, newContent *string) error {
	newExpirationStr := newExpiration.Format(time.RFC3339)

	var res sql.Result
	var err error

	if newContent != nil {
		res, err = r.db.ExecContext(ctx, `
			UPDATE fichas_web
			SET expira_em = ?, conteudo_json = ?
			WHERE hash = ? AND ativo = 1
		`, newExpirationStr, *newContent, hash)
	} else {
		res, err = r.db.ExecContext(ctx, `
			UPDATE fichas_web
			SET expira_em = ?
			WHERE hash = ? AND ativo = 1
		`, newExpirationStr, hash)
	}

	if err != nil {
		return fmt.Errorf("failed to renew public link: %w", err)
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

// Deactivate desativa o link público (exclusão lógica).
func (r *FichaWebRepository) Deactivate(ctx context.Context, hash string) error {
	res, err := r.db.ExecContext(ctx, "UPDATE fichas_web SET ativo = 0 WHERE hash = ? AND ativo = 1", hash)
	if err != nil {
		return fmt.Errorf("failed to deactivate public link: %w", err)
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

func (r *FichaWebRepository) ListByAlunoID(ctx context.Context, alunoID int64, includeExpired bool) ([]*domain.FichaWeb, error) {
	var query string
	var args []any

	if includeExpired {
		query = `
			SELECT 
				id, hash, ficha_id, aluno_id, user_id, conteudo_json, criado_em, expira_em, acessos, ultimo_acesso, ativo, renovado_de
			FROM fichas_web
			WHERE aluno_id = ?
			ORDER BY criado_em DESC
		`
		args = []any{alunoID}
	} else {
		query = `
			SELECT 
				id, hash, ficha_id, aluno_id, user_id, conteudo_json, criado_em, expira_em, acessos, ultimo_acesso, ativo, renovado_de
			FROM fichas_web
			WHERE aluno_id = ? AND expira_em > ? AND ativo = 1
			ORDER BY criado_em DESC
		`
		args = []any{alunoID, time.Now().Format(time.RFC3339)}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query public links by student: %w", err)
	}
	defer rows.Close()

	var list []*domain.FichaWeb
	for rows.Next() {
		var fw domain.FichaWeb
		var criadoEmStr, expiraEmStr string
		var ultimoAcessoStr sql.NullString
		var ativoInt int

		err := rows.Scan(
			&fw.ID, &fw.Hash, &fw.FichaID, &fw.AlunoID, &fw.UserID, &fw.ConteudoJSON,
			&criadoEmStr, &expiraEmStr, &fw.Acessos, &ultimoAcessoStr, &ativoInt, &fw.RenovadoDe,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		fw.Ativo = ativoInt == 1

		if t, err := time.Parse(time.RFC3339, criadoEmStr); err == nil {
			fw.CriadoEm = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", criadoEmStr); err == nil {
			fw.CriadoEm = t
		}

		if t, err := time.Parse(time.RFC3339, expiraEmStr); err == nil {
			fw.ExpiraEm = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", expiraEmStr); err == nil {
			fw.ExpiraEm = t
		}

		if ultimoAcessoStr.Valid {
			if t, err := time.Parse(time.RFC3339, ultimoAcessoStr.String); err == nil {
				fw.UltimoAcesso = &t
			} else if t, err := time.Parse("2006-01-02 15:04:05", ultimoAcessoStr.String); err == nil {
				fw.UltimoAcesso = &t
			}
		}

		list = append(list, &fw)
	}

	return list, nil
}
