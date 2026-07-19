package services

import (
	"strings"
	"testing"

	"staff_app/internal/corrida/blocos"
	"staff_app/internal/domain"
)

func TestLocalBlocksAIProviderEnrichesNotes(t *testing.T) {
	p := LocalBlocksAIProvider{}
	if !p.Available() {
		t.Fatal("local blocks provider must be available")
	}
	res, err := p.Enrich(t.Context(), &BlocksEnrichRequest{
		Days: []blocos.PreviewDay{{
			Dia:  1,
			Nome: "Easy",
			Blocos: []domain.BlocoCorrida{
				{Type: "atomic", Intensity: "E", DurationMin: 20},
			},
		}},
		VDOT:     45,
		Objetivo: "performance",
	})
	if err != nil {
		t.Fatalf("Enrich() error: %v", err)
	}
	if len(res.Days) != 1 || !strings.Contains(res.Days[0].Blocos[0].Notas, "zona E") {
		t.Fatalf("expected E-zone note, got %+v", res.Days)
	}
}

func TestLocalBlocksAIProviderDowngradesHardIntensities(t *testing.T) {
	p := LocalBlocksAIProvider{}
	res, err := p.Enrich(t.Context(), &BlocksEnrichRequest{
		Days: []blocos.PreviewDay{{
			Dia: 1,
			Blocos: []domain.BlocoCorrida{
				{Type: "atomic", Intensity: "I", DurationMin: 3},
				{Type: "atomic", Intensity: "R", DurationMin: 1},
			},
		}},
		VDOT:           45,
		HighCardioRisk: true,
	})
	if err != nil {
		t.Fatalf("Enrich() error: %v", err)
	}
	if err := ValidateBlocksSafety(res.Days, true); err != nil {
		t.Fatalf("expected safe output, got %v", err)
	}
	for _, b := range res.Days[0].Blocos {
		if b.Intensity == "I" || b.Intensity == "R" {
			t.Fatalf("expected I/R downgraded, got %s", b.Intensity)
		}
	}
}

func TestValidateBlocksSafetyRejectsHardIntervals(t *testing.T) {
	days := []blocos.PreviewDay{{
		Blocos: []domain.BlocoCorrida{{Type: "atomic", Intensity: "I", DurationMin: 2}},
	}}
	if err := ValidateBlocksSafety(days, true); err == nil {
		t.Fatal("expected safety rejection for I under high risk")
	}
	if err := ValidateBlocksSafety(days, false); err != nil {
		t.Fatalf("unexpected rejection without high risk: %v", err)
	}
}

func TestHighCardioRiskFromText(t *testing.T) {
	if !HighCardioRiskFromText("histórico de arritmia e dispneia") {
		t.Fatal("expected cardio risk from limitations text")
	}
	if HighCardioRiskFromText("joelho direito sensível") {
		t.Fatal("did not expect cardio risk for knee text")
	}
}
