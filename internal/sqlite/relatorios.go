package sqlite

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/services"
)

type RelatoriosRepository struct {
	db *DB
}

func NewRelatoriosRepository(db *DB) *RelatoriosRepository {
	return &RelatoriosRepository{db: db}
}

func extrairExerciciosDeFichaJSON(fichaJSON string) []services.ExercicioJSON {
	var parsed struct {
		Exercicios []services.ExercicioJSON `json:"exercicios"`
		Treinos    []struct {
			Exercicios []services.ExercicioJSON `json:"exercicios"`
		} `json:"treinos"`
	}
	if err := json.Unmarshal([]byte(fichaJSON), &parsed); err != nil {
		return nil
	}

	var list []services.ExercicioJSON
	list = append(list, parsed.Exercicios...)
	for _, t := range parsed.Treinos {
		list = append(list, t.Exercicios...)
	}
	return list
}

func (r *RelatoriosRepository) GetDashboardResumo(ctx context.Context) (*domain.RelatoriosDashboardResumo, error) {
	var resumo domain.RelatoriosDashboardResumo
	resumo.DataAtualizacao = time.Now().Format("2006-01-02 15:04:05")

	// 1. Total de exercícios ativos
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM exercicios_reabilitacao WHERE status = 'ativo'").Scan(&resumo.TotalExerciciosAtivos)
	if err != nil {
		return nil, err
	}

	// 2. Total de sugestões pendentes
	err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sugestoes_exercicios_rehab WHERE status = 'pendente'").Scan(&resumo.SugestoesPendentes)
	if err != nil {
		return nil, err
	}

	// 3. Sugestões nos últimos 30 dias
	time30dAgo := time.Now().AddDate(0, 0, -30).Format("2006-01-02 15:04:05")
	err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sugestoes_exercicios_rehab WHERE data_sugestao >= ?", time30dAgo).Scan(&resumo.SugestoesUltimos30d)
	if err != nil {
		return nil, err
	}

	// 4. Taxa de aprovação nos últimos 30 dias
	var aprovados30d, total30d int
	err = r.db.QueryRowContext(ctx, `
		SELECT 
			COALESCE(SUM(CASE WHEN status = 'aprovado' THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM sugestoes_exercicios_rehab
		WHERE data_sugestao >= ?
	`, time30dAgo).Scan(&aprovados30d, &total30d)
	if err == nil && total30d > 0 {
		resumo.TaxaAprovacao30dPct = math.Round((float64(aprovados30d)/float64(total30d))*1000) / 10
	}

	// 5. Total de recomendações (soma de frequencia_sugestao das sugestões + utilizações reais)
	var freqSugSoma int
	err = r.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(frequencia_sugestao), 0) FROM sugestoes_exercicios_rehab").Scan(&freqSugSoma)
	if err != nil {
		return nil, err
	}

	// 6. Processar utilizações a partir de fichas_treino_web
	rows, err := r.db.QueryContext(ctx, "SELECT ficha_json FROM fichas_treino_web")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	exerciseUsageCount := make(map[string]int)
	totalUtilizacoes := 0

	for rows.Next() {
		var fichaJSON string
		if err := rows.Scan(&fichaJSON); err != nil {
			continue
		}
		exs := extrairExerciciosDeFichaJSON(fichaJSON)
		for _, ex := range exs {
			nameLower := strings.ToLower(strings.TrimSpace(ex.Nome))
			if nameLower != "" {
				exerciseUsageCount[nameLower]++
				totalUtilizacoes++
			}
		}
	}

	resumo.TotalUtilizacoes = totalUtilizacoes
	resumo.TotalRecomendacoes = freqSugSoma + totalUtilizacoes
	if resumo.TotalRecomendacoes > 0 {
		resumo.TaxaUsoGlobalPct = math.Round((float64(totalUtilizacoes)/float64(resumo.TotalRecomendacoes))*1000) / 10
	}

	// 7. Exercícios ativos nunca usados
	exRows, err := r.db.QueryContext(ctx, "SELECT nome FROM exercicios_reabilitacao WHERE status = 'ativo'")
	if err == nil {
		defer exRows.Close()
		nuncaUsados := 0
		for exRows.Next() {
			var nome string
			if err := exRows.Scan(&nome); err == nil {
				nameLower := strings.ToLower(strings.TrimSpace(nome))
				if exerciseUsageCount[nameLower] == 0 {
					nuncaUsados++
				}
			}
		}
		resumo.ExerciciosNuncaUsados = nuncaUsados
	}

	return &resumo, nil
}

