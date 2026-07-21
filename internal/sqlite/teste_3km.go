package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"staff_app/internal/domain"
)

type Teste3kmRepository struct {
	db *DB
}

func NewTeste3kmRepository(db *DB) *Teste3kmRepository {
	return &Teste3kmRepository{db: db}
}

func (r *Teste3kmRepository) Create(ctx context.Context, t *domain.Teste3km) error {
	query := `
		INSERT INTO teste_3km (
			aluno_id, data_teste, tempo_segundos, pse, fonte, vdot, ftp_pace_segundos,
			pace_z1_min, pace_z1_max, pace_z2_min, pace_z2_max,
			pace_z3_min, pace_z3_max, pace_z4_min, pace_z4_max,
			pace_z5_min, pace_z5_max, indice_confianca, observacoes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Data do teste em TEXT (YYYY-MM-DD).
	dataStr := t.DataTeste.Format("2006-01-02")

	res, err := r.db.ExecContext(ctx, query,
		t.AlunoID,
		dataStr,
		t.TempoSegundos,
		t.PSE,
		t.Fonte,
		t.VDOT,
		t.FTPPaceSegundos,
		t.PaceZ1Min, t.PaceZ1Max,
		t.PaceZ2Min, t.PaceZ2Max,
		t.PaceZ3Min, t.PaceZ3Max,
		t.PaceZ4Min, t.PaceZ4Max,
		t.PaceZ5Min, t.PaceZ5Max,
		t.IndiceConfianca,
		t.Observacoes,
	)
	if err != nil {
		return fmt.Errorf("failed to insert 3k test: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to retrieve last insert id: %w", err)
	}
	t.ID = id

	return nil
}

func (r *Teste3kmRepository) ListByAlunoID(ctx context.Context, alunoID int64) ([]*domain.Teste3km, error) {
	query := `
		SELECT 
			id, aluno_id, data_teste, tempo_segundos, pse, fonte, vdot, ftp_pace_segundos,
			pace_z1_min, pace_z1_max, pace_z2_min, pace_z2_max,
			pace_z3_min, pace_z3_max, pace_z4_min, pace_z4_max,
			pace_z5_min, pace_z5_max, indice_confianca, observacoes
		FROM teste_3km
		WHERE aluno_id = ?
		ORDER BY data_teste DESC, id DESC
	`

	rows, err := r.db.QueryContext(ctx, query, alunoID)
	if err != nil {
		return nil, fmt.Errorf("failed to query 3k tests: %w", err)
	}
	defer rows.Close()

	var tests []*domain.Teste3km
	for rows.Next() {
		var t domain.Teste3km
		var dataStr string
		var pseVal, confVal sql.NullInt64

		err := rows.Scan(
			&t.ID, &t.AlunoID, &dataStr, &t.TempoSegundos, &pseVal, &t.Fonte, &t.VDOT, &t.FTPPaceSegundos,
			&t.PaceZ1Min, &t.PaceZ1Max, &t.PaceZ2Min, &t.PaceZ2Max,
			&t.PaceZ3Min, &t.PaceZ3Max, &t.PaceZ4Min, &t.PaceZ4Max,
			&t.PaceZ5Min, &t.PaceZ5Max, &confVal, &t.Observacoes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan 3k test row: %w", err)
		}

		parsedTime, err := time.Parse("2006-01-02", dataStr)
		if err != nil {
			parsedTime, _ = time.Parse(time.RFC3339, dataStr)
		}
		t.DataTeste = parsedTime

		if pseVal.Valid {
			val := int(pseVal.Int64)
			t.PSE = &val
		}
		if confVal.Valid {
			val := int(confVal.Int64)
			t.IndiceConfianca = &val
		}

		tests = append(tests, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return tests, nil
}

// Delete exige id + aluno_id para impedir exclusão cruzada entre alunos.
func (r *Teste3kmRepository) Delete(ctx context.Context, id int64, alunoID int64) error {
	query := `DELETE FROM teste_3km WHERE id = ? AND aluno_id = ?`
	res, err := r.db.ExecContext(ctx, query, id, alunoID)
	if err != nil {
		return fmt.Errorf("failed to delete 3k test: %w", err)
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
