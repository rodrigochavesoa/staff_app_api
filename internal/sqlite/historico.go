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

type HistoricoRepository struct {
	db *DB
}

func NewHistoricoRepository(db *DB) *HistoricoRepository {
	return &HistoricoRepository{db: db}
}

func (r *HistoricoRepository) GetDB() *DB {
	return r.db
}

// SearchAlunos searches for active or inactive students by name, email, or group (turma) with a limit.
func (r *HistoricoRepository) SearchAlunos(ctx context.Context, q string, limit int, ativo string) ([]*domain.AlunoSearchResponse, error) {
	var queryBuilder strings.Builder
	queryBuilder.WriteString(`
		SELECT a.id, a.nome, a.email, a.turma, a.ativo, a.plano_ativo, COALESCE(p.nome, '')
		FROM alunos a
		LEFT JOIN planos p ON a.plano_id = p.id
		WHERE 1=1
	`)

	var args []any

	if q != "" {
		pattern := "%" + q + "%"
		queryBuilder.WriteString(" AND (a.nome LIKE ? OR a.email LIKE ? OR a.turma LIKE ?)")
		args = append(args, pattern, pattern, pattern)
	}

	if ativo == "true" {
		queryBuilder.WriteString(" AND a.ativo = 1")
	} else if ativo == "false" {
		queryBuilder.WriteString(" AND a.ativo = 0")
	}

	queryBuilder.WriteString(" ORDER BY a.nome ASC")

	if limit > 0 {
		queryBuilder.WriteString(" LIMIT ?")
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, queryBuilder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search alunos: %w", err)
	}
	defer rows.Close()

	var list []*domain.AlunoSearchResponse
	for rows.Next() {
		var a domain.AlunoSearchResponse
		var ativoVal, planoAtivoVal int
		if err := rows.Scan(&a.ID, &a.Nome, &a.Email, &a.Turma, &ativoVal, &planoAtivoVal, &a.PlanoNome); err != nil {
			return nil, fmt.Errorf("failed to scan search student row: %w", err)
		}
		a.Ativo = (ativoVal == 1)
		a.PlanoAtivo = (planoAtivoVal == 1)
		list = append(list, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading search students rows: %w", err)
	}

	return list, nil
}

// GetTreinosRealizadosByAluno retrieves weight lifting sessions for a student, optionally filtered by month and year.
func (r *HistoricoRepository) GetTreinosRealizadosByAluno(ctx context.Context, alunoID int64, mes, ano int) ([]*domain.TreinoRealizado, error) {
	var query string
	var args []any

	if mes > 0 && ano > 0 {
		startDate := fmt.Sprintf("%04d-%02d-01", ano, mes)
		endDate := fmt.Sprintf("%04d-%02d-31", ano, mes) // string comparison works for months
		query = `
			SELECT id, ficha_id, aluno_id, hash_ficha, data_treino, tipo_treino, tipo_ficha, observacao, criado_em
			FROM treinos_realizados
			WHERE aluno_id = ? AND data_treino >= ? AND data_treino <= ?
			ORDER BY data_treino DESC, id DESC
		`
		args = []any{alunoID, startDate, endDate}
	} else {
		query = `
			SELECT id, ficha_id, aluno_id, hash_ficha, data_treino, tipo_treino, tipo_ficha, observacao, criado_em
			FROM treinos_realizados
			WHERE aluno_id = ?
			ORDER BY data_treino DESC, id DESC
		`
		args = []any{alunoID}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query treinos_realizados: %w", err)
	}
	defer rows.Close()

	var list []*domain.TreinoRealizado
	for rows.Next() {
		var tr domain.TreinoRealizado
		var alunoIDVal sql.NullInt64
		var hashFichaVal sql.NullString
		var tipoTreinoVal sql.NullString
		var observacaoVal sql.NullString
		var criadoEmStr string

		err := rows.Scan(
			&tr.ID, &tr.FichaID, &alunoIDVal, &hashFichaVal, &tr.DataTreino,
			&tipoTreinoVal, &tr.TipoFicha, &observacaoVal, &criadoEmStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan treino_realizado row: %w", err)
		}

		if idx := strings.Index(tr.DataTreino, "T"); idx != -1 {
			tr.DataTreino = tr.DataTreino[:idx]
		}

		if alunoIDVal.Valid {
			val := alunoIDVal.Int64
			tr.AlunoID = &val
		}
		if hashFichaVal.Valid {
			val := hashFichaVal.String
			tr.HashFicha = &val
		}
		if tipoTreinoVal.Valid {
			val := tipoTreinoVal.String
			tr.TipoTreino = &val
		}
		if observacaoVal.Valid {
			val := observacaoVal.String
			tr.Observacao = &val
		}

		if t, err := time.Parse(time.RFC3339, criadoEmStr); err == nil {
			tr.CriadoEm = t
		} else if t, err := time.Parse("2006-01-02 15:04:05", criadoEmStr); err == nil {
			tr.CriadoEm = t
		}
		list = append(list, &tr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading treinos_realizados rows: %w", err)
	}

	return list, nil
}

// MarkTreinoRealizado writes a record to the treinos_realizados table with SQLite UPSERT.
func (r *HistoricoRepository) MarkTreinoRealizado(ctx context.Context, tr *domain.TreinoRealizado) error {
	query := `
		INSERT INTO treinos_realizados (
			ficha_id, aluno_id, hash_ficha, data_treino, tipo_treino, tipo_ficha, observacao, criado_em
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ficha_id, data_treino) DO UPDATE SET
			aluno_id = COALESCE(excluded.aluno_id, treinos_realizados.aluno_id),
			hash_ficha = COALESCE(excluded.hash_ficha, treinos_realizados.hash_ficha),
			tipo_treino = COALESCE(excluded.tipo_treino, treinos_realizados.tipo_treino),
			tipo_ficha = COALESCE(excluded.tipo_ficha, treinos_realizados.tipo_ficha),
			observacao = COALESCE(excluded.observacao, treinos_realizados.observacao)
	`
	nowStr := time.Now().Format(time.RFC3339)

	var alunoID sql.NullInt64
	if tr.AlunoID != nil {
		alunoID.Valid = true
		alunoID.Int64 = *tr.AlunoID
	}

	var hashFicha sql.NullString
	if tr.HashFicha != nil {
		hashFicha.Valid = true
		hashFicha.String = *tr.HashFicha
	}

	var tipoTreino sql.NullString
	if tr.TipoTreino != nil {
		tipoTreino.Valid = true
		tipoTreino.String = *tr.TipoTreino
	}

	var observacao sql.NullString
	if tr.Observacao != nil {
		observacao.Valid = true
		observacao.String = *tr.Observacao
	}
	res, err := r.db.ExecContext(ctx, query,
		tr.FichaID, alunoID, hashFicha, tr.DataTreino, tipoTreino, tr.TipoFicha, observacao, nowStr,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert treino_realizado: %w", err)
	}

	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		tr.ID = id
	}

	return nil
}

// UnmarkTreinoRealizado deletes a finished workout record.
func (r *HistoricoRepository) UnmarkTreinoRealizado(ctx context.Context, fichaID int64, dataTreino string) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM treinos_realizados WHERE ficha_id = ? AND data_treino = ?", fichaID, dataTreino)
	if err != nil {
		return fmt.Errorf("failed to delete treino_realizado: %w", err)
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

// GetHistoricoFichaByID loads archived training sheet/periodization snapshotted info.
func (r *HistoricoRepository) GetHistoricoFichaByID(ctx context.Context, id int64) (*domain.HistoricoFicha, error) {
	query := `
		SELECT 
			id, aluno_id, tipo_ficha, versao, status, data_criacao, data_arquivamento,
			data_inicio_uso, ficha_origem_id, ficha_origem_tabela, ficha_json, plano_json,
			vdot, pace_base, distancia_prova, nivel, duracao_semanas, modo,
			semanas_completadas, taxa_completude, feedback_dificuldade_medio, dores_reportadas,
			total_treinos_planejados, total_treinos_realizados, dias_uso, objetivo,
			modalidade, frequencia_semanal, observacoes_gerais, coach_notes
		FROM historico_fichas
		WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)

	var h domain.HistoricoFicha
	var dataCriacaoStr sql.NullString
	var dataArquivamentoStr string
	var dataInicioUsoStr sql.NullString
	var fichaOrigemID sql.NullInt64
	var fichaOrigemTabela sql.NullString
	var fichaJSON sql.NullString
	var planoJSON sql.NullString
	var vdotVal sql.NullFloat64
	var paceBase sql.NullString
	var distanciaProva sql.NullFloat64
	var nivel sql.NullString
	var duracaoSemanas sql.NullInt32
	var modo sql.NullString
	var doresReportadas sql.NullString
	var objetivo sql.NullString
	var modalidade sql.NullString
	var frequenciaSemanal sql.NullInt32
	var observacoesGerais sql.NullString
	var coachNotes sql.NullString

	err := row.Scan(
		&h.ID, &h.AlunoID, &h.TipoFicha, &h.Versao, &h.Status, &dataCriacaoStr, &dataArquivamentoStr,
		&dataInicioUsoStr, &fichaOrigemID, &fichaOrigemTabela, &fichaJSON, &planoJSON,
		&vdotVal, &paceBase, &distanciaProva, &nivel, &duracaoSemanas, &modo,
		&h.SemanasCompletadas, &h.TaxaCompletude, &h.FeedbackDificuldadeMedio, &doresReportadas,
		&h.TotalTreinosPlanejados, &h.TotalTreinosRealizados, &h.DiasUso, &objetivo,
		&modalidade, &frequenciaSemanal, &observacoesGerais, &coachNotes,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to scan historico_ficha: %w", err)
	}

	if dataCriacaoStr.Valid {
		val := dataCriacaoStr.String
		h.DataCriacao = &val
	}

	if t, err := time.Parse(time.RFC3339, dataArquivamentoStr); err == nil {
		h.DataArquivamento = t
	} else if t, err := time.Parse("2006-01-02 15:04:05", dataArquivamentoStr); err == nil {
		h.DataArquivamento = t
	}

	if dataInicioUsoStr.Valid {
		val := dataInicioUsoStr.String
		h.DataInicioUso = &val
	}
	if fichaOrigemID.Valid {
		val := fichaOrigemID.Int64
		h.FichaOrigemID = &val
	}
	if fichaOrigemTabela.Valid {
		val := fichaOrigemTabela.String
		h.FichaOrigemTabela = &val
	}
	if fichaJSON.Valid {
		val := fichaJSON.String
		h.FichaJSON = &val
	}
	if planoJSON.Valid {
		val := planoJSON.String
		h.PlanoJSON = &val
	}
	if vdotVal.Valid {
		val := vdotVal.Float64
		h.VDOT = &val
	}
	if paceBase.Valid {
		val := paceBase.String
		h.PaceBase = &val
	}
	if distanciaProva.Valid {
		val := distanciaProva.Float64
		h.DistanciaProva = &val
	}
	if nivel.Valid {
		val := nivel.String
		h.Nivel = &val
	}
	if duracaoSemanas.Valid {
		val := int(duracaoSemanas.Int32)
		h.DuracaoSemanas = &val
	}
	if modo.Valid {
		val := modo.String
		h.Modo = &val
	}
	if doresReportadas.Valid {
		val := doresReportadas.String
		h.DoresReportadas = &val
	}
	if objetivo.Valid {
		val := objetivo.String
		h.Objetivo = &val
	}
	if modalidade.Valid {
		val := modalidade.String
		h.Modalidade = &val
	}
	if frequenciaSemanal.Valid {
		val := int(frequenciaSemanal.Int32)
		h.FrequenciaSemanal = &val
	}
	if observacoesGerais.Valid {
		val := observacoesGerais.String
		h.ObservacoesGerais = &val
	}
	if coachNotes.Valid {
		val := coachNotes.String
		h.CoachNotes = &val
	}

	return &h, nil
}
