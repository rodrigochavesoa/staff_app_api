package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"staff_app/internal/domain"
)

type PreRegistroRepository struct {
	db *DB
}

func NewPreRegistroRepository(db *DB) *PreRegistroRepository {
	return &PreRegistroRepository{db: db}
}

func (r *PreRegistroRepository) Create(ctx context.Context, p *domain.PreRegistro) error {
	query := `
		INSERT INTO pre_registros (
			nome, email, telefone, data_nascimento, genero, payment_ref, 
			plano_id, plano_valor, ip_origem, user_agent, expira_em, 
			criado_em, usado, status, aprovado_por, aprovado_em, 
			aluno_id_criado, motivo_rejeicao
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	expStr := p.ExpiraEm.Format("2006-01-02 15:04:05")
	criStr := p.CriadoEm.Format("2006-01-02 15:04:05")

	var apEmStr *string
	if p.AprovadoEm != nil {
		s := p.AprovadoEm.Format("2006-01-02 15:04:05")
		apEmStr = &s
	}

	usadoVal := 0
	if p.Usado {
		usadoVal = 1
	}

	result, err := r.db.ExecContext(ctx, query,
		p.Nome, p.Email, p.Telefone, p.DataNascimento, p.Genero, p.PaymentRef,
		p.PlanoID, p.PlanoValor, p.IpOrigem, p.UserAgent, expStr,
		criStr, usadoVal, p.Status, p.AprovadoPor, apEmStr,
		p.AlunoIDCriado, p.MotivoRejeicao,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = id
	return nil
}

func (r *PreRegistroRepository) FindByID(ctx context.Context, id int64) (*domain.PreRegistro, error) {
	query := `
		SELECT 
			id, nome, email, telefone, data_nascimento, genero, payment_ref, 
			plano_id, plano_valor, ip_origem, user_agent, expira_em, 
			criado_em, usado, status, aprovado_por, aprovado_em, 
			aluno_id_criado, motivo_rejeicao
		FROM pre_registros
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var p domain.PreRegistro
	var expStr, criStr string
	var apEmStr *string
	var usadoVal int

	err := row.Scan(
		&p.ID, &p.Nome, &p.Email, &p.Telefone, &p.DataNascimento, &p.Genero, &p.PaymentRef,
		&p.PlanoID, &p.PlanoValor, &p.IpOrigem, &p.UserAgent, &expStr,
		&criStr, &usadoVal, &p.Status, &p.AprovadoPor, &apEmStr,
		&p.AlunoIDCriado, &p.MotivoRejeicao,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	p.Usado = usadoVal == 1
	p.ExpiraEm, _ = parseDateTime(expStr)
	p.CriadoEm, _ = parseDateTime(criStr)
	if apEmStr != nil {
		t, _ := parseDateTime(*apEmStr)
		p.AprovadoEm = &t
	}

	return &p, nil
}

func (r *PreRegistroRepository) FindByEmail(ctx context.Context, email string) (*domain.PreRegistro, error) {
	query := `
		SELECT 
			id, nome, email, telefone, data_nascimento, genero, payment_ref, 
			plano_id, plano_valor, ip_origem, user_agent, expira_em, 
			criado_em, usado, status, aprovado_por, aprovado_em, 
			aluno_id_criado, motivo_rejeicao
		FROM pre_registros
		WHERE email = ? AND (status = 'aguardando_aprovacao' OR status = 'aprovado')
		ORDER BY criado_em DESC LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, email)

	var p domain.PreRegistro
	var expStr, criStr string
	var apEmStr *string
	var usadoVal int

	err := row.Scan(
		&p.ID, &p.Nome, &p.Email, &p.Telefone, &p.DataNascimento, &p.Genero, &p.PaymentRef,
		&p.PlanoID, &p.PlanoValor, &p.IpOrigem, &p.UserAgent, &expStr,
		&criStr, &usadoVal, &p.Status, &p.AprovadoPor, &apEmStr,
		&p.AlunoIDCriado, &p.MotivoRejeicao,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	p.Usado = usadoVal == 1
	p.ExpiraEm, _ = parseDateTime(expStr)
	p.CriadoEm, _ = parseDateTime(criStr)
	if apEmStr != nil {
		t, _ := parseDateTime(*apEmStr)
		p.AprovadoEm = &t
	}

	return &p, nil
}

func (r *PreRegistroRepository) List(ctx context.Context, status string, nomeQuery string) ([]domain.PreRegistro, error) {
	var queryParts []string
	var args []any

	queryParts = append(queryParts, `
		SELECT 
			id, nome, email, telefone, data_nascimento, genero, payment_ref, 
			plano_id, plano_valor, ip_origem, user_agent, expira_em, 
			criado_em, usado, status, aprovado_por, aprovado_em, 
			aluno_id_criado, motivo_rejeicao
		FROM pre_registros
		WHERE 1=1
	`)

	if status != "" {
		queryParts = append(queryParts, "AND status = ?")
		args = append(args, status)
	}

	if nomeQuery != "" {
		queryParts = append(queryParts, "AND nome LIKE ?")
		args = append(args, "%"+nomeQuery+"%")
	}

	queryParts = append(queryParts, "ORDER BY criado_em DESC")

	query := strings.Join(queryParts, " ")
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.PreRegistro
	for rows.Next() {
		var p domain.PreRegistro
		var expStr, criStr string
		var apEmStr *string
		var usadoVal int

		err := rows.Scan(
			&p.ID, &p.Nome, &p.Email, &p.Telefone, &p.DataNascimento, &p.Genero, &p.PaymentRef,
			&p.PlanoID, &p.PlanoValor, &p.IpOrigem, &p.UserAgent, &expStr,
			&criStr, &usadoVal, &p.Status, &p.AprovadoPor, &apEmStr,
			&p.AlunoIDCriado, &p.MotivoRejeicao,
		)
		if err != nil {
			return nil, err
		}

		p.Usado = usadoVal == 1
		p.ExpiraEm, _ = parseDateTime(expStr)
		p.CriadoEm, _ = parseDateTime(criStr)
		if apEmStr != nil {
			t, _ := parseDateTime(*apEmStr)
			p.AprovadoEm = &t
		}

		list = append(list, p)
	}
	return list, nil
}

func (r *PreRegistroRepository) Update(ctx context.Context, p *domain.PreRegistro) error {
	query := `
		UPDATE pre_registros SET
			usado = ?,
			status = ?,
			aprovado_por = ?,
			aprovado_em = ?,
			aluno_id_criado = ?,
			motivo_rejeicao = ?
		WHERE id = ?
	`
	var apEmStr *string
	if p.AprovadoEm != nil {
		s := p.AprovadoEm.Format("2006-01-02 15:04:05")
		apEmStr = &s
	}

	usadoVal := 0
	if p.Usado {
		usadoVal = 1
	}

	_, err := r.db.ExecContext(ctx, query,
		usadoVal, p.Status, p.AprovadoPor, apEmStr, p.AlunoIDCriado, p.MotivoRejeicao,
		p.ID,
	)
	return err
}

func (r *PreRegistroRepository) AddAudit(ctx context.Context, a *domain.PreRegistroAudit) error {
	query := `
		INSERT INTO pre_registros_audit (
			pre_registro_id, evento, usuario_id, detalhes, ip_origem, user_agent, criado_em
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	criStr := a.CriadoEm.Format("2006-01-02 15:04:05")
	result, err := r.db.ExecContext(ctx, query,
		a.PreRegistroID, a.Evento, a.UsuarioID, a.Detalhes, a.IpOrigem, a.UserAgent, criStr,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	a.ID = id
	return nil
}

func (r *PreRegistroRepository) GetAuditTrail(ctx context.Context, preRegistroID int64) ([]domain.PreRegistroAudit, error) {
	query := `
		SELECT 
			id, pre_registro_id, evento, usuario_id, detalhes, ip_origem, user_agent, criado_em
		FROM pre_registros_audit
		WHERE pre_registro_id = ?
		ORDER BY criado_em DESC
	`
	rows, err := r.db.QueryContext(ctx, query, preRegistroID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trail []domain.PreRegistroAudit
	for rows.Next() {
		var a domain.PreRegistroAudit
		var criStr string
		err := rows.Scan(
			&a.ID, &a.PreRegistroID, &a.Evento, &a.UsuarioID, &a.Detalhes, &a.IpOrigem, &a.UserAgent, &criStr,
		)
		if err != nil {
			return nil, err
		}
		a.CriadoEm, _ = time.Parse("2006-01-02 15:04:05", criStr)
		trail = append(trail, a)
	}
	return trail, nil
}
