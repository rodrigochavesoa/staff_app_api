package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"staff_app/internal/platform/logger"
)

func TestSVEDRepositoryReads(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-sved-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	ctx := t.Context()
	_, err = db.ExecContext(ctx, `
		INSERT INTO alunos (id, nome, idade, sexo, email) VALUES (1, 'Aluno SVED', 30, 'M', 'sved@test.com')
	`)
	if err != nil {
		t.Fatalf("seed aluno: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_treino_web (
			id, aluno, turma, data_criacao, ficha_json, ies_score, densidade, volume_sved, tut_total
		) VALUES (
			10, 'Aluno SVED', 'Ficha A', '2026-07-20 10:00:00',
			'{"exercicios":[{"nome":"Supino","series":3,"repeticoes":10,"cadencia":"4010","descanso":"60s","rir":2}]}',
			7.5, 0.42, 30, 120
		)
	`)
	if err != nil {
		t.Fatalf("seed ficha: %v", err)
	}

	repo := NewSVEDRepository(db)

	nome, err := repo.GetAlunoNomeByID(ctx, 1)
	if err != nil || nome != "Aluno SVED" {
		t.Fatalf("GetAlunoNomeByID: nome=%q err=%v", nome, err)
	}

	sheets, err := repo.ListFichaSheetsByAlunoNome(ctx, nome, 20)
	if err != nil || len(sheets) != 1 || sheets[0].ID != 10 {
		t.Fatalf("ListFichaSheets: %+v err=%v", sheets, err)
	}

	aluno, err := repo.GetFichaAlunoByID(ctx, 10)
	if err != nil || aluno != "Aluno SVED" {
		t.Fatalf("GetFichaAlunoByID: %q err=%v", aluno, err)
	}

	detail, err := repo.GetFichaDetailByID(ctx, 10)
	if err != nil || detail.Titulo != "Ficha A" {
		t.Fatalf("GetFichaDetailByID: %+v err=%v", detail, err)
	}

	id, ok, err := repo.GetAlunoIDByNomeLatest(ctx, "Aluno SVED")
	if err != nil || !ok || id != 1 {
		t.Fatalf("GetAlunoIDByNomeLatest: id=%d ok=%v err=%v", id, ok, err)
	}

	stats, err := repo.GetAggregatedStatsByAluno(ctx, nome)
	if err != nil || stats.IesMedio != 7.5 {
		t.Fatalf("GetAggregatedStats: %+v err=%v", stats, err)
	}

	dash, err := repo.ListDashboardSheetsByAluno(ctx, nome, 20)
	if err != nil || len(dash) != 1 || dash[0].VolumeSved != 30 {
		t.Fatalf("ListDashboardSheets: %+v err=%v", dash, err)
	}
}
