package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"staff_app/internal/domain"
)

type AlunoRepository struct {
	db *DB
}

func NewAlunoRepository(db *DB) *AlunoRepository {
	return &AlunoRepository{db: db}
}

func (r *AlunoRepository) Create(ctx context.Context, a *domain.Aluno) error {
	query := `
		INSERT INTO alunos (
			nome, idade, sexo, email, telefone, objetivo, exclusoes_permanentes, turma, usuario_id,
			plano_id, plano_valor, plano_pago, plano_ativo, plano_inicio, plano_fim,
			cadastro_aprovado, cadastro_aprovado_por, cadastro_aprovado_em, pre_registro_id, ativo
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	planoPagoInt := 0
	if a.PlanoPago {
		planoPagoInt = 1
	}

	planoAtivoInt := 0
	if a.PlanoAtivo {
		planoAtivoInt = 1
	}

	cadastroAprovadoInt := 0
	if a.CadastroAprovado {
		cadastroAprovadoInt = 1
	}

	var cadastroAprovadoEmStr *string
	if a.CadastroAprovadoEm != nil {
		str := a.CadastroAprovadoEm.Format(time.RFC3339)
		cadastroAprovadoEmStr = &str
	}

	ativoInt := 0
	if a.Ativo {
		ativoInt = 1
	}

	res, err := r.db.ExecContext(ctx, query,
		a.Nome,
		a.Idade,
		a.Sexo,
		a.Email,
		a.Telefone,
		a.Objetivo,
		a.ExclusoesPermanentes,
		a.Turma,
		a.UsuarioID,
		a.PlanoID,
		a.PlanoValor,
		planoPagoInt,
		planoAtivoInt,
		a.PlanoInicio,
		a.PlanoFim,
		cadastroAprovadoInt,
		a.CadastroAprovadoPor,
		cadastroAprovadoEmStr,
		a.PreRegistroID,
		ativoInt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert aluno: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to retrieve last insert id: %w", err)
	}
	a.ID = id

	return nil
}

func (r *AlunoRepository) GetByID(ctx context.Context, id int64) (*domain.Aluno, error) {
	query := `
		SELECT 
			id, nome, idade, sexo, email, telefone, objetivo, exclusoes_permanentes, turma, usuario_id,
			plano_id, plano_valor, plano_pago, plano_ativo, plano_inicio, plano_fim,
			cadastro_aprovado, cadastro_aprovado_por, cadastro_aprovado_em, pre_registro_id, ativo
		FROM alunos
		WHERE id = ?
	`

	var a domain.Aluno
	var planoPagoInt, planoAtivoInt, cadastroAprovadoInt, ativoInt int
	var cadastroAprovadoEmStr sql.NullString
	var telNull, objNull, exclNull, turmaNull sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&a.ID, &a.Nome, &a.Idade, &a.Sexo, &a.Email, &telNull, &objNull, &exclNull, &turmaNull, &a.UsuarioID,
		&a.PlanoID, &a.PlanoValor, &planoPagoInt, &planoAtivoInt, &a.PlanoInicio, &a.PlanoFim,
		&cadastroAprovadoInt, &a.CadastroAprovadoPor, &cadastroAprovadoEmStr, &a.PreRegistroID, &ativoInt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to get aluno: %w", err)
	}

	a.PlanoPago = planoPagoInt == 1
	a.PlanoAtivo = planoAtivoInt == 1
	a.CadastroAprovado = cadastroAprovadoInt == 1
	a.Ativo = ativoInt == 1

	a.Telefone = telNull.String
	a.Objetivo = objNull.String
	a.ExclusoesPermanentes = exclNull.String
	a.Turma = turmaNull.String

	if cadastroAprovadoEmStr.Valid {
		if t, err := time.Parse(time.RFC3339, cadastroAprovadoEmStr.String); err == nil {
			a.CadastroAprovadoEm = &t
		} else if t, err := time.Parse("2006-01-02 15:04:05", cadastroAprovadoEmStr.String); err == nil {
			a.CadastroAprovadoEm = &t
		}
	}

	return &a, nil
}

func (r *AlunoRepository) GetByEmail(ctx context.Context, email string) (*domain.Aluno, error) {
	query := `
		SELECT 
			id, nome, idade, sexo, email, telefone, objetivo, exclusoes_permanentes, turma, usuario_id,
			plano_id, plano_valor, plano_pago, plano_ativo, plano_inicio, plano_fim,
			cadastro_aprovado, cadastro_aprovado_por, cadastro_aprovado_em, pre_registro_id, ativo
		FROM alunos
		WHERE email = ? AND ativo = 1
		LIMIT 1
	`

	var a domain.Aluno
	var planoPagoInt, planoAtivoInt, cadastroAprovadoInt, ativoInt int
	var cadastroAprovadoEmStr sql.NullString
	var telNull, objNull, exclNull, turmaNull sql.NullString

	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&a.ID, &a.Nome, &a.Idade, &a.Sexo, &a.Email, &telNull, &objNull, &exclNull, &turmaNull, &a.UsuarioID,
		&a.PlanoID, &a.PlanoValor, &planoPagoInt, &planoAtivoInt, &a.PlanoInicio, &a.PlanoFim,
		&cadastroAprovadoInt, &a.CadastroAprovadoPor, &cadastroAprovadoEmStr, &a.PreRegistroID, &ativoInt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get aluno by email: %w", err)
	}

	a.PlanoPago = planoPagoInt == 1
	a.PlanoAtivo = planoAtivoInt == 1
	a.CadastroAprovado = cadastroAprovadoInt == 1
	a.Ativo = ativoInt == 1

	a.Telefone = telNull.String
	a.Objetivo = objNull.String
	a.ExclusoesPermanentes = exclNull.String
	a.Turma = turmaNull.String

	if cadastroAprovadoEmStr.Valid {
		if t, err := time.Parse(time.RFC3339, cadastroAprovadoEmStr.String); err == nil {
			a.CadastroAprovadoEm = &t
		} else if t, err := time.Parse("2006-01-02 15:04:05", cadastroAprovadoEmStr.String); err == nil {
			a.CadastroAprovadoEm = &t
		}
	}

	return &a, nil
}

// GetByUsuarioID returns the aluno linked to the given user ID, or (nil, nil) if none.
func (r *AlunoRepository) GetByUsuarioID(ctx context.Context, userID int64) (*domain.Aluno, error) {
	query := `
		SELECT 
			id, nome, idade, sexo, email, telefone, objetivo, exclusoes_permanentes, turma, usuario_id,
			plano_id, plano_valor, plano_pago, plano_ativo, plano_inicio, plano_fim,
			cadastro_aprovado, cadastro_aprovado_por, cadastro_aprovado_em, pre_registro_id, ativo
		FROM alunos
		WHERE usuario_id = ?
		LIMIT 1
	`

	var a domain.Aluno
	var planoPagoInt, planoAtivoInt, cadastroAprovadoInt, ativoInt int
	var cadastroAprovadoEmStr sql.NullString
	var telNull, objNull, exclNull, turmaNull sql.NullString

	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&a.ID, &a.Nome, &a.Idade, &a.Sexo, &a.Email, &telNull, &objNull, &exclNull, &turmaNull, &a.UsuarioID,
		&a.PlanoID, &a.PlanoValor, &planoPagoInt, &planoAtivoInt, &a.PlanoInicio, &a.PlanoFim,
		&cadastroAprovadoInt, &a.CadastroAprovadoPor, &cadastroAprovadoEmStr, &a.PreRegistroID, &ativoInt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get aluno by usuario_id: %w", err)
	}

	a.PlanoPago = planoPagoInt == 1
	a.PlanoAtivo = planoAtivoInt == 1
	a.CadastroAprovado = cadastroAprovadoInt == 1
	a.Ativo = ativoInt == 1

	a.Telefone = telNull.String
	a.Objetivo = objNull.String
	a.ExclusoesPermanentes = exclNull.String
	a.Turma = turmaNull.String

	if cadastroAprovadoEmStr.Valid {
		if t, err := time.Parse(time.RFC3339, cadastroAprovadoEmStr.String); err == nil {
			a.CadastroAprovadoEm = &t
		} else if t, err := time.Parse("2006-01-02 15:04:05", cadastroAprovadoEmStr.String); err == nil {
			a.CadastroAprovadoEm = &t
		}
	}

	return &a, nil
}

func (r *AlunoRepository) List(ctx context.Context, busca string, includeInactives bool) ([]*domain.Aluno, error) {
	var queryBuilder strings.Builder
	queryBuilder.WriteString(`
		SELECT 
			id, nome, idade, sexo, email, telefone, objetivo, exclusoes_permanentes, turma, usuario_id,
			plano_id, plano_valor, plano_pago, plano_ativo, plano_inicio, plano_fim,
			cadastro_aprovado, cadastro_aprovado_por, cadastro_aprovado_em, pre_registro_id, ativo
		FROM alunos
	`)

	var args []any
	conditions := make([]string, 0)

	if busca != "" {
		busca = strings.TrimSpace(busca)
		searchCondition := "(nome LIKE ? OR email LIKE ? OR objetivo LIKE ?"
		args = append(args, "%"+busca+"%", "%"+busca+"%", "%"+busca+"%")

		if num, err := strconvParseInt64(busca); err == nil {
			searchCondition += " OR id = ?"
			args = append(args, num)
		}
		searchCondition += ")"
		conditions = append(conditions, searchCondition)
	}

	if !includeInactives {
		conditions = append(conditions, "ativo = 1")
	}

	if len(conditions) > 0 {
		queryBuilder.WriteString(" WHERE ")
		queryBuilder.WriteString(strings.Join(conditions, " AND "))
	}

	if includeInactives {
		queryBuilder.WriteString(" ORDER BY ativo DESC, nome ASC")
	} else {
		queryBuilder.WriteString(" ORDER BY nome ASC")
	}

	rows, err := r.db.QueryContext(ctx, queryBuilder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list alunos: %w", err)
	}
	defer rows.Close()

	var alunos []*domain.Aluno
	for rows.Next() {
		var a domain.Aluno
		var planoPagoInt, planoAtivoInt, cadastroAprovadoInt, ativoInt int
		var cadastroAprovadoEmStr sql.NullString
		var telNull, objNull, exclNull, turmaNull sql.NullString

		err := rows.Scan(
			&a.ID, &a.Nome, &a.Idade, &a.Sexo, &a.Email, &telNull, &objNull, &exclNull, &turmaNull, &a.UsuarioID,
			&a.PlanoID, &a.PlanoValor, &planoPagoInt, &planoAtivoInt, &a.PlanoInicio, &a.PlanoFim,
			&cadastroAprovadoInt, &a.CadastroAprovadoPor, &cadastroAprovadoEmStr, &a.PreRegistroID, &ativoInt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan aluno row: %w", err)
		}

		a.PlanoPago = planoPagoInt == 1
		a.PlanoAtivo = planoAtivoInt == 1
		a.CadastroAprovado = cadastroAprovadoInt == 1
		a.Ativo = ativoInt == 1

		a.Telefone = telNull.String
		a.Objetivo = objNull.String
		a.ExclusoesPermanentes = exclNull.String
		a.Turma = turmaNull.String

		if cadastroAprovadoEmStr.Valid {
			if t, err := time.Parse(time.RFC3339, cadastroAprovadoEmStr.String); err == nil {
				a.CadastroAprovadoEm = &t
			} else if t, err := time.Parse("2006-01-02 15:04:05", cadastroAprovadoEmStr.String); err == nil {
				a.CadastroAprovadoEm = &t
			}
		}

		alunos = append(alunos, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return alunos, nil
}

func (r *AlunoRepository) Update(ctx context.Context, a *domain.Aluno) error {
	query := `
		UPDATE alunos
		SET nome = ?, idade = ?, sexo = ?, email = ?, telefone = ?, objetivo = ?,
		    exclusoes_permanentes = ?, turma = ?, usuario_id = ?, plano_id = ?, plano_valor = ?,
		    plano_pago = ?, plano_ativo = ?, plano_inicio = ?, plano_fim = ?,
		    cadastro_aprovado = ?, cadastro_aprovado_por = ?, cadastro_aprovado_em = ?,
		    pre_registro_id = ?, ativo = ?
		WHERE id = ?
	`

	planoPagoInt := 0
	if a.PlanoPago {
		planoPagoInt = 1
	}

	planoAtivoInt := 0
	if a.PlanoAtivo {
		planoAtivoInt = 1
	}

	cadastroAprovadoInt := 0
	if a.CadastroAprovado {
		cadastroAprovadoInt = 1
	}

	var cadastroAprovadoEmStr *string
	if a.CadastroAprovadoEm != nil {
		str := a.CadastroAprovadoEm.Format(time.RFC3339)
		cadastroAprovadoEmStr = &str
	}

	ativoInt := 0
	if a.Ativo {
		ativoInt = 1
	}

	res, err := r.db.ExecContext(ctx, query,
		a.Nome,
		a.Idade,
		a.Sexo,
		a.Email,
		a.Telefone,
		a.Objetivo,
		a.ExclusoesPermanentes,
		a.Turma,
		a.UsuarioID,
		a.PlanoID,
		a.PlanoValor,
		planoPagoInt,
		planoAtivoInt,
		a.PlanoInicio,
		a.PlanoFim,
		cadastroAprovadoInt,
		a.CadastroAprovadoPor,
		cadastroAprovadoEmStr,
		a.PreRegistroID,
		ativoInt,
		a.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update aluno: %w", err)
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

// Delete desativa o aluno (exclusão lógica).
func (r *AlunoRepository) Delete(ctx context.Context, id int64) error {
	query := `UPDATE alunos SET ativo = 0 WHERE id = ? AND ativo = 1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to soft delete aluno: %w", err)
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

func (r *AlunoRepository) Reactivate(ctx context.Context, id int64) error {
	query := `UPDATE alunos SET ativo = 1 WHERE id = ? AND ativo = 0`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to reactivate aluno: %w", err)
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

func strconvParseInt64(s string) (int64, error) {
	var val int64
	_, err := fmt.Sscanf(s, "%d", &val)
	return val, err
}