func (r *RelatoriosRepository) GetPatologiasCobertura(ctx context.Context) ([]domain.RelatorioPatologiaItem, error) {
	// Obter todas as anamneses aprovadas com patologias
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.nome, an.patologias 
		FROM alunos a 
		JOIN anamneses an ON a.id = an.aluno_id 
		WHERE an.status_aprovacao = 'aprovada' AND an.patologias IS NOT NULL AND an.patologias != ''
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Mapear patologias para alunos
	patologyStudents := make(map[string][]string)
	for rows.Next() {
		var alunoNome, patologiasStr string
		if err := rows.Scan(&alunoNome, &patologiasStr); err != nil {
			continue
		}
		parts := strings.Split(patologiasStr, ",")
		for _, part := range parts {
			partClean := strings.ToLower(strings.TrimSpace(part))
			if partClean == "" {
				continue
			}
			words := strings.Fields(partClean)
			for idx, w := range words {
				if len(w) > 0 {
					words[idx] = strings.ToUpper(w[:1]) + w[1:]
				}
			}
			pat := strings.Join(words, " ")
			if pat != "" {
				patologyStudents[pat] = append(patologyStudents[pat], alunoNome)
			}
		}
	}
	rows.Close() // #nosec G104

	var list []domain.RelatorioPatologiaItem

	for pat, students := range patologyStudents {
		// Encontrar quantidade de exercícios na tabela de exercícios que targetam essa patologia
		var totalExerciciosDisponiveis int
		patSearch := "%" + strings.ToLower(pat) + "%"
		err = r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM exercicios_reabilitacao 
			WHERE status = 'ativo' AND (
				LOWER(nome) LIKE ? OR 
				LOWER(indicacoes) LIKE ? OR 
				LOWER(descricao_terapeutica) LIKE ?
			)
		`, patSearch, patSearch, patSearch).Scan(&totalExerciciosDisponiveis)
		if err != nil {
			totalExerciciosDisponiveis = 1
		}
		if totalExerciciosDisponiveis == 0 {
			totalExerciciosDisponiveis = 1
		}

		// Encontrar total de utilizações reais nas fichas de treino desses alunos
		totalUtilizacoes := 0
		uniqueExerciciosPrescritos := make(map[string]bool)

		for _, student := range students {
			var fichaJSON string
			err = r.db.QueryRowContext(ctx, `
				SELECT ficha_json FROM fichas_treino_web 
				WHERE aluno = ? 
				ORDER BY data_criacao DESC LIMIT 1
			`, student).Scan(&fichaJSON)
			if err == nil {
				exs := extrairExerciciosDeFichaJSON(fichaJSON)
				for _, ex := range exs {
					nameClean := strings.ToLower(strings.TrimSpace(ex.Nome))
					if nameClean != "" {
						uniqueExerciciosPrescritos[nameClean] = true
						totalUtilizacoes++
					}
				}
			}
		}

		mediaUso := 0.0
		if totalExerciciosDisponiveis > 0 {
			mediaUso = math.Round((float64(totalUtilizacoes)/float64(totalExerciciosDisponiveis))*10) / 10
		}

		list = append(list, domain.RelatorioPatologiaItem{
			PatologiaAlvo:        pat,
			TotalExercicios:      totalExerciciosDisponiveis,
			TotalUtilizacoes:     totalUtilizacoes,
			MediaUsoPorExercicio: mediaUso,
		})
	}

	return list, nil
}

func (r *RelatoriosRepository) GetExerciciosSubutilizados(ctx context.Context, minRecomendacoes int) ([]domain.ExercicioSubutilizadoItem, error) {
	// 1. Contar utilizações de exercícios reais nas fichas_treino_web
	rows, err := r.db.QueryContext(ctx, "SELECT ficha_json FROM fichas_treino_web")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	exerciseUsageCount := make(map[string]int)
	for rows.Next() {
		var fichaJSON string
		if err := rows.Scan(&fichaJSON); err != nil {
			continue
		}
		exs := extrairExerciciosDeFichaJSON(fichaJSON)
		for _, ex := range exs {
			nameLower := strings.ToLower(strings.TrimSpace(ex.Nome))
			if nameLower != "" {
				exerciseUsageCount[nameLower]++
			}
		}
	}
	rows.Close() // #nosec G104

	// 2. Contar sugestões acumuladas da IA
	sugRows, err := r.db.QueryContext(ctx, `
		SELECT LOWER(TRIM(nome_exercicio)), COALESCE(SUM(frequencia_sugestao), 0)
		FROM sugestoes_exercicios_rehab
		GROUP BY LOWER(TRIM(nome_exercicio))
	`)
	sugestionsMap := make(map[string]int)
	if err == nil {
		defer sugRows.Close()
		for sugRows.Next() {
			var name string
			var freq int
			if err := sugRows.Scan(&name, &freq); err == nil {
				sugestionsMap[name] = freq
			}
		}
		sugRows.Close() // #nosec G104
	}

	// 3. Obter todos os exercícios ativos
	exRows, err := r.db.QueryContext(ctx, `
		SELECT codigo, nome, COALESCE(grupo_muscular, 'Geral'), COALESCE(indicacoes, 'N/A')
		FROM exercicios_reabilitacao
		WHERE status = 'ativo'
	`)
	if err != nil {
		return nil, err
	}
	defer exRows.Close()

	var list []domain.ExercicioSubutilizadoItem
	for exRows.Next() {
		var item domain.ExercicioSubutilizadoItem
		if err := exRows.Scan(&item.Codigo, &item.Nome, &item.GrupoMuscular, &item.PatologiaAlvo); err != nil {
			continue
		}

		nameLower := strings.ToLower(strings.TrimSpace(item.Nome))
		item.VezesUsado = exerciseUsageCount[nameLower]
		
		sugFreq := sugestionsMap[nameLower]
		item.VezesRecomendado = sugFreq + item.VezesUsado
		if item.VezesRecomendado == 0 {
			item.VezesRecomendado = 1
		}

		if item.VezesRecomendado < minRecomendacoes {
			continue
		}

		item.TaxaUsoPct = math.Round((float64(item.VezesUsado)/float64(item.VezesRecomendado))*1000) / 10

		list = append(list, item)
	}

	return list, nil
}

func (r *RelatoriosRepository) GetRelatorioAprovacao(ctx context.Context, dias int) ([]domain.RelatorioAprovacaoItem, error) {
	timeLimit := time.Now().AddDate(0, 0, -dias).Format("2006-01-02 15:04:05")

	rows, err := r.db.QueryContext(ctx, `
		SELECT 
			s.nome_exercicio,
			COUNT(s.id),
			COALESCE(SUM(CASE WHEN s.status = 'aprovado' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN s.status = 'rejeitado' THEN 1 ELSE 0 END), 0),
			COALESCE(e.codigo, 0)
		FROM sugestoes_exercicios_rehab s
		LEFT JOIN exercicios_reabilitacao e ON s.nome_exercicio = e.nome
		WHERE s.data_sugestao >= ?
		GROUP BY s.nome_exercicio, e.codigo
	`, timeLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.RelatorioAprovacaoItem
	for rows.Next() {
		var item domain.RelatorioAprovacaoItem
		if err := rows.Scan(&item.NomeExercicio, &item.TotalSugestoes, &item.Aprovadas, &item.Rejeitadas, &item.CodigoExercicio); err != nil {
			continue
		}

		if item.TotalSugestoes > 0 {
			item.TaxaAprovacaoPct = math.Round((float64(item.Aprovadas)/float64(item.TotalSugestoes))*1000) / 10
		}

		list = append(list, item)
	}
	rows.Close() // #nosec G104

	return list, nil
}
