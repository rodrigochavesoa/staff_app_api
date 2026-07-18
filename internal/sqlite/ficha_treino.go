package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"staff_app/internal/domain"
)

type FichaTreinoRepository struct {
	db *DB
}

func NewFichaTreinoRepository(db *DB) *FichaTreinoRepository {
	return &FichaTreinoRepository{db: db}
}

func (r *FichaTreinoRepository) Create(ctx context.Context, f *domain.FichaTreinoWeb) error {
	query := `
		INSERT INTO fichas_treino_web (
			aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal,
			duracao_treino, restricoes, feedback, turma, lista_exercicios, data_criacao,
			ficha_json, tipo_ficha, num_treinos, versao, ficha_anterior_id, data_arquivamento,
			ies_score, volume_sved, densidade, tut_total, series, rir, cadencia, rest_seconds
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	dataCriacaoStr := f.DataCriacao.Format("2006-01-02 15:04:05")
	var dataArquivamentoStr sql.NullString
	if f.DataArquivamento != nil {
		dataArquivamentoStr.Valid = true
		dataArquivamentoStr.String = f.DataArquivamento.Format("2006-01-02 15:04:05")
	}

	res, err := r.db.ExecContext(ctx, query,
		f.Aluno, f.Idade, f.Sexo, f.Objetivo, f.Modalidade, f.Nivel, f.FrequenciaSemanal,
		f.DuracaoTreino, f.Restricoes, f.Feedback, f.Turma, f.ListaExercicios, dataCriacaoStr,
		f.FichaJSON, f.TipoFicha, f.NumTreinos, f.Versao, f.FichaAnteriorID, dataArquivamentoStr,
		f.IesScore, f.VolumeSved, f.Densidade, f.TutTotal, f.Series, f.RIR, f.Cadencia, f.RestSeconds,
	)
	if err != nil {
		return fmt.Errorf("failed to insert fichas_treino_web: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	f.ID = id
	return nil
}

func (r *FichaTreinoRepository) GetByID(ctx context.Context, id int64) (*domain.FichaTreinoWeb, error) {
	query := `
		SELECT 
			id, aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal,
			duracao_treino, restricoes, feedback, turma, lista_exercicios, data_criacao,
			ficha_json, tipo_ficha, num_treinos, versao, ficha_anterior_id, data_arquivamento,
			COALESCE(ies_score, 0.0), COALESCE(volume_sved, 0), COALESCE(densidade, 0.0), 
			COALESCE(tut_total, 0), COALESCE(series, ''), COALESCE(rir, 2), 
			COALESCE(cadencia, '4010'), COALESCE(rest_seconds, 60)
		FROM fichas_treino_web
		WHERE id = ?
	`
	var f domain.FichaTreinoWeb
	var dataCriacaoStr string
	var dataArquivamentoStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&f.ID, &f.Aluno, &f.Idade, &f.Sexo, &f.Objetivo, &f.Modalidade, &f.Nivel, &f.FrequenciaSemanal,
		&f.DuracaoTreino, &f.Restricoes, &f.Feedback, &f.Turma, &f.ListaExercicios, &dataCriacaoStr,
		&f.FichaJSON, &f.TipoFicha, &f.NumTreinos, &f.Versao, &f.FichaAnteriorID, &dataArquivamentoStr,
		&f.IesScore, &f.VolumeSved, &f.Densidade, &f.TutTotal, &f.Series, &f.RIR, &f.Cadencia, &f.RestSeconds,
	)
	if err != nil {
		return nil, err
	}

	if t, err := time.Parse("2006-01-02 15:04:05", dataCriacaoStr); err == nil {
		f.DataCriacao = t
	} else if t, err := time.Parse(time.RFC3339, dataCriacaoStr); err == nil {
		f.DataCriacao = t
	}

	if dataArquivamentoStr.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", dataArquivamentoStr.String); err == nil {
			f.DataArquivamento = &t
		} else if t, err := time.Parse(time.RFC3339, dataArquivamentoStr.String); err == nil {
			f.DataArquivamento = &t
		}
	}

	return &f, nil
}

func (r *FichaTreinoRepository) Update(ctx context.Context, f *domain.FichaTreinoWeb) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		UPDATE fichas_treino_web
		SET 
			aluno = ?, idade = ?, sexo = ?, objetivo = ?, modalidade = ?, nivel = ?, 
			frequencia_semanal = ?, duracao_treino = ?, restricoes = ?, feedback = ?, 
			turma = ?, lista_exercicios = ?, ficha_json = ?, tipo_ficha = ?, 
			num_treinos = ?, versao = versao + 1, ficha_anterior_id = ?, data_arquivamento = ?,
			ies_score = ?, volume_sved = ?, densidade = ?, tut_total = ?, series = ?, rir = ?, cadencia = ?, rest_seconds = ?
		WHERE id = ? AND versao = ?
	`
	var dataArquivamentoStr sql.NullString
	if f.DataArquivamento != nil {
		dataArquivamentoStr.Valid = true
		dataArquivamentoStr.String = f.DataArquivamento.Format("2006-01-02 15:04:05")
	}

	res, err := tx.ExecContext(ctx, query,
		f.Aluno, f.Idade, f.Sexo, f.Objetivo, f.Modalidade, f.Nivel, f.FrequenciaSemanal,
		f.DuracaoTreino, f.Restricoes, f.Feedback, f.Turma, f.ListaExercicios, f.FichaJSON,
		f.TipoFicha, f.NumTreinos, f.FichaAnteriorID, dataArquivamentoStr,
		f.IesScore, f.VolumeSved, f.Densidade, f.TutTotal, f.Series, f.RIR, f.Cadencia, f.RestSeconds,
		f.ID, f.Versao,
	)
	if err != nil {
		return fmt.Errorf("failed to update fichas_treino_web: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	// Sychronize with active public link if exists
	_, err = tx.ExecContext(ctx, `
		UPDATE fichas_web
		SET conteudo_json = ?
		WHERE ficha_id = ? AND ativo = 1
	`, f.FichaJSON, f.ID)
	if err != nil {
		return fmt.Errorf("failed to sync public link: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	f.Versao++
	return nil
}

func (r *FichaTreinoRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Delete matching public links in fichas_web
	_, err = tx.ExecContext(ctx, "DELETE FROM fichas_web WHERE ficha_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete related public links: %w", err)
	}

	// 2. Delete the sheet itself
	res, err := tx.ExecContext(ctx, "DELETE FROM fichas_treino_web WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete fichas_treino_web: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
}

func (r *FichaTreinoRepository) ArchiveActiveByAlunoName(ctx context.Context, studentName string, dataArquivamento string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE fichas_treino_web
		SET data_arquivamento = ?, versao = versao + 1
		WHERE aluno = ? AND data_arquivamento IS NULL
	`, dataArquivamento, studentName)
	return err
}

func (r *FichaTreinoRepository) CreatePeriodizadaWithArchiveAndLink(ctx context.Context, f *domain.FichaTreinoWeb, hash string, validDays int, alunoID int64) (string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")

	// 0. Query the ID of the previous active sheet for this student
	var previousActiveID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM fichas_treino_web 
		WHERE aluno = ? AND data_arquivamento IS NULL 
		ORDER BY id DESC LIMIT 1
	`, f.Aluno).Scan(&previousActiveID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to query previous active sheet: %w", err)
	}

	var fichaAnteriorID *int64
	if previousActiveID.Valid {
		idVal := previousActiveID.Int64
		fichaAnteriorID = &idVal
		f.FichaAnteriorID = fichaAnteriorID
	}

	// 1. Archive previous active sheets for this student name
	_, err = tx.ExecContext(ctx, `
		UPDATE fichas_treino_web
		SET data_arquivamento = ?, versao = versao + 1
		WHERE aluno = ? AND data_arquivamento IS NULL
	`, nowStr, f.Aluno)
	if err != nil {
		return "", fmt.Errorf("failed to archive active sheets: %w", err)
	}

	// 2. Deactivate previous active links for this student ID
	_, err = tx.ExecContext(ctx, `
		UPDATE fichas_web
		SET ativo = 0
		WHERE aluno_id = ? AND ativo = 1
	`, alunoID)
	if err != nil {
		return "", fmt.Errorf("failed to deactivate active links: %w", err)
	}

	// 3. Insert new sheet
	queryInsertSheet := `
		INSERT INTO fichas_treino_web (
			aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal,
			duracao_treino, restricoes, feedback, turma, lista_exercicios, data_criacao,
			ficha_json, tipo_ficha, num_treinos, versao, ficha_anterior_id, data_arquivamento,
			ies_score, volume_sved, densidade, tut_total, series, rir, cadencia, rest_seconds
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'periodizada', ?, 1, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := tx.ExecContext(ctx, queryInsertSheet,
		f.Aluno, f.Idade, f.Sexo, f.Objetivo, f.Modalidade, f.Nivel, f.FrequenciaSemanal,
		f.DuracaoTreino, f.Restricoes, f.Feedback, f.Turma, f.ListaExercicios, nowStr,
		f.FichaJSON, f.NumTreinos, fichaAnteriorID,
		f.IesScore, f.VolumeSved, f.Densidade, f.TutTotal, f.Series, f.RIR, f.Cadencia, f.RestSeconds,
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert periodized sheet: %w", err)
	}

	fichaID, err := res.LastInsertId()
	if err != nil {
		return "", err
	}
	f.ID = fichaID

	// 4. Generate public link with 90 days validity
	expiration := now.Add(time.Duration(validDays) * 24 * time.Hour)
	expiraEmStr := expiration.Format(time.RFC3339)

	queryInsertLink := `
		INSERT INTO fichas_web (
			hash, ficha_id, aluno_id, conteudo_json, criado_em, expira_em, acessos, ativo
		) VALUES (?, ?, ?, ?, ?, ?, 0, 1)
	`
	_, err = tx.ExecContext(ctx, queryInsertLink,
		hash, fichaID, alunoID, f.FichaJSON, now.Format(time.RFC3339), expiraEmStr,
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert public link: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return expiraEmStr, nil
}
