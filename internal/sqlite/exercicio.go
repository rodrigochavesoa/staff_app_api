package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"staff_app/internal/domain"
)

type ExercicioRepository struct {
	db *DB
}

func NewExercicioRepository(db *DB) *ExercicioRepository {
	return &ExercicioRepository{db: db}
}

// Com WithTx no contexto, a leitura usa a mesma transação do sync do catálogo.
func (r *ExercicioRepository) GetByCodigo(ctx context.Context, codigo int) (*domain.ExercicioReabilitacao, error) {
	return r.getByCodigoConn(ctx, codigo)
}

func (r *ExercicioRepository) GetByNome(ctx context.Context, nome string) (*domain.ExercicioReabilitacao, error) {
	query := `
		SELECT 
			codigo, nome, categoria, 
			COALESCE(descricao_terapeutica, ''), COALESCE(descricao, ''), COALESCE(indicacoes, ''),
			COALESCE(contraindicacoes, ''), COALESCE(restricoes_sugeridas, ''), COALESCE(grupo_muscular, ''), 
			COALESCE(musculo_foco, ''), COALESCE(tipo_exercicio, ''), COALESCE(intensidade, ''), 
			nivel_prioridade, COALESCE(fonte_cientifica, ''), COALESCE(url, ''), 
			COALESCE(url_secundaria, ''), COALESCE(video_url, ''), COALESCE(criado_por, ''), 
			criado_em, status, COALESCE(notas_profissional, ''), atualizado_em, atualizado_por
		FROM exercicios_reabilitacao
		WHERE LOWER(nome) = LOWER(?)
	`
	row := r.db.QueryRowContext(ctx, query, nome)

	var ex domain.ExercicioReabilitacao
	var criStr string
	var updStr, updPor sql.NullString

	err := row.Scan(
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

func (r *ExercicioRepository) GetMaxCodigoInRange(ctx context.Context, min, max int) (int, error) {
	query := `SELECT MAX(codigo) FROM exercicios_reabilitacao WHERE codigo >= ? AND codigo <= ?`
	var maxVal sql.NullInt64
	err := r.db.QueryRowContext(ctx, query, min, max).Scan(&maxVal)
	if err != nil {
		return 0, err
	}
	if !maxVal.Valid {
		return 0, nil
	}
	return int(maxVal.Int64), nil
}

func (r *ExercicioRepository) Create(ctx context.Context, ex *domain.ExercicioReabilitacao) error {
	query := `
		INSERT INTO exercicios_reabilitacao (
			codigo, nome, categoria, descricao_terapeutica, descricao, indicacoes,
			contraindicacoes, restricoes_sugeridas, grupo_muscular, musculo_foco,
			tipo_exercicio, intensidade, nivel_prioridade, fonte_cientifica,
			url, url_secundaria, video_url, criado_por, criado_em, status,
			notas_profissional
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	criStr := ex.CriadoEm.Format("2006-01-02 15:04:05")
	_, err := r.db.ExecContext(ctx, query,
		ex.Codigo, ex.Nome, ex.Categoria, ex.DescricaoTerapeutica, ex.Descricao, ex.Indicacoes,
		ex.Contraindicacoes, ex.RestricoesSugeridas, ex.GrupoMuscular, ex.MusculoFoco,
		ex.TipoExercicio, ex.Intensidade, ex.NivelPrioridade, ex.FonteCientifica,
		ex.Url, ex.UrlSecundaria, ex.VideoUrl, ex.CriadoPor, criStr, ex.Status,
		ex.NotasProfissional,
	)
	return err
}

func (r *ExercicioRepository) Update(ctx context.Context, ex *domain.ExercicioReabilitacao) error {
	query := `
		UPDATE exercicios_reabilitacao SET
			nome = ?,
			descricao_terapeutica = ?,
			descricao = ?,
			indicacoes = ?,
			contraindicacoes = ?,
			restricoes_sugeridas = ?,
			grupo_muscular = ?,
			musculo_foco = ?,
			tipo_exercicio = ?,
			intensidade = ?,
			nivel_prioridade = ?,
			fonte_cientifica = ?,
			url_secundaria = ?,
			video_url = ?,
			notas_profissional = ?,
			status = ?,
			atualizado_em = ?,
			atualizado_por = ?
		WHERE codigo = ?
	`
	var updStr string
	if ex.AtualizadoEm != nil {
		updStr = ex.AtualizadoEm.Format("2006-01-02 15:04:05")
	} else {
		updStr = time.Now().Format("2006-01-02 15:04:05")
	}

	_, err := r.db.ExecContext(ctx, query,
		ex.Nome, ex.DescricaoTerapeutica, ex.Descricao, ex.Indicacoes, ex.Contraindicacoes,
		ex.RestricoesSugeridas, ex.GrupoMuscular, ex.MusculoFoco, ex.TipoExercicio,
		ex.Intensidade, ex.NivelPrioridade, ex.FonteCientifica, ex.UrlSecundaria,
		ex.VideoUrl, ex.NotasProfissional, ex.Status, updStr, ex.AtualizadoPor,
		ex.Codigo,
	)
	return err
}

func (r *ExercicioRepository) Delete(ctx context.Context, codigo int) error {
	query := `DELETE FROM exercicios_reabilitacao WHERE codigo = ?`
	_, err := r.db.ExecContext(ctx, query, codigo)
	return err
}

func (r *ExercicioRepository) List(ctx context.Context, filters map[string]string) ([]*domain.ExercicioReabilitacao, error) {
	query := `
		SELECT 
			codigo, nome, categoria, 
			COALESCE(descricao_terapeutica, ''), COALESCE(descricao, ''), COALESCE(indicacoes, ''),
			COALESCE(contraindicacoes, ''), COALESCE(restricoes_sugeridas, ''), COALESCE(grupo_muscular, ''), 
			COALESCE(musculo_foco, ''), COALESCE(tipo_exercicio, ''), COALESCE(intensidade, ''), 
			nivel_prioridade, COALESCE(fonte_cientifica, ''), COALESCE(url, ''), 
			COALESCE(url_secundaria, ''), COALESCE(video_url, ''), COALESCE(criado_por, ''), 
			criado_em, status, COALESCE(notas_profissional, ''), atualizado_em, atualizado_por
		FROM exercicios_reabilitacao
		WHERE 1=1
	`
	var args []interface{}

	if cat, ok := filters["categoria"]; ok && cat != "" {
		query += " AND categoria = ?"
		args = append(args, cat)
	}
	if stat, ok := filters["status"]; ok && stat != "" {
		query += " AND status = ?"
		args = append(args, stat)
	}
	if gp, ok := filters["grupo_muscular"]; ok && gp != "" {
		query += " AND grupo_muscular = ?"
		args = append(args, gp)
	}
	if tp, ok := filters["tipo_exercicio"]; ok && tp != "" {
		query += " AND tipo_exercicio = ?"
		args = append(args, tp)
	}
	if q, ok := filters["busca"]; ok && q != "" {
		query += " AND (nome LIKE ? OR codigo LIKE ?)"
		args = append(args, "%"+q+"%", "%"+q+"%")
	}

	query += " ORDER BY codigo DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domain.ExercicioReabilitacao
	for rows.Next() {
		var ex domain.ExercicioReabilitacao
		var criStr string
		var updStr, updPor sql.NullString

		err := rows.Scan(
			&ex.Codigo, &ex.Nome, &ex.Categoria, &ex.DescricaoTerapeutica, &ex.Descricao, &ex.Indicacoes,
			&ex.Contraindicacoes, &ex.RestricoesSugeridas, &ex.GrupoMuscular, &ex.MusculoFoco,
			&ex.TipoExercicio, &ex.Intensidade, &ex.NivelPrioridade, &ex.FonteCientifica,
			&ex.Url, &ex.UrlSecundaria, &ex.VideoUrl, &ex.CriadoPor, &criStr, &ex.Status,
			&ex.NotasProfissional, &updStr, &updPor,
		)
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

		result = append(result, &ex)
	}

	return result, nil
}

func (r *ExercicioRepository) GetUniqueGrupos(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT grupo_muscular 
		FROM exercicios_reabilitacao 
		WHERE status = 'ativo' AND grupo_muscular IS NOT NULL AND grupo_muscular != ''
		ORDER BY grupo_muscular
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grupos []string
	for rows.Next() {
		var gp string
		if err := rows.Scan(&gp); err == nil {
			grupos = append(grupos, gp)
		}
	}
	return grupos, nil
}

func (r *ExercicioRepository) GetUniqueTipos(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT tipo_exercicio 
		FROM exercicios_reabilitacao 
		WHERE status = 'ativo' AND tipo_exercicio IS NOT NULL AND tipo_exercicio != ''
		ORDER BY tipo_exercicio
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tipos []string
	for rows.Next() {
		var tp string
		if err := rows.Scan(&tp); err == nil {
			tipos = append(tipos, tp)
		}
	}
	return tipos, nil
}

func (r *ExercicioRepository) GetEstatisticas(ctx context.Context) (map[string]interface{}, error) {
	qTotal := `SELECT COUNT(*) FROM exercicios_reabilitacao WHERE status = 'ativo'`
	qTerap := `SELECT COUNT(*) FROM exercicios_reabilitacao WHERE status = 'ativo' AND categoria = 'terapeutico'`
	qNorm := `SELECT COUNT(*) FROM exercicios_reabilitacao WHERE status = 'ativo' AND categoria = 'normal'`

	var total, terapeuticos, normais int
	err := r.db.QueryRowContext(ctx, qTotal).Scan(&total)
	if err != nil {
		return nil, err
	}
	err = r.db.QueryRowContext(ctx, qTerap).Scan(&terapeuticos)
	if err != nil {
		return nil, err
	}
	err = r.db.QueryRowContext(ctx, qNorm).Scan(&normais)
	if err != nil {
		return nil, err
	}

	maxTerap, err := r.GetMaxCodigoInRange(ctx, 5000, 5999)
	if err != nil {
		return nil, err
	}
	maxNorm, err := r.GetMaxCodigoInRange(ctx, 6000, 9999)
	if err != nil {
		return nil, err
	}

	nextTerap := maxTerap + 1
	if maxTerap == 0 {
		nextTerap = 5000
	}

	nextNorm := maxNorm + 1
	if maxNorm == 0 {
		nextNorm = 6000
	}

	return map[string]interface{}{
		"total":                      total,
		"terapeuticos":               terapeuticos,
		"normais":                    normais,
		"proximo_codigo_terapeutico": nextTerap,
		"proximo_codigo_normal":      nextNorm,
	}, nil
}

func (r *ExercicioRepository) GetSugestaoByID(ctx context.Context, id int) (*domain.SugestaoExercicioRehab, error) {
	query := `
		SELECT 
			id, nome_exercicio, COALESCE(tipo_exercicio, ''), nivel_prioridade, frequencia_sugestao,
			COALESCE(exercicio_similar_nome, ''), COALESCE(rag_fonte, ''), COALESCE(justificativa_clinica, ''), status,
			COALESCE(aprovado_em, ''), COALESCE(aprovado_por, ''), exercicio_reabilitacao_codigo,
			COALESCE(notas_profissional, ''), COALESCE(motivo_rejeicao, ''), data_sugestao
		FROM sugestoes_exercicios_rehab
		WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, id)

	var s domain.SugestaoExercicioRehab
	var apEmStr, dtStr string
	var exCode sql.NullInt64

	err := row.Scan(
		&s.ID, &s.NomeExercicio, &s.TipoExercicio, &s.NivelPrioridade, &s.FrequenciaSugestao,
		&s.ExercicioSimilarNome, &s.RagFonte, &s.JustificativaClinica, &s.Status,
		&apEmStr, &s.AprovadoPor, &exCode, &s.NotasProfissional, &s.MotivoRejeicao, &dtStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.DataSugestao, _ = parseDateTime(dtStr)
	if apEmStr != "" {
		t, _ := parseDateTime(apEmStr)
		s.AprovadoEm = &t
	}
	if exCode.Valid {
		val := int(exCode.Int64)
		s.ExercicioReabilitacaoCodigo = &val
	}

	return &s, nil
}

func (r *ExercicioRepository) ListSugestoes(ctx context.Context, priorityFilter *int, order string) ([]*domain.SugestaoExercicioRehab, error) {
	query := `
		SELECT 
			id, nome_exercicio, COALESCE(tipo_exercicio, ''), nivel_prioridade, frequencia_sugestao,
			COALESCE(exercicio_similar_nome, ''), COALESCE(rag_fonte, ''), COALESCE(justificativa_clinica, ''), status,
			COALESCE(aprovado_em, ''), COALESCE(aprovado_por, ''), exercicio_reabilitacao_codigo,
			COALESCE(notas_profissional, ''), COALESCE(motivo_rejeicao, ''), data_sugestao
		FROM sugestoes_exercicios_rehab
		WHERE status = 'pendente'
	`
	var args []interface{}
	if priorityFilter != nil {
		query += " AND nivel_prioridade = ?"
		args = append(args, *priorityFilter)
	}

	if order == "prioridade" {
		query += " ORDER BY nivel_prioridade ASC, frequencia_sugestao DESC"
	} else {
		query += " ORDER BY frequencia_sugestao DESC, nivel_prioridade ASC"
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*domain.SugestaoExercicioRehab
	for rows.Next() {
		var s domain.SugestaoExercicioRehab
		var apEmStr, dtStr string
		var exCode sql.NullInt64

		err := rows.Scan(
			&s.ID, &s.NomeExercicio, &s.TipoExercicio, &s.NivelPrioridade, &s.FrequenciaSugestao,
			&s.ExercicioSimilarNome, &s.RagFonte, &s.JustificativaClinica, &s.Status,
			&apEmStr, &s.AprovadoPor, &exCode, &s.NotasProfissional, &s.MotivoRejeicao, &dtStr,
		)
		if err != nil {
			return nil, err
		}

		s.DataSugestao, _ = parseDateTime(dtStr)
		if apEmStr != "" {
			t, _ := parseDateTime(apEmStr)
			s.AprovadoEm = &t
		}
		if exCode.Valid {
			val := int(exCode.Int64)
			s.ExercicioReabilitacaoCodigo = &val
		}

		result = append(result, &s)
	}

	return result, nil
}

// AprovarSugestao aprova a sugestão e cria o exercício terapêutico na mesma transação.
func (r *ExercicioRepository) AprovarSugestao(ctx context.Context, sugestaoID int, ex *domain.ExercicioReabilitacao, approvedBy string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // Se der erro antes do Commit, dá rollback automático

	var status string
	err = tx.QueryRowContext(ctx, "SELECT status FROM sugestoes_exercicios_rehab WHERE id = ?", sugestaoID).Scan(&status)
	if err != nil {
		return 0, fmt.Errorf("sugestão não encontrada: %w", err)
	}
	if status != "pendente" {
		return 0, errors.New("sugestão já foi aprovada ou rejeitada")
	}

	var maxTerap sql.NullInt64
	err = tx.QueryRowContext(ctx, "SELECT MAX(codigo) FROM exercicios_reabilitacao WHERE codigo >= 5000 AND codigo <= 5999").Scan(&maxTerap)
	if err != nil {
		return 0, fmt.Errorf("erro ao buscar último código terapêutico: %w", err)
	}

	nextCode := 5000
	if maxTerap.Valid {
		nextCode = int(maxTerap.Int64) + 1
	}
	if nextCode > 5999 {
		return 0, errors.New("range de códigos terapêuticos esgotado")
	}

	ex.Codigo = nextCode
	ex.Url = fmt.Sprintf("https://rcstorestaff.com.br/exercicios_html/%d", nextCode)

	insertExQuery := `
		INSERT INTO exercicios_reabilitacao (
			codigo, nome, categoria, descricao_terapeutica, descricao, indicacoes,
			contraindicacoes, restricoes_sugeridas, grupo_muscular, musculo_foco,
			tipo_exercicio, intensidade, nivel_prioridade, fonte_cientifica,
			url, url_secundaria, video_url, criado_por, criado_em, status,
			notas_profissional
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ativo', ?)
	`
	criStr := ex.CriadoEm.Format("2006-01-02 15:04:05")
	_, err = tx.ExecContext(ctx, insertExQuery,
		ex.Codigo, ex.Nome, ex.Categoria, ex.DescricaoTerapeutica, ex.Descricao, ex.Indicacoes,
		ex.Contraindicacoes, ex.RestricoesSugeridas, ex.GrupoMuscular, ex.MusculoFoco,
		ex.TipoExercicio, ex.Intensidade, ex.NivelPrioridade, ex.FonteCientifica,
		ex.Url, ex.UrlSecundaria, ex.VideoUrl, ex.CriadoPor, criStr, ex.NotasProfissional,
	)
	if err != nil {
		return 0, fmt.Errorf("erro ao inserir exercício: %w", err)
	}

	apStr := time.Now().Format("2006-01-02 15:04:05")
	updateSugQuery := `
		UPDATE sugestoes_exercicios_rehab SET
			status = 'aprovado',
			aprovado_em = ?,
			aprovado_por = ?,
			exercicio_reabilitacao_codigo = ?
		WHERE id = ?
	`
	_, err = tx.ExecContext(ctx, updateSugQuery, apStr, approvedBy, ex.Codigo, sugestaoID)
	if err != nil {
		return 0, fmt.Errorf("erro ao atualizar sugestão: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("erro ao commitar transação de aprovação: %w", err)
	}

	return ex.Codigo, nil
}

func (r *ExercicioRepository) RejeitarSugestao(ctx context.Context, id int, motivo string, rejectedBy string) error {
	query := `
		UPDATE sugestoes_exercicios_rehab SET
			status = 'rejeitado',
			motivo_rejeicao = ?,
			aprovado_por = ?, -- reuso para quem moderou
			aprovado_em = ?
		WHERE id = ? AND status = 'pendente'
	`
	apStr := time.Now().Format("2006-01-02 15:04:05")
	res, err := r.db.ExecContext(ctx, query, motivo, rejectedBy, apStr, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("sugestão não encontrada ou já moderada")
	}
	return nil
}
