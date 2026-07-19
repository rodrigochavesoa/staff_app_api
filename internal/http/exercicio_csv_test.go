package http

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCSVExercisesReadsGrupoMuscularAndFoco(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exercicios.csv")
	content := "Código,Nome do Exercício,grupo_muscular,musculo_foco,url\n" +
		"100,Abdominal Amplitude Máxima,Abdome,Reto Abdominal,https://example.com/100\n" +
		"200,Supino Reto,Peito,Peitoral Maior,https://example.com/200\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	list, err := loadCSVExercises(path)
	if err != nil {
		t.Fatalf("loadCSVExercises: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d want 2", len(list))
	}
	if list[0].GrupoMuscular != "Abdome" {
		t.Fatalf("grupo[0]=%q want Abdome", list[0].GrupoMuscular)
	}
	if list[0].MusculoFoco != "Reto Abdominal" {
		t.Fatalf("foco[0]=%q want Reto Abdominal", list[0].MusculoFoco)
	}
	if list[1].GrupoMuscular != "Peito" {
		t.Fatalf("grupo[1]=%q want Peito", list[1].GrupoMuscular)
	}
	if list[1].MusculoFoco != "Peitoral Maior" {
		t.Fatalf("foco[1]=%q want Peitoral Maior", list[1].MusculoFoco)
	}
}

func TestLoadCSVExercisesLegacyGrupoMuscularHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.csv")
	content := "codigo,nome,grupo muscular\n1,Remada Curvada,Costas\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	list, err := loadCSVExercises(path)
	if err != nil {
		t.Fatalf("loadCSVExercises: %v", err)
	}
	if len(list) != 1 || list[0].GrupoMuscular != "Costas" {
		t.Fatalf("got %+v", list)
	}
}

func TestLoadCSVExercisesMissingFileReturnsEmpty(t *testing.T) {
	list, err := loadCSVExercises(filepath.Join(t.TempDir(), "missing.csv"))
	if err != nil {
		t.Fatalf("err=%v want nil", err)
	}
	if list != nil {
		t.Fatalf("list=%v want nil", list)
	}
}
