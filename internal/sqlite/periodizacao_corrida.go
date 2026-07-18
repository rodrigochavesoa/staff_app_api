package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"staff_app/internal/domain"
)

type PeriodizacaoCorridaRepository struct {
	db *DB
}

func NewPeriodizacaoCorridaRepository(db *DB) *PeriodizacaoCorridaRepository {
	return &PeriodizacaoCorridaRepository{db: db}
}

func (r *PeriodizacaoCorridaRepository) Create(ctx context.Context, pc *domain.PeriodizacaoCorrida) error {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO periodizacao_corrida (
			aluno_id, data_inicio, duracao_semanas, modo, semana_atual, status,
			distancia_prova, nivel, vdot, pace_base, volume_semanal,
			dias_disponiveis, plano_json, modo_geracao, data_ultima_geracao,
			dias_semana_selecionados, versao, ficha_anterior_id, data_arquivamento
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, pc.AlunoID, pc.DataInicio, pc.DuracaoSemanas, pc.Modo, pc.SemanaAtual, pc.Status,
		pc.DistanciaProva, pc.Nivel, pc.VDOT, pc.PaceBase, pc.VolumeSemanal,
		pc.DiasDisponiveis, pc.PlanoJSON, pc.ModoGeracao, pc.DataUltimaGeracao,
		pc.DiasSemanaSelecionados, pc.Versao, pc.FichaAnteriorID, pc.DataArquivamento)
	if err != nil {
		return fmt.Errorf("failed to insert periodizacao_corrida: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to retrieve last insert id: %w", err)
	}
	pc.ID = id
	return nil
}

