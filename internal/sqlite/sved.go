package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"staff_app/internal/repositories"
)

type SVEDRepository struct {
	db *DB
}

func NewSVEDRepository(db *DB) *SVEDRepository {
	return &SVEDRepository{db: db}
}

func (r *SVEDRepository) GetAlunoNomeByID(ctx context.Context, alunoID int64) (string, error) {
	var nome string
	err := r.db.QueryRowContext(ctx, "SELECT nome FROM alunos WHERE id = ?", alunoID).Scan(&nome)
	if err != nil {
		return "", err
	}
	return nome, nil
}

func (r *SVEDRepository) ListFichaSheetsByAlunoNome(ctx context.Context, nome string, limit int) ([]repositories.SVEDSheet, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, COALESCE(data_criacao, ''), ficha_json
		FROM fichas_treino_web
		WHERE aluno = ?
		ORDER BY data_criacao DESC
		LIMIT ?
	`, nome, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sheets := make([]repositories.SVEDSheet, 0, limit)
	for rows.Next() {
		var sheet repositories.SVEDSheet
		if err := rows.Scan(&sheet.ID, &sheet.DataCriacao, &sheet.FichaJSON); err != nil {
			return nil, err
		}
		sheets = append(sheets, sheet)
	}
	return sheets, rows.Err()
}

func (r *SVEDRepository) GetFichaAlunoByID(ctx context.Context, fichaID int64) (string, error) {
	var aluno string
	err := r.db.QueryRowContext(ctx, "SELECT aluno FROM fichas_treino_web WHERE id = ?", fichaID).Scan(&aluno)
	if err != nil {
		return "", err
	}
	return aluno, nil
}

func (r *SVEDRepository) GetFichaDetailByID(ctx context.Context, fichaID int64) (*repositories.SVEDFichaDetail, error) {
	var d repositories.SVEDFichaDetail
	err := r.db.QueryRowContext(ctx, `
		SELECT aluno, COALESCE(turma, 'Ficha'), ficha_json
		FROM fichas_treino_web
		WHERE id = ?
	`, fichaID).Scan(&d.AlunoNome, &d.Titulo, &d.FichaJSON)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *SVEDRepository) GetAlunoIDByNomeLatest(ctx context.Context, nome string) (int64, bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM alunos WHERE nome = ? ORDER BY id DESC LIMIT 1
	`, nome).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (r *SVEDRepository) GetAggregatedStatsByAluno(ctx context.Context, nome string) (*repositories.SVEDAggregatedStats, error) {
	var s repositories.SVEDAggregatedStats
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(ies_score), 0.0), COALESCE(AVG(densidade), 0.0), COALESCE(AVG(volume_sved), 0.0)
		FROM fichas_treino_web
		WHERE aluno = ?
	`, nome).Scan(&s.IesMedio, &s.DensidadeMedia, &s.VolumeEfetivo)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SVEDRepository) ListDashboardSheetsByAluno(ctx context.Context, nome string, limit int) ([]repositories.SVEDDashboardSheet, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, COALESCE(turma, 'Ficha'), data_criacao, ies_score, tut_total, densidade, volume_sved, ficha_json
		FROM fichas_treino_web
		WHERE aluno = ?
		ORDER BY data_criacao DESC
		LIMIT ?
	`, nome, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]repositories.SVEDDashboardSheet, 0, limit)
	for rows.Next() {
		var s repositories.SVEDDashboardSheet
		if err := rows.Scan(
			&s.ID, &s.Turma, &s.DataCriacao, &s.IesScore, &s.TutTotal, &s.Densidade, &s.VolumeSved, &s.FichaJSON,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
