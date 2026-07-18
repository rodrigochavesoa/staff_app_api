package blocos

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"staff_app/internal/domain"
)

func templatesPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/corrida/blocos -> repo root data/json
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return filepath.Join(root, "data", "json", "templates_daniels_blocos.json")
}

func TestNormalizeModoGeracao(t *testing.T) {
	tests := []struct {
		usar   bool
		modo   string
		want   string
		wantErr bool
	}{
		{false, "", ModoTemplate, false},
		{false, "todas", ModoTemplate, false},
		{true, "", ModoBlocosCompleta, false},
		{true, "todas", ModoBlocosCompleta, false},
		{true, "semana_a_semana", ModoSemanaASemana, false},
		{false, "semana_a_semana", "", true},
		{true, "template", "", true},
	}
	for _, tc := range tests {
		got, err := NormalizeModoGeracao(tc.usar, tc.modo)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("usar=%v modo=%q expected error", tc.usar, tc.modo)
			}
			continue
		}
		if err != nil {
			t.Fatalf("usar=%v modo=%q unexpected err: %v", tc.usar, tc.modo, err)
		}
		if got != tc.want {
			t.Fatalf("usar=%v modo=%q got %q want %q", tc.usar, tc.modo, got, tc.want)
		}
	}
}

func TestGenerateWeekAndValidate(t *testing.T) {
	templates, err := LoadTemplates(templatesPath(t))
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	semana, err := GenerateWeek(templates, 2, 12, []int{2, 4, 6}, 45, "intermediario", "10K")
	if err != nil {
		t.Fatalf("GenerateWeek: %v", err)
	}
	if len(semana.Treinos) != 3 {
		t.Fatalf("expected 3 treinos, got %d", len(semana.Treinos))
	}
	for _, tr := range semana.Treinos {
		if len(tr.Blocos) == 0 {
			t.Fatalf("expected blocos on day %d", tr.Dia)
		}
		if tr.PaceAlvo == "" {
			t.Fatalf("expected pace on day %d", tr.Dia)
		}
		if errs := ValidateStructure(tr.Blocos); len(errs) > 0 {
			t.Fatalf("unexpected validation errors: %v", errs)
		}
	}
}

func TestValidateStructureRejectsEmptyRepeater(t *testing.T) {
	errs := ValidateStructure([]domain.BlocoCorrida{
		{Type: "repeater", Repeat: 3, Content: nil},
	})
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}
}

func TestGeneratePlanoSemanaASemana(t *testing.T) {
	templates, err := LoadTemplates(templatesPath(t))
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	pd, err := GeneratePlano(templates, 45, "10K", 10, 12, "intermediario", []int{1, 3, 5}, ModoSemanaASemana, nil)
	if err != nil {
		t.Fatalf("GeneratePlano: %v", err)
	}
	if len(pd.Semanas) != 1 {
		t.Fatalf("expected 1 week, got %d", len(pd.Semanas))
	}
	if pd.Tipo != "blocos_dinamicos" || pd.ModoGeracao != ModoSemanaASemana {
		t.Fatalf("unexpected metadata: tipo=%s modo=%s", pd.Tipo, pd.ModoGeracao)
	}
}

func TestFaseUsesRealDuration(t *testing.T) {
	fase, _ := FaseForWeek(8, 8)
	if fase != "Específica" {
		t.Fatalf("week 8/8 should be Específica, got %s", fase)
	}
	fase2, cats := FaseForWeek(2, 12)
	if fase2 != "Base" {
		t.Fatalf("week 2/12 should be Base, got %s", fase2)
	}
	if len(cats) == 0 {
		t.Fatal("expected categories")
	}
}

func TestComputeHistoricoStatsEmpty(t *testing.T) {
	stats := ComputeHistoricoStats(1, 30, nil, nil, time.Now().AddDate(0, 0, -30))
	if stats.TemHistorico {
		t.Fatal("expected no history")
	}
}

func TestDowngradeHardIntensities(t *testing.T) {
	in := []domain.BlocoCorrida{
		{Type: "atomic", Intensity: "I", DurationMin: 3},
		{Type: "repeater", Repeat: 2, Content: []domain.BlocoCorrida{
			{Type: "atomic", Intensity: "R", DurationMin: 1},
		}},
	}
	out := DowngradeHardIntensities(in)
	if out[0].Intensity != "E" {
		t.Fatalf("expected I->E, got %s", out[0].Intensity)
	}
	if out[1].Content[0].Intensity != "E" {
		t.Fatalf("expected nested R->E, got %s", out[1].Content[0].Intensity)
	}
}
