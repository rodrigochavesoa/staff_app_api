package sqlite

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"time"

	"staff_app/internal/domain"
)

type RAGRepository struct {
	db *DB
}

func NewRAGRepository(db *DB) *RAGRepository {
	return &RAGRepository{db: db}
}

func (r *RAGRepository) GetCachedQuery(ctx context.Context, queryNorm, modalidade, objetivo, perfil string, k int) (*domain.ConsultaBaseConhecimento, error) {
	var c domain.ConsultaBaseConhecimento
	var mod, obj, perf sql.NullString
	var userID sql.NullInt64
	var criadoEmStr, ultimaUtilizacaoStr string

	err := r.db.QueryRowContext(ctx, `
		SELECT id, query_original, query_normalizada, modalidade, objetivo, perfil, k, 
		       total_resultados, hits, resultados_json, usuario_id, criado_em, ultima_utilizacao
		FROM consultas_base_conhecimento
		WHERE query_normalizada = ? 
		  AND COALESCE(modalidade, '') = ? 
		  AND COALESCE(objetivo, '') = ? 
		  AND COALESCE(perfil, '') = ? 
		  AND k = ?
	`, queryNorm, modalidade, objetivo, perfil, k).Scan(
		&c.ID, &c.QueryOriginal, &c.QueryNormalizada, &mod, &obj, &perf, &c.K,
		&c.TotalResultados, &c.Hits, &c.ResultadosJSON, &userID, &criadoEmStr, &ultimaUtilizacaoStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if mod.Valid {
		c.Modalidade = &mod.String
	}
	if obj.Valid {
		c.Objetivo = &obj.String
	}
	if perf.Valid {
		c.Perfil = &perf.String
	}
	if userID.Valid {
		c.UsuarioID = &userID.Int64
	}

	if t, err := time.Parse("2006-01-02 15:04:05", criadoEmStr); err == nil {
		c.CriadoEm = t
	} else if t, err := time.Parse(time.RFC3339, criadoEmStr); err == nil {
		c.CriadoEm = t
	}
	if t, err := time.Parse("2006-01-02 15:04:05", ultimaUtilizacaoStr); err == nil {
		c.UltimaUtilizacao = t
	} else if t, err := time.Parse(time.RFC3339, ultimaUtilizacaoStr); err == nil {
		c.UltimaUtilizacao = t
	}

	return &c, nil
}

func (r *RAGRepository) IncrementCacheHits(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE consultas_base_conhecimento
		SET hits = hits + 1, ultima_utilizacao = CURRENT_TIMESTAMP
		WHERE id = ?
	`, id)
	return err
}

func (r *RAGRepository) SaveCachedQuery(ctx context.Context, queryOrig, queryNorm, modalidade, objetivo, perfil string, k int, totalResultados int, resultadosJSON string, usuarioID *int64) error {
	var modVal, objVal, perfVal sql.NullString
	if modalidade != "" {
		modVal = sql.NullString{String: modalidade, Valid: true}
	}
	if objetivo != "" {
		objVal = sql.NullString{String: objetivo, Valid: true}
	}
	if perfil != "" {
		perfVal = sql.NullString{String: perfil, Valid: true}
	}

	var userVal sql.NullInt64
	if usuarioID != nil {
		userVal = sql.NullInt64{Int64: *usuarioID, Valid: true}
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO consultas_base_conhecimento (
			query_original, query_normalizada, modalidade, objetivo, perfil, k, 
			total_resultados, hits, resultados_json, usuario_id, criado_em, ultima_utilizacao
		) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, queryOrig, queryNorm, modVal, objVal, perfVal, k, totalResultados, resultadosJSON, userVal)
	return err
}

func (r *RAGRepository) SearchLocalDocuments(ctx context.Context, query string, modalidade string, k int) ([]domain.KnowledgeDocument, error) {
	searchPattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	modLower := strings.ToLower(strings.TrimSpace(modalidade))

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, fonte, COALESCE(titulo, ''), conteudo, COALESCE(tags, ''), COALESCE(modalidade, '')
		FROM base_conhecimento_documentos
		WHERE ativo = 1 AND (LOWER(conteudo) LIKE ? OR LOWER(titulo) LIKE ? OR LOWER(tags) LIKE ?)
		ORDER BY CASE WHEN LOWER(COALESCE(modalidade, '')) = ? THEN 0 ELSE 1 END, id DESC
		LIMIT ?
	`, searchPattern, searchPattern, searchPattern, modLower, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // #nosec G104

	var list []domain.KnowledgeDocument
	rank := 1
	for rows.Next() {
		var id int64
		var fonte, titulo, conteudo, tagsStr, mod string
		if err := rows.Scan(&id, &fonte, &titulo, &conteudo, &tagsStr, &mod); err != nil {
			return nil, err
		}

		var tags []string
		if tagsStr != "" {
			parts := strings.Split(tagsStr, ",")
			for _, p := range parts {
				t := strings.TrimSpace(p)
				if t != "" {
					tags = append(tags, t)
				}
			}
		}

		list = append(list, domain.KnowledgeDocument{
			Rank:       rank,
			Fonte:      fonte,
			Conteudo:   conteudo,
			Tags:       tags,
			Relevancia: 1.0,
		})
		rank++
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return list, nil
}

func (r *RAGRepository) SeedLocalDocument(ctx context.Context, fonte, titulo, conteudo, tags, modalidade string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO base_conhecimento_documentos (fonte, titulo, conteudo, tags, modalidade, ativo)
		VALUES (?, ?, ?, ?, ?, 1)
	`, fonte, titulo, conteudo, tags, modalidade)
	return err
}

func (r *RAGRepository) GetHistorico(ctx context.Context, limit int) ([]map[string]any, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT query_original, COALESCE(modalidade, ''), COALESCE(objetivo, ''), COALESCE(perfil, ''), total_resultados, hits, ultima_utilizacao
		FROM consultas_base_conhecimento
		ORDER BY ultima_utilizacao DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // #nosec G104

	list := make([]map[string]any, 0)
	for rows.Next() {
		var query, mod, obj, perf, ultUtil string
		var totalResultados, hits int
		if err := rows.Scan(&query, &mod, &obj, &perf, &totalResultados, &hits, &ultUtil); err != nil {
			return nil, err
		}
		list = append(list, map[string]any{
			"query":             query,
			"modalidade":         mod,
			"objetivo":           obj,
			"perfil":             perf,
			"total_resultados":  totalResultados,
			"hits":              hits,
			"ultima_utilizacao": ultUtil,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return list, nil
}

func (r *RAGRepository) GetEstatisticas(ctx context.Context) (map[string]any, error) {
	var totalConsultasUnicas int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM consultas_base_conhecimento").Scan(&totalConsultasUnicas)
	if err != nil {
		return nil, err
	}

	var totalHits int
	err = r.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(hits), 0) FROM consultas_base_conhecimento").Scan(&totalHits)
	if err != nil {
		return nil, err
	}

	var economiaChamadasAPI int
	err = r.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(hits - 1), 0) FROM consultas_base_conhecimento").Scan(&economiaChamadasAPI)
	if err != nil {
		return nil, err
	}

	taxaHitCache := 0.0
	if totalHits > 0 {
		taxaHitCache = math.Round((float64(economiaChamadasAPI)/float64(totalHits))*1000) / 10
	}

	// Top 5 consultas
	topRows, err := r.db.QueryContext(ctx, `
		SELECT query_original, COALESCE(modalidade, ''), COALESCE(objetivo, ''), hits
		FROM consultas_base_conhecimento
		ORDER BY hits DESC
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer topRows.Close() // #nosec G104

	var top5 []map[string]any
	for topRows.Next() {
		var query, mod, obj string
		var hits int
		if err := topRows.Scan(&query, &mod, &obj, &hits); err != nil {
			return nil, err
		}
		top5 = append(top5, map[string]any{
			"query":      query,
			"modalidade": mod,
			"objetivo":   obj,
			"hits":       hits,
		})
	}
	if err := topRows.Err(); err != nil {
		return nil, err
	}

	// Por modalidade
	modRows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(modalidade, 'Geral'), COUNT(*), SUM(hits)
		FROM consultas_base_conhecimento
		GROUP BY COALESCE(modalidade, 'Geral')
	`)
	if err != nil {
		return nil, err
	}
	defer modRows.Close() // #nosec G104

	var porModalidade []map[string]any
	for modRows.Next() {
		var mod string
		var consultas, hits int
		if err := modRows.Scan(&mod, &consultas, &hits); err != nil {
			return nil, err
		}
		porModalidade = append(porModalidade, map[string]any{
			"modalidade": mod,
			"consultas":  consultas,
			"hits":       hits,
		})
	}
	if err := modRows.Err(); err != nil {
		return nil, err
	}

	return map[string]any{
		"total_consultas_unicas": totalConsultasUnicas,
		"total_hits":             totalHits,
		"economia_chamadas_api":  economiaChamadasAPI,
		"taxa_hit_cache":         taxaHitCache,
		"top_5_consultas":        top5,
		"por_modalidade":         porModalidade,
	}, nil
}

func (r *RAGRepository) GetPopulares(ctx context.Context) ([]map[string]any, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT query_original, COALESCE(modalidade, ''), COALESCE(objetivo, ''), hits
		FROM consultas_base_conhecimento
		ORDER BY hits DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // #nosec G104

	var list []map[string]any
	for rows.Next() {
		var query, mod, obj string
		var hits int
		if err := rows.Scan(&query, &mod, &obj, &hits); err != nil {
			return nil, err
		}
		list = append(list, map[string]any{
			"query":      query,
			"modalidade": mod,
			"objetivo":   obj,
			"hits":       hits,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return list, nil
}