func (r *PeriodizacaoCorridaRepository) CreateWithArchiveActive(ctx context.Context, pc *domain.PeriodizacaoCorrida, dataArquivamento string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin periodizacao transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var archivedID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM periodizacao_corrida 
		WHERE aluno_id = ? AND status = 'ativo' 
		ORDER BY data_inicio DESC LIMIT 1
	`, pc.AlunoID).Scan(&archivedID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check for active periodization: %w", err)
	}

	if archivedID > 0 {
		res, err := tx.ExecContext(ctx, `
			UPDATE periodizacao_corrida
			SET status = 'arquivado', data_arquivamento = ?, versao = versao + 1
			WHERE id = ? AND status = 'ativo'
		`, dataArquivamento, archivedID)
		if err != nil {
			return fmt.Errorf("failed to archive active periodization: %w", err)
		}

		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected during archiving: %w", err)
		}
		if rowsAffected == 0 {
			return sql.ErrNoRows
		}

		pc.FichaAnteriorID = &archivedID
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO periodizacao_corrida (
			aluno_id, data_inicio, duracao_semanas, modo, semana_atual, status,
			distancia_prova, nivel, vdot, pace_base, volume_semanal,
			dias_disponiveis, plano_json, modo_geracao, data_ultima_geracao,
			dias_semana_selecionados, versao, ficha_anterior_id, data_arquivamento
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, pc.AlunoID, pc.DataInicio, pc.DuracaoSemanas, pc.Modo, pc.SemanaAtual, pc.Status,
		pc.DistanciaProva, pc.Nivel, pc.VDOT, pc.PaceBase, pc.VolumeSemanal,
		pc.DiasDisponiveis, pc.PlanoJSON, pc.ModoGeracao, pc.DataUltimaGeracao,
		pc.DiasSemanaSelecionados, pc.Versao, pc.FichaAnteriorID, pc.DataArquivamento)
	if err != nil {
		return fmt.Errorf("failed to insert periodizacao_corrida: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to retrieve last insert id: %w", err)
	}
	pc.ID = id

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit periodizacao transaction: %w", err)
	}

	return nil
}

func (r *PeriodizacaoCorridaRepository) GetByID(ctx context.Context, id int64) (*domain.PeriodizacaoCorrida, error) {
	query := `
		SELECT 
			p.id, p.aluno_id, p.data_inicio, p.duracao_semanas, p.modo, p.semana_atual, p.status,
			p.distancia_prova, p.nivel, p.vdot, p.pace_base, p.volume_semanal, p.dias_disponiveis,
			p.plano_json, p.modo_geracao, p.data_ultima_geracao, p.dias_semana_selecionados,
			p.versao, p.ficha_anterior_id, p.data_arquivamento, COALESCE(a.nome, ''), COALESCE(a.idade, 0)
		FROM periodizacao_corrida p
		LEFT JOIN alunos a ON p.aluno_id = a.id
		WHERE p.id = ?`

	row := r.db.QueryRowContext(ctx, query, id)
	return scanPeriodizacao(row)
}

func (r *PeriodizacaoCorridaRepository) ListByAlunoID(ctx context.Context, alunoID int64) ([]*domain.PeriodizacaoCorrida, error) {
	query := `
		SELECT 
			p.id, p.aluno_id, p.data_inicio, p.duracao_semanas, p.modo, p.semana_atual, p.status,
			p.distancia_prova, p.nivel, p.vdot, p.pace_base, p.volume_semanal, p.dias_disponiveis,
			p.plano_json, p.modo_geracao, p.data_ultima_geracao, p.dias_semana_selecionados,
			p.versao, p.ficha_anterior_id, p.data_arquivamento, COALESCE(a.nome, ''), COALESCE(a.idade, 0)
		FROM periodizacao_corrida p
		LEFT JOIN alunos a ON p.aluno_id = a.id
		WHERE p.aluno_id = ?
		ORDER BY p.data_inicio DESC, p.id DESC`

	rows, err := r.db.QueryContext(ctx, query, alunoID)
	if err != nil {
		return nil, fmt.Errorf("failed to list periodizacao_corrida: %w", err)
	}
	defer rows.Close()

	var list []*domain.PeriodizacaoCorrida
	for rows.Next() {
		pc, err := scanPeriodizacao(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, pc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error in periodizacao rows: %w", err)
	}
	return list, nil
}

func (r *PeriodizacaoCorridaRepository) Update(ctx context.Context, pc *domain.PeriodizacaoCorrida) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE periodizacao_corrida
		SET 
			aluno_id = ?, data_inicio = ?, duracao_semanas = ?, modo = ?, semana_atual = ?, status = ?,
			distancia_prova = ?, nivel = ?, vdot = ?, pace_base = ?, volume_semanal = ?,
			dias_disponiveis = ?, plano_json = ?, modo_geracao = ?, data_ultima_geracao = ?,
			dias_semana_selecionados = ?, versao = versao + 1, ficha_anterior_id = ?, data_arquivamento = ?
		WHERE id = ? AND versao = ?
	`, pc.AlunoID, pc.DataInicio, pc.DuracaoSemanas, pc.Modo, pc.SemanaAtual, pc.Status,
		pc.DistanciaProva, pc.Nivel, pc.VDOT, pc.PaceBase, pc.VolumeSemanal,
		pc.DiasDisponiveis, pc.PlanoJSON, pc.ModoGeracao, pc.DataUltimaGeracao,
		pc.DiasSemanaSelecionados, pc.FichaAnteriorID, pc.DataArquivamento, pc.ID, pc.Versao)
	if err != nil {
		return fmt.Errorf("failed to update periodizacao_corrida: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	pc.Versao++
	return nil
}

func (r *PeriodizacaoCorridaRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM periodizacao_corrida WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete periodizacao_corrida: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PeriodizacaoCorridaRepository) ArchiveActiveByAlunoID(ctx context.Context, alunoID int64, dataArquivamento string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM periodizacao_corrida 
		WHERE aluno_id = ? AND status = 'ativo' 
		ORDER BY data_inicio DESC LIMIT 1
	`, alunoID).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to check for active periodization: %w", err)
	}

	res, err := r.db.ExecContext(ctx, `
		UPDATE periodizacao_corrida
		SET status = 'arquivado', data_arquivamento = ?, versao = versao + 1
		WHERE id = ? AND status = 'ativo'
	`, dataArquivamento, id)
	if err != nil {
		return 0, fmt.Errorf("failed to archive active periodization: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected during archiving: %w", err)
	}
	if rowsAffected == 0 {
		return 0, nil
	}

	return id, nil
}

func (r *PeriodizacaoCorridaRepository) CreatePublicLink(ctx context.Context, link *domain.PeriodizacaoCorridaWeb) error {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO periodizacao_corrida_web (
			hash, periodizacao_id, aluno_id, user_id, criado_em, expira_em, acessos, ultimo_acesso, ativo
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, link.Hash, link.PeriodizacaoID, link.AlunoID, link.UserID, link.CriadoEm, link.ExpiraEm, link.Acessos, link.UltimoAcesso, link.Ativo)
	if err != nil {
		return fmt.Errorf("failed to create public link: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to retrieve public link id: %w", err)
	}
	link.ID = id
	return nil
}

func (r *PeriodizacaoCorridaRepository) GetPublicLinkByHash(ctx context.Context, hash string) (*domain.PeriodizacaoCorridaWeb, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, hash, periodizacao_id, aluno_id, user_id, criado_em, expira_em, acessos, ultimo_acesso, ativo
		FROM periodizacao_corrida_web
		WHERE hash = ?`, hash)
	return scanPublicLink(row)
}

