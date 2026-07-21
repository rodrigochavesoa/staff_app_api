package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"staff_app/internal/domain"
)

type AnamneseRepository struct {
	db *DB
}

func NewAnamneseRepository(db *DB) *AnamneseRepository {
	return &AnamneseRepository{db: db}
}

func (r *AnamneseRepository) Create(ctx context.Context, a *domain.Anamnese) error {
	query := `
		INSERT INTO anamneses (
			aluno_id, data_nascimento, idade, sexo, altura, peso, telefone, email,
			patologias, medicamentos, lesoes_atuais, dores_cronicas,
			parq_doenca_cardiaca, parq_dor_peito, parq_tontura, parq_problema_osseo,
			parq_medicamento_pressao, parq_impedimento_activity, experiencia_treino,
			objetivo_principal, contato_emergencia_nome, contato_emergencia_telefone,
			risk_score_cached, preenchido_por, ativa, criado_em, pre_registro_id,
			status_aprovacao, aprovado_por, aprovado_em, motivo_rejeicao, token_origem
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	criStr := a.CriadoEm.Format("2006-01-02 15:04:05")
	var apEmStr *string
	if a.AprovadoEm != nil {
		s := a.AprovadoEm.Format("2006-01-02 15:04:05")
		apEmStr = &s
	}

	ativaVal := 0
	if a.Ativa {
		ativaVal = 1
	}

	result, err := r.db.ExecContext(ctx, query,
		a.AlunoID, a.DataNascimento, a.Idade, a.Sexo, a.Altura, a.Peso, a.Telefone, a.Email,
		a.Patologias, a.Medicamentos, a.LesoesAtuais, a.DoresCronicas,
		a.ParqDoencaCardiaca, a.ParqDorPeito, a.ParqTontura, a.ParqProblemaOsseo,
		a.ParqMedicamentoPressao, a.ParqImpedimentoActivity, a.ExperienciaTreino,
		a.ObjetivoPrincipal, a.ContatoEmergenciaNome, a.ContatoEmergenciaTelefone,
		a.RiskScoreCached, a.PreenchidoPor, ativaVal, criStr, a.PreRegistroID,
		a.StatusAprovacao, a.AprovadoPor, apEmStr, a.MotivoRejeicao, a.TokenOrigem,
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

func (r *AnamneseRepository) FindByID(ctx context.Context, id int64) (*domain.Anamnese, error) {
	query := `
		SELECT 
			id, aluno_id, data_nascimento, idade, sexo, altura, peso, telefone, email,
			patologias, medicamentos, lesoes_atuais, dores_cronicas,
			parq_doenca_cardiaca, parq_dor_peito, parq_tontura, parq_problema_osseo,
			parq_medicamento_pressao, parq_impedimento_activity, experiencia_treino,
			objetivo_principal, contato_emergencia_nome, contato_emergencia_telefone,
			risk_score_cached, preenchido_por, ativa, criado_em, pre_registro_id,
			status_aprovacao, aprovado_por, aprovado_em, motivo_rejeicao, token_origem
		FROM anamneses
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var a domain.Anamnese
	var criStr string
	var apEmStr *string
	var ativaVal int

	err := row.Scan(
		&a.ID, &a.AlunoID, &a.DataNascimento, &a.Idade, &a.Sexo, &a.Altura, &a.Peso, &a.Telefone, &a.Email,
		&a.Patologias, &a.Medicamentos, &a.LesoesAtuais, &a.DoresCronicas,
		&a.ParqDoencaCardiaca, &a.ParqDorPeito, &a.ParqTontura, &a.ParqProblemaOsseo,
		&a.ParqMedicamentoPressao, &a.ParqImpedimentoActivity, &a.ExperienciaTreino,
		&a.ObjetivoPrincipal, &a.ContatoEmergenciaNome, &a.ContatoEmergenciaTelefone,
		&a.RiskScoreCached, &a.PreenchidoPor, &ativaVal, &criStr, &a.PreRegistroID,
		&a.StatusAprovacao, &a.AprovadoPor, &apEmStr, &a.MotivoRejeicao, &a.TokenOrigem,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	a.Ativa = ativaVal == 1
	a.CriadoEm, _ = parseDateTime(criStr)
	if apEmStr != nil {
		t, _ := parseDateTime(*apEmStr)
		a.AprovadoEm = &t
	}

	return &a, nil
}

func (r *AnamneseRepository) FindActiveByAlunoID(ctx context.Context, alunoID int64) (*domain.Anamnese, error) {
	query := `
		SELECT 
			id, aluno_id, data_nascimento, idade, sexo, altura, peso, telefone, email,
			patologias, medicamentos, lesoes_atuais, dores_cronicas,
			parq_doenca_cardiaca, parq_dor_peito, parq_tontura, parq_problema_osseo,
			parq_medicamento_pressao, parq_impedimento_activity, experiencia_treino,
			objetivo_principal, contato_emergencia_nome, contato_emergencia_telefone,
			risk_score_cached, preenchido_por, ativa, criado_em, pre_registro_id,
			status_aprovacao, aprovado_por, aprovado_em, motivo_rejeicao, token_origem
		FROM anamneses
		WHERE aluno_id = ? AND ativa = 1
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, alunoID)

	var a domain.Anamnese
	var criStr string
	var apEmStr *string
	var ativaVal int

	err := row.Scan(
		&a.ID, &a.AlunoID, &a.DataNascimento, &a.Idade, &a.Sexo, &a.Altura, &a.Peso, &a.Telefone, &a.Email,
		&a.Patologias, &a.Medicamentos, &a.LesoesAtuais, &a.DoresCronicas,
		&a.ParqDoencaCardiaca, &a.ParqDorPeito, &a.ParqTontura, &a.ParqProblemaOsseo,
		&a.ParqMedicamentoPressao, &a.ParqImpedimentoActivity, &a.ExperienciaTreino,
		&a.ObjetivoPrincipal, &a.ContatoEmergenciaNome, &a.ContatoEmergenciaTelefone,
		&a.RiskScoreCached, &a.PreenchidoPor, &ativaVal, &criStr, &a.PreRegistroID,
		&a.StatusAprovacao, &a.AprovadoPor, &apEmStr, &a.MotivoRejeicao, &a.TokenOrigem,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	a.Ativa = ativaVal == 1
	a.CriadoEm, _ = parseDateTime(criStr)
	if apEmStr != nil {
		t, _ := parseDateTime(*apEmStr)
		a.AprovadoEm = &t
	}

	return &a, nil
}

func (r *AnamneseRepository) List(ctx context.Context) ([]domain.Anamnese, error) {
	query := `
		SELECT 
			id, aluno_id, data_nascimento, idade, sexo, altura, peso, telefone, email,
			patologias, medicamentos, lesoes_atuais, dores_cronicas,
			parq_doenca_cardiaca, parq_dor_peito, parq_tontura, parq_problema_osseo,
			parq_medicamento_pressao, parq_impedimento_activity, experiencia_treino,
			objetivo_principal, contato_emergencia_nome, contato_emergencia_telefone,
			risk_score_cached, preenchido_por, ativa, criado_em, pre_registro_id,
			status_aprovacao, aprovado_por, aprovado_em, motivo_rejeicao, token_origem
		FROM anamneses
		ORDER BY criado_em DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Anamnese
	for rows.Next() {
		var a domain.Anamnese
		var criStr string
		var apEmStr *string
		var ativaVal int

		err := rows.Scan(
			&a.ID, &a.AlunoID, &a.DataNascimento, &a.Idade, &a.Sexo, &a.Altura, &a.Peso, &a.Telefone, &a.Email,
			&a.Patologias, &a.Medicamentos, &a.LesoesAtuais, &a.DoresCronicas,
			&a.ParqDoencaCardiaca, &a.ParqDorPeito, &a.ParqTontura, &a.ParqProblemaOsseo,
			&a.ParqMedicamentoPressao, &a.ParqImpedimentoActivity, &a.ExperienciaTreino,
			&a.ObjetivoPrincipal, &a.ContatoEmergenciaNome, &a.ContatoEmergenciaTelefone,
			&a.RiskScoreCached, &a.PreenchidoPor, &ativaVal, &criStr, &a.PreRegistroID,
			&a.StatusAprovacao, &a.AprovadoPor, &apEmStr, &a.MotivoRejeicao, &a.TokenOrigem,
		)
		if err != nil {
			return nil, err
		}

		a.Ativa = ativaVal == 1
		a.CriadoEm, _ = parseDateTime(criStr)
		if apEmStr != nil {
			t, _ := parseDateTime(*apEmStr)
			a.AprovadoEm = &t
		}

		list = append(list, a)
	}
	return list, nil
}

func (r *AnamneseRepository) ListPending(ctx context.Context) ([]domain.Anamnese, error) {
	query := `
		SELECT 
			id, aluno_id, data_nascimento, idade, sexo, altura, peso, telefone, email,
			patologias, medicamentos, lesoes_atuais, dores_cronicas,
			parq_doenca_cardiaca, parq_dor_peito, parq_tontura, parq_problema_osseo,
			parq_medicamento_pressao, parq_impedimento_activity, experiencia_treino,
			objetivo_principal, contato_emergencia_nome, contato_emergencia_telefone,
			risk_score_cached, preenchido_por, ativa, criado_em, pre_registro_id,
			status_aprovacao, aprovado_por, aprovado_em, motivo_rejeicao, token_origem
		FROM anamneses
		WHERE status_aprovacao = 'pendente'
		ORDER BY criado_em DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Anamnese
	for rows.Next() {
		var a domain.Anamnese
		var criStr string
		var apEmStr *string
		var ativaVal int

		err := rows.Scan(
			&a.ID, &a.AlunoID, &a.DataNascimento, &a.Idade, &a.Sexo, &a.Altura, &a.Peso, &a.Telefone, &a.Email,
			&a.Patologias, &a.Medicamentos, &a.LesoesAtuais, &a.DoresCronicas,
			&a.ParqDoencaCardiaca, &a.ParqDorPeito, &a.ParqTontura, &a.ParqProblemaOsseo,
			&a.ParqMedicamentoPressao, &a.ParqImpedimentoActivity, &a.ExperienciaTreino,
			&a.ObjetivoPrincipal, &a.ContatoEmergenciaNome, &a.ContatoEmergenciaTelefone,
			&a.RiskScoreCached, &a.PreenchidoPor, &ativaVal, &criStr, &a.PreRegistroID,
			&a.StatusAprovacao, &a.AprovadoPor, &apEmStr, &a.MotivoRejeicao, &a.TokenOrigem,
		)
		if err != nil {
			return nil, err
		}

		a.Ativa = ativaVal == 1
		a.CriadoEm, _ = parseDateTime(criStr)
		if apEmStr != nil {
			t, _ := parseDateTime(*apEmStr)
			a.AprovadoEm = &t
		}

		list = append(list, a)
	}
	return list, nil
}

func (r *AnamneseRepository) Update(ctx context.Context, a *domain.Anamnese) error {
	query := `
		UPDATE anamneses SET
			ativa = ?,
			status_aprovacao = ?,
			aprovado_por = ?,
			aprovado_em = ?,
			motivo_rejeicao = ?
		WHERE id = ?
	`
	var apEmStr *string
	if a.AprovadoEm != nil {
		s := a.AprovadoEm.Format("2006-01-02 15:04:05")
		apEmStr = &s
	}

	ativaVal := 0
	if a.Ativa {
		ativaVal = 1
	}

	_, err := r.db.ExecContext(ctx, query,
		ativaVal, a.StatusAprovacao, a.AprovadoPor, apEmStr, a.MotivoRejeicao,
		a.ID,
	)
	return err
}

func (r *AnamneseRepository) DeactivateAllPreviousForAluno(ctx context.Context, alunoID int64, currentAnamneseID int64) error {
	query := `
		UPDATE anamneses 
		SET ativa = 0 
		WHERE aluno_id = ? AND id != ?
	`
	_, err := r.db.ExecContext(ctx, query, alunoID, currentAnamneseID)
	return err
}

func (r *AnamneseRepository) Delete(ctx context.Context, id int64) error {
	query := "DELETE FROM anamneses WHERE id = ?"
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *AnamneseRepository) CreateToken(ctx context.Context, t *domain.AnamneseToken) error {
	query := `
		INSERT INTO anamnese_tokens (
			token, pre_registro_id, expira_em, usado, aluno_id, aluno_nome, aluno_email,
			criado_em, criado_por, ip_origem, usado_em, ip_submissao, anamnese_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	expStr := t.ExpiraEm.Format("2006-01-02 15:04:05")
	criStr := t.CriadoEm.Format("2006-01-02 15:04:05")

	var usadoEmStr *string
	if t.UsadoEm != nil {
		s := t.UsadoEm.Format("2006-01-02 15:04:05")
		usadoEmStr = &s
	}

	usadoVal := 0
	if t.Usado {
		usadoVal = 1
	}

	result, err := r.db.ExecContext(ctx, query,
		t.Token, t.PreRegistroID, expStr, usadoVal, t.AlunoID, t.AlunoNome, t.AlunoEmail,
		criStr, t.CriadoPor, t.IpOrigem, usadoEmStr, t.IpSubmissao, t.AnamneseID,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = id
	return nil
}

func (r *AnamneseRepository) FindToken(ctx context.Context, tokenStr string) (*domain.AnamneseToken, error) {
	query := `
		SELECT 
			id, token, pre_registro_id, expira_em, usado, aluno_id, aluno_nome, aluno_email,
			criado_em, criado_por, ip_origem, usado_em, ip_submissao, anamnese_id
		FROM anamnese_tokens
		WHERE token = ?
	`
	row := r.db.QueryRowContext(ctx, query, tokenStr)

	var t domain.AnamneseToken
	var expStr, criStr string
	var usadoEmStr *string
	var usadoVal int

	err := row.Scan(
		&t.ID, &t.Token, &t.PreRegistroID, &expStr, &usadoVal, &t.AlunoID, &t.AlunoNome, &t.AlunoEmail,
		&criStr, &t.CriadoPor, &t.IpOrigem, &usadoEmStr, &t.IpSubmissao, &t.AnamneseID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	t.Usado = usadoVal == 1
	t.ExpiraEm, _ = parseDateTime(expStr)
	t.CriadoEm, _ = parseDateTime(criStr)
	if usadoEmStr != nil {
		tm, _ := parseDateTime(*usadoEmStr)
		t.UsadoEm = &tm
	}

	return &t, nil
}

func (r *AnamneseRepository) UpdateToken(ctx context.Context, t *domain.AnamneseToken) error {
	query := `
		UPDATE anamnese_tokens SET
			usado = ?,
			usado_em = ?,
			ip_submissao = ?,
			anamnese_id = ?
		WHERE id = ?
	`
	var usadoEmStr *string
	if t.UsadoEm != nil {
		s := t.UsadoEm.Format("2006-01-02 15:04:05")
		usadoEmStr = &s
	}

	usadoVal := 0
	if t.Usado {
		usadoVal = 1
	}

	_, err := r.db.ExecContext(ctx, query,
		usadoVal, usadoEmStr, t.IpSubmissao, t.AnamneseID, t.ID,
	)
	return err
}

func (r *AnamneseRepository) AddTokenAudit(ctx context.Context, a *domain.AnamneseTokenAudit) error {
	query := `
		INSERT INTO anamnese_tokens_audit (
			token, aluno_id, pre_registro_id, evento, ip, user_agent, detalhes, data_evento
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	dtStr := a.DataEvento.Format("2006-01-02 15:04:05")
	result, err := r.db.ExecContext(ctx, query,
		a.Token, a.AlunoID, a.PreRegistroID, a.Evento, a.Ip, a.UserAgent, a.Detalhes, dtStr,
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

func (r *AnamneseRepository) GetTokenAuditTrail(ctx context.Context, tokenStr string) ([]domain.AnamneseTokenAudit, error) {
	query := `
		SELECT 
			id, token, aluno_id, pre_registro_id, evento, ip, user_agent, detalhes, data_evento
		FROM anamnese_tokens_audit
		WHERE token = ?
		ORDER BY data_evento DESC
	`
	rows, err := r.db.QueryContext(ctx, query, tokenStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trail []domain.AnamneseTokenAudit
	for rows.Next() {
		var a domain.AnamneseTokenAudit
		var dtStr string
		err := rows.Scan(
			&a.ID, &a.Token, &a.AlunoID, &a.PreRegistroID, &a.Evento, &a.Ip, &a.UserAgent, &a.Detalhes, &dtStr,
		)
		if err != nil {
			return nil, err
		}
		a.DataEvento, _ = parseDateTime(dtStr)
		trail = append(trail, a)
	}
	return trail, nil
}
