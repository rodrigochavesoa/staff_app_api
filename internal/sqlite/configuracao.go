package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"staff_app/internal/domain"
)

type ConfiguracaoRepository struct {
	db *DB
}

func NewConfiguracaoRepository(db *DB) *ConfiguracaoRepository {
	return &ConfiguracaoRepository{db: db}
}

func (r *ConfiguracaoRepository) GetByChave(ctx context.Context, chave string) (*domain.Configuracao, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT chave, valor, tipo, sensivel, COALESCE(descricao, ''), atualizado_em, atualizado_por
		FROM configuracoes_sistema
		WHERE chave = ?
	`, chave)

	var c domain.Configuracao
	var sensivelInt int
	var atualizadoEmRaw string
	var atualizadoPor sql.NullInt64

	err := row.Scan(&c.Chave, &c.Valor, &c.Tipo, &sensivelInt, &c.Descricao, &atualizadoEmRaw, &atualizadoPor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to scan configuration: %w", err)
	}

	c.Sensivel = sensivelInt == 1
	c.AtualizadoEm = parseSQLiteTime(atualizadoEmRaw)
	if atualizadoPor.Valid {
		c.AtualizadoPor = &atualizadoPor.Int64
	}

	return &c, nil
}

func (r *ConfiguracaoRepository) List(ctx context.Context) ([]*domain.Configuracao, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT chave, valor, tipo, sensivel, COALESCE(descricao, ''), atualizado_em, atualizado_por
		FROM configuracoes_sistema
		ORDER BY chave ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list configurations: %w", err)
	}
	defer rows.Close()

	var configs []*domain.Configuracao
	for rows.Next() {
		var c domain.Configuracao
		var sensivelInt int
		var atualizadoEmRaw string
		var atualizadoPor sql.NullInt64

		if err := rows.Scan(&c.Chave, &c.Valor, &c.Tipo, &sensivelInt, &c.Descricao, &atualizadoEmRaw, &atualizadoPor); err != nil {
			return nil, fmt.Errorf("failed to scan configuration row: %w", err)
		}

		c.Sensivel = sensivelInt == 1
		c.AtualizadoEm = parseSQLiteTime(atualizadoEmRaw)
		if atualizadoPor.Valid {
			c.AtualizadoPor = &atualizadoPor.Int64
		}

		configs = append(configs, &c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during configuration rows iteration: %w", err)
	}

	return configs, nil
}

func (r *ConfiguracaoRepository) Update(ctx context.Context, config *domain.Configuracao) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE configuracoes_sistema
		SET valor = ?, atualizado_em = CURRENT_TIMESTAMP, atualizado_por = ?
		WHERE chave = ?
	`, config.Valor, config.AtualizadoPor, config.Chave)
	if err != nil {
		return fmt.Errorf("failed to update system configuration: %w", err)
	}
	return requireAffected(res, sql.ErrNoRows)
}

func (r *ConfiguracaoRepository) UpdateMultiple(ctx context.Context, configs []*domain.Configuracao) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	for _, c := range configs {
		res, err := tx.ExecContext(ctx, `
			UPDATE configuracoes_sistema
			SET valor = ?, atualizado_em = CURRENT_TIMESTAMP, atualizado_por = ?
			WHERE chave = ?
		`, c.Valor, c.AtualizadoPor, c.Chave)
		if err != nil {
			return fmt.Errorf("failed to update key %s: %w", c.Chave, err)
		}
		if err := requireAffected(res, sql.ErrNoRows); err != nil {
			return fmt.Errorf("key %s not found: %w", c.Chave, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

type DashboardRepository struct {
	db *DB
}

func NewDashboardRepository(db *DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

func (r *DashboardRepository) GetStats(ctx context.Context) (*domain.DashboardStats, error) {
	var stats domain.DashboardStats

	err := r.db.QueryRowContext(ctx, `
		SELECT 
			(SELECT COUNT(*) FROM alunos),
			(SELECT COUNT(*) FROM alunos WHERE ativo = 1),
			(SELECT COUNT(*) FROM alunos WHERE ativo = 0),
			(SELECT COUNT(*) FROM alunos a LEFT JOIN anamneses an ON a.id = an.aluno_id WHERE an.id IS NULL)
	`).Scan(&stats.Alunos.Total, &stats.Alunos.Ativos, &stats.Alunos.Inativos, &stats.Alunos.SemAnamnese)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve student statistics: %w", err)
	}

	err = r.db.QueryRowContext(ctx, `
		SELECT 
			(SELECT COUNT(*) FROM anamneses),
			(SELECT COUNT(*) FROM anamneses WHERE status_aprovacao = 'pendente'),
			(SELECT COUNT(*) FROM anamneses WHERE COALESCE(risk_score_cached, 0) >= 2)
	`).Scan(&stats.Anamneses.Total, &stats.Anamneses.PendentesAprovacao, &stats.Anamneses.AltoRisco)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve anamnese statistics: %w", err)
	}

	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pre_registros WHERE status = 'aguardando_aprovacao'
	`).Scan(&stats.PreRegistrosPendentes)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve pending pre-registration count: %w", err)
	}

	time24hAgo := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	err = r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM atividades_garmin WHERE start_time >= ?
	`, time24hAgo).Scan(&stats.AtividadesGarmin24h)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve garmin activities count: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT a.plano_id, COALESCE(p.nome, 'Sem Plano'), COUNT(a.id) AS quantidade_alunos
		FROM alunos a
		LEFT JOIN planos p ON a.plano_id = p.id
		WHERE a.ativo = 1 AND a.plano_ativo = 1 AND a.plano_id IS NOT NULL
		GROUP BY a.plano_id, p.nome
		ORDER BY quantidade_alunos DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query plan distribution: %w", err)
	}
	defer rows.Close()

	stats.DistribuicaoPlanos = make([]domain.PlanoDistribuicao, 0)
	for rows.Next() {
		var pd domain.PlanoDistribuicao
		if err := rows.Scan(&pd.PlanoID, &pd.Nome, &pd.QuantidadeAlunos); err != nil {
			return nil, fmt.Errorf("failed to scan plan distribution row: %w", err)
		}
		stats.DistribuicaoPlanos = append(stats.DistribuicaoPlanos, pd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error in plan distribution iteration: %w", err)
	}

	return &stats, nil
}
