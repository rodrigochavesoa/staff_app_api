package services

import (
	"testing"
)

func TestParseInt(t *testing.T) {
	tests := []struct {
		val        any
		defaultVal int
		expected   int
	}{
		{3.0, 1, 3},
		{4, 1, 4},
		{int64(5), 1, 5},
		{"3", 1, 3},
		{"3-4", 1, 3},
		{"4x10", 1, 4},
		{"invalid", 2, 2},
		{nil, 2, 2},
	}

	for _, tt := range tests {
		res := ParseInt(tt.val, tt.defaultVal)
		if res != tt.expected {
			t.Errorf("ParseInt(%v, %d) = %d; expected %d", tt.val, tt.defaultVal, res, tt.expected)
		}
	}
}

func TestParseDescanso(t *testing.T) {
	tests := []struct {
		val      any
		expected int
	}{
		{60.0, 60},
		{90, 90},
		{"60s", 60},
		{"1 min", 60},
		{"2 mins", 120},
		{"60-90s", 60},
		{"invalid", 60},
		{nil, 60},
	}

	for _, tt := range tests {
		res := ParseDescanso(tt.val)
		if res != tt.expected {
			t.Errorf("ParseDescanso(%v) = %d; expected %d", tt.val, res, tt.expected)
		}
	}
}

func TestCalcularIES(t *testing.T) {
	res := CalcularIES(3, 10, "4010", 60, 2)
	expected := 50.0
	if res != expected {
		t.Errorf("CalcularIES(3, 10, 4010, 60, 2) = %f; expected %f", res, expected)
	}
}

func TestCalcularMetricasSVED(t *testing.T) {
	exercises := []ExercicioJSON{
		{
			Nome:       "Supino",
			Series:     3,
			Repeticoes: 10,
			Cadencia:   "4010",
			Descanso:   60,
			RIR:        2,
			Bloco:      "principal",
		},
		{
			Nome:       "Alongamento",
			Series:     3,
			Repeticoes: 10,
			Cadencia:   "4010",
			Descanso:   60,
			RIR:        2,
			Bloco:      "alongamento", // Should be ignored
		},
	}

	metrics := CalcularMetricasSVED(exercises)
	// Only main block considered:
	// tut_total_ex = 10 * 5 * 3 = 150s
	// tut_medio_s = 150
	// densidade = 150 / 60 = 2.5
	// ies = 2.5 * 8 * 2.5 = 50.0
	// volume = 150 * 0.85 = 127
	if metrics.TutMedioS != 150 {
		t.Errorf("TutMedioS = %d; expected 150", metrics.TutMedioS)
	}
	if metrics.DensidadeMedia != 2.5 {
		t.Errorf("DensidadeMedia = %f; expected 2.5", metrics.DensidadeMedia)
	}
	if metrics.IesMedioJoules != 50.0 {
		t.Errorf("IesMedioJoules = %f; expected 50.0", metrics.IesMedioJoules)
	}
	if metrics.VolumeSved != 128 {
		t.Errorf("VolumeSved = %d; expected 128", metrics.VolumeSved)
	}
}

func TestSugerirProgressaoSVED(t *testing.T) {
	hist := []SVEDHistoricoItem{
		{
			RIR:         0,
			Reps:        10,
			Series:      3,
			Densidade:   2.5,
			RestSeconds: 60,
			Cadencia:    "4010",
		},
		{
			RIR:         2,
			Reps:        10,
			Series:      3,
			Densidade:   2.5,
			RestSeconds: 60,
			Cadencia:    "4010",
		},
	}

	sug := SugerirProgressaoSVED("Supino", hist)
	expectedType := "aumentar_reps"
	if sug["tipo"] != expectedType {
		t.Errorf("Sugestion type = %v; expected %s", sug["tipo"], expectedType)
	}

	params := sug["parametros"].(map[string]any)
	if params["reps"] != 12 || params["rir"] != 2 {
		t.Errorf("Obtained parameters %+v; expected reps=12, rir=2", params)
	}
}