func (r *PeriodizacaoCorridaRepository) GetPublicLinkByPeriodizacaoID(ctx context.Context, periodizacaoID int64) (*domain.PeriodizacaoCorridaWeb, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, hash, periodizacao_id, aluno_id, user_id, criado_em, expira_em, acessos, ultimo_acesso, ativo
		FROM periodizacao_corrida_web
		WHERE periodizacao_id = ? AND ativo = 1
		ORDER BY criado_em DESC LIMIT 1`, periodizacaoID)
	return scanPublicLink(row)
}

func (r *PeriodizacaoCorridaRepository) UpdatePublicLink(ctx context.Context, link *domain.PeriodizacaoCorridaWeb) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE periodizacao_corrida_web
		SET hash = ?, periodizacao_id = ?, aluno_id = ?, user_id = ?, criado_em = ?, expira_em = ?, acessos = ?, ultimo_acesso = ?, ativo = ?
		WHERE id = ?
	`, link.Hash, link.PeriodizacaoID, link.AlunoID, link.UserID, link.CriadoEm, link.ExpiraEm, link.Acessos, link.UltimoAcesso, link.Ativo, link.ID)
	if err != nil {
		return fmt.Errorf("failed to update public link: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PeriodizacaoCorridaRepository) IncrementPublicLinkAccess(ctx context.Context, hash string) error {
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `
		UPDATE periodizacao_corrida_web
		SET acessos = acessos + 1, ultimo_acesso = ?
		WHERE hash = ? AND ativo = 1
	`, now, hash)
	if err != nil {
		return fmt.Errorf("failed to increment public link access: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanPeriodizacao(row rowScanner) (*domain.PeriodizacaoCorrida, error) {
	var pc domain.PeriodizacaoCorrida
	var alunoNome sql.NullString
	var alunoIdade sql.NullInt32
	var fichaAnteriorID sql.NullInt64
	var dataArquivamento sql.NullString

	err := row.Scan(
		&pc.ID, &pc.AlunoID, &pc.DataInicio, &pc.DuracaoSemanas, &pc.Modo, &pc.SemanaAtual, &pc.Status,
		&pc.DistanciaProva, &pc.Nivel, &pc.VDOT, &pc.PaceBase, &pc.VolumeSemanal, &pc.DiasDisponiveis,
		&pc.PlanoJSON, &pc.ModoGeracao, &pc.DataUltimaGeracao, &pc.DiasSemanaSelecionados,
		&pc.Versao, &fichaAnteriorID, &dataArquivamento, &alunoNome, &alunoIdade,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to scan periodizacao_corrida: %w", err)
	}

	if fichaAnteriorID.Valid {
		val := fichaAnteriorID.Int64
		pc.FichaAnteriorID = &val
	}
	if dataArquivamento.Valid {
		val := dataArquivamento.String
		pc.DataArquivamento = &val
	}
	if alunoNome.Valid {
		pc.AlunoNome = alunoNome.String
	}
	if alunoIdade.Valid {
		pc.AlunoIdade = int(alunoIdade.Int32)
	}

	return &pc, nil
}

func scanPublicLink(row rowScanner) (*domain.PeriodizacaoCorridaWeb, error) {
	var link domain.PeriodizacaoCorridaWeb
	var userID sql.NullInt64
	var criadoEmStr, expiraEmStr string
	var ultimoAcessoStr sql.NullString

	err := row.Scan(
		&link.ID, &link.Hash, &link.PeriodizacaoID, &link.AlunoID, &userID,
		&criadoEmStr, &expiraEmStr, &link.Acessos, &ultimoAcessoStr, &link.Ativo,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to scan public link: %w", err)
	}

	if userID.Valid {
		val := userID.Int64
		link.UserID = &val
	}

	link.CriadoEm, err = parseDateTime(criadoEmStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse criado_em: %w", err)
	}

	link.ExpiraEm, err = parseDateTime(expiraEmStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expira_em: %w", err)
	}

	if ultimoAcessoStr.Valid && ultimoAcessoStr.String != "" {
		t, err := parseDateTime(ultimoAcessoStr.String)
		if err == nil {
			link.UltimoAcesso = &t
		}
	}

	return &link, nil
}
