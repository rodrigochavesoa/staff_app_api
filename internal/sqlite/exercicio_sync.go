package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/exercicios/csvsync"
)

type txCtxKey struct{}

func (r *ExercicioRepository) connQuery(ctx context.Context) interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
} {
	if tx, ok := ctx.Value(txCtxKey{}).(*sql.Tx); ok && tx != nil {
		return tx
	}
	return r.db
}

func (r *ExercicioRepository) connExec(ctx context.Context) interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
} {
	if tx, ok := ctx.Value(txCtxKey{}).(*sql.Tx); ok && tx != nil {
		return tx
	}
	return r.db
}

// WithTx executa fn numa única transação SQLite ligada ao contexto (sync do catálogo).
func (r *ExercicioRepository) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(context.WithValue(ctx, txCtxKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit()
}

// UpsertCatalogExercise insere ou atualiza linha gerida pelo CSV (spec §5.2).
// Preferir chamar dentro de WithTx. Retorna inserted=true no INSERT.
func (r *ExercicioRepository) UpsertCatalogExercise(ctx context.Context, ex *domain.ExercicioReabilitacao, existing *domain.ExercicioReabilitacao) (bool, error) {
	if ex == nil {
		return false, errors.New("nil exercise")
	}
	if existing == nil {
		var err error
		existing, err = r.getByCodigoConn(ctx, ex.Codigo)
		if err != nil {
			return false, err
		}
	}
	if existing == nil {
		err := r.insertCatalog(ctx, ex)
		if isUniqueConstraint(err) {
			return false, csvsync.ErrNameConflict
		}
		return err == nil, err
	}
	if existing.CriadoPor != csvsync.CatalogMarker {
		return false, fmt.Errorf("codigo %d not csv-managed", ex.Codigo)
	}
	err := r.updateCatalog(ctx, ex)
	if isUniqueConstraint(err) {
		return false, csvsync.ErrNameConflict
	}
	return false, err
}

func (r *ExercicioRepository) getByCodigoConn(ctx context.Context, codigo int) (*domain.ExercicioReabilitacao, error) {
	const q = `
		SELECT codigo, nome, categoria,
			COALESCE(descricao_terapeutica, ''), COALESCE(descricao, ''), COALESCE(indicacoes, ''),
			COALESCE(contraindicacoes, ''), COALESCE(restricoes_sugeridas, ''), COALESCE(grupo_muscular, ''),
			COALESCE(musculo_foco, ''), COALESCE(tipo_exercicio, ''), COALESCE(intensidade, ''),
			nivel_prioridade, COALESCE(fonte_cientifica, ''), COALESCE(url, ''),
			COALESCE(url_secundaria, ''), COALESCE(video_url, ''), COALESCE(criado_por, ''),
			criado_em, status, COALESCE(notas_profissional, ''), atualizado_em, atualizado_por
		FROM exercicios_reabilitacao WHERE codigo = ?`
	var ex domain.ExercicioReabilitacao
	var criStr string
	var updStr, updPor sql.NullString
	err := r.connQuery(ctx).QueryRowContext(ctx, q, codigo).Scan(
		&ex.Codigo, &ex.Nome, &ex.Categoria, &ex.DescricaoTerapeutica, &ex.Descricao, &ex.Indicacoes,
		&ex.Contraindicacoes, &ex.RestricoesSugeridas, &ex.GrupoMuscular, &ex.MusculoFoco,
		&ex.TipoExercicio, &ex.Intensidade, &ex.NivelPrioridade, &ex.FonteCientifica,
		&ex.Url, &ex.UrlSecundaria, &ex.VideoUrl, &ex.CriadoPor, &criStr, &ex.Status,
		&ex.NotasProfissional, &updStr, &updPor,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ex.CriadoEm, _ = parseDateTime(criStr)
	if updStr.Valid && updStr.String != "" {
		t, _ := parseDateTime(updStr.String)
		ex.AtualizadoEm = &t
	}
	if updPor.Valid {
		ex.AtualizadoPor = updPor.String
	}
	return &ex, nil
}

func (r *ExercicioRepository) insertCatalog(ctx context.Context, ex *domain.ExercicioReabilitacao) error {
	const q = `
		INSERT INTO exercicios_reabilitacao (
			codigo, nome, categoria, descricao_terapeutica, descricao, indicacoes,
			contraindicacoes, restricoes_sugeridas, grupo_muscular, musculo_foco,
			tipo_exercicio, intensidade, nivel_prioridade, fonte_cientifica,
			url, url_secundaria, video_url, criado_por, criado_em, status, notas_profissional
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.connExec(ctx).ExecContext(ctx, q,
		ex.Codigo, ex.Nome, ex.Categoria, ex.DescricaoTerapeutica, ex.Descricao, ex.Indicacoes,
		ex.Contraindicacoes, ex.RestricoesSugeridas, ex.GrupoMuscular, ex.MusculoFoco,
		ex.TipoExercicio, ex.Intensidade, ex.NivelPrioridade, ex.FonteCientifica,
		ex.Url, ex.UrlSecundaria, ex.VideoUrl, ex.CriadoPor,
		ex.CriadoEm.UTC().Format("2006-01-02 15:04:05"), ex.Status, ex.NotasProfissional,
	)
	return err
}

func (r *ExercicioRepository) updateCatalog(ctx context.Context, ex *domain.ExercicioReabilitacao) error {
	const q = `
		UPDATE exercicios_reabilitacao SET
			nome = ?, grupo_muscular = ?, musculo_foco = ?, url = ?, status = 'ativo', atualizado_em = ?
		WHERE codigo = ? AND criado_por = ?`
	res, err := r.connExec(ctx).ExecContext(ctx, q,
		ex.Nome, ex.GrupoMuscular, ex.MusculoFoco, ex.Url,
		time.Now().UTC().Format("2006-01-02 15:04:05"),
		ex.Codigo, csvsync.CatalogMarker,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("catalog update matched 0 rows for codigo %d", ex.Codigo)
	}
	return nil
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
