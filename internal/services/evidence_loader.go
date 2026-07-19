package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"staff_app/internal/domain"
)

// ActiveAnamneseFinder loads the active anamnese for an athlete.
type ActiveAnamneseFinder interface {
	FindActiveByAlunoID(ctx context.Context, alunoID int64) (*domain.Anamnese, error)
}

// LocalDocumentSearcher searches the local knowledge base documents table.
type LocalDocumentSearcher interface {
	SearchLocalDocuments(ctx context.Context, query string, modalidade string, k int) ([]domain.KnowledgeDocument, error)
}

// LocalDocumentCandidateSource returns a broad candidate set for hybrid ranking.
// Optional: HybridKnowledgeEvidenceSearcher prefers this over substring SearchLocalDocuments.
type LocalDocumentCandidateSource interface {
	SearchLocalDocumentCandidates(ctx context.Context, query string, modalidade string, k int) ([]domain.KnowledgeDocument, error)
}

// ContextQueryDB is the SQL surface needed for history and SVED aggregates.
type ContextQueryDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLStructuredContextLoader loads relational athlete context only (no evidence, no classify).
type SQLStructuredContextLoader struct {
	DB       ContextQueryDB
	Anamnese ActiveAnamneseFinder
}

func NewSQLStructuredContextLoader(db ContextQueryDB, anam ActiveAnamneseFinder) *SQLStructuredContextLoader {
	return &SQLStructuredContextLoader{DB: db, Anamnese: anam}
}

func (l *SQLStructuredContextLoader) Load(ctx context.Context, alunoID int64, req GenerationRequest) (*AthleteTrainingContext, error) {
	if l == nil || l.DB == nil || l.Anamnese == nil {
		return nil, fmt.Errorf("structured context loader is not configured")
	}

	result := &AthleteTrainingContext{
		Complexidade: "simples",
		DadosUsados:  []string{"aluno", "ficha_local"},
	}

	anamnese, err := l.Anamnese.FindActiveByAlunoID(ctx, alunoID)
	if err != nil {
		return nil, fmt.Errorf("loading active anamnese: %w", err)
	}
	if anamnese != nil {
		result.Anamnese = AnamneseToTrainingHint(anamnese)
		result.DadosUsados = append(result.DadosUsados, "anamnese")
	}

	historico, err := l.loadTrainingHistory(ctx, alunoID)
	if err != nil {
		return nil, err
	}
	if len(historico) > 0 {
		result.Historico = historico
		result.DadosUsados = append(result.DadosUsados, "historico_fichas")
	}

	sved, err := l.loadSVEDSummary(ctx, alunoID)
	if err != nil {
		return nil, err
	}
	if sved != nil {
		result.SVED = sved
		result.DadosUsados = append(result.DadosUsados, "sved")
	}

	return result, nil
}

// AnamneseToTrainingHint maps a domain anamnese to the training hint DTO.
func AnamneseToTrainingHint(a *domain.Anamnese) *AnamneseTrainingHint {
	if a == nil {
		return nil
	}
	return &AnamneseTrainingHint{
		StatusAprovacao: a.StatusAprovacao,
		Patologias:      a.Patologias,
		LesoesAtuais:    a.LesoesAtuais,
		DoresCronicas:   a.DoresCronicas,
		Medicamentos:    a.Medicamentos,
		RiskScore:       a.RiskScoreCached,
		Experiencia:     a.ExperienciaTreino,
		Objetivo:        a.ObjetivoPrincipal,
	}
}

func (l *SQLStructuredContextLoader) loadTrainingHistory(ctx context.Context, alunoID int64) ([]TrainingHistoryHint, error) {
	rows, err := l.DB.QueryContext(ctx, `
		SELECT id, COALESCE(tipo_ficha, ''), COALESCE(objetivo, ''), COALESCE(nivel, ''),
		       COALESCE(ies_score, 0), COALESCE(volume_sved, 0), COALESCE(data_criacao, '')
		FROM fichas_treino_web
		WHERE aluno IN (SELECT nome FROM alunos WHERE id = ?)
		ORDER BY id DESC
		LIMIT 3
	`, alunoID)
	if err != nil {
		return nil, fmt.Errorf("loading training history: %w", err)
	}
	defer rows.Close() // #nosec G104

	historico := make([]TrainingHistoryHint, 0, 3)
	for rows.Next() {
		var item TrainingHistoryHint
		if err := rows.Scan(&item.ID, &item.TipoFicha, &item.Objetivo, &item.Nivel, &item.IesScore, &item.Volume, &item.Data); err != nil {
			return nil, fmt.Errorf("scanning training history: %w", err)
		}
		item.Status = "registrada"
		historico = append(historico, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating training history: %w", err)
	}
	return historico, nil
}

func (l *SQLStructuredContextLoader) loadSVEDSummary(ctx context.Context, alunoID int64) (*SVEDTrainingHint, error) {
	var summary SVEDTrainingHint
	err := l.DB.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(ies_score), 0), COALESCE(AVG(densidade), 0),
		       COALESCE(AVG(volume_sved), 0), COUNT(*)
		FROM fichas_treino_web
		WHERE aluno IN (SELECT nome FROM alunos WHERE id = ?)
		  AND COALESCE(volume_sved, 0) > 0
	`, alunoID).Scan(&summary.IesMedio, &summary.DensidadeMedia, &summary.VolumeMedio, &summary.Fichas)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("loading sved summary: %w", err)
	}
	if summary.Fichas == 0 {
		return nil, nil
	}
	return &summary, nil
}

func truncateEvidenceText(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}
