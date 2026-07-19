package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type classifyFixture struct {
	Name           string `json:"name"`
	Frequencia     int    `json:"frequencia"`
	Restricoes     string `json:"restricoes"`
	Observacoes    string `json:"observacoes"`
	WantComplexity string `json:"want_complexity"`
	WantScore      int    `json:"want_score"`
	HistoricoCount int    `json:"historico_count"`
	Anamnese       *struct {
		RiskScore     float64 `json:"risk_score"`
		Patologias    string  `json:"patologias"`
		LesoesAtuais  string  `json:"lesoes_atuais"`
		DoresCronicas string  `json:"dores_cronicas"`
	} `json:"anamnese"`
	SVED *struct {
		IesMedio float64 `json:"ies_medio"`
		Fichas   int     `json:"fichas"`
	} `json:"sved"`
}

func classificationInputFromFixture(fx classifyFixture) ClassificationInput {
	in := ClassificationInput{
		Frequencia:  fx.Frequencia,
		Restricoes:  fx.Restricoes,
		Observacoes: fx.Observacoes,
		Context:     &AthleteTrainingContext{},
	}
	if fx.Anamnese != nil {
		in.Context.Anamnese = &AnamneseTrainingHint{
			RiskScore:     fx.Anamnese.RiskScore,
			Patologias:    fx.Anamnese.Patologias,
			LesoesAtuais:  fx.Anamnese.LesoesAtuais,
			DoresCronicas: fx.Anamnese.DoresCronicas,
		}
	}
	if fx.SVED != nil {
		in.Context.SVED = &SVEDTrainingHint{
			IesMedio: fx.SVED.IesMedio,
			Fichas:   fx.SVED.Fichas,
		}
	}
	if fx.HistoricoCount > 0 {
		in.Context.Historico = make([]TrainingHistoryHint, fx.HistoricoCount)
	}
	return in
}

func runClassifierFixtures(t *testing.T, clf CaseComplexityClassifier, pattern string) {
	t.Helper()
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no fixtures matched %s", pattern)
	}
	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var fx classifyFixture
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatalf("unmarshal fixture: %v", err)
			}
			got := clf.Classify(t.Context(), classificationInputFromFixture(fx))
			if got.Complexity != fx.WantComplexity {
				t.Fatalf("%s: complexity=%q want %q (score=%d reasons=%v)",
					fx.Name, got.Complexity, fx.WantComplexity, got.Score, got.Reasons)
			}
			if got.Score != fx.WantScore {
				t.Fatalf("%s: score=%d want %d (reasons=%v)", fx.Name, got.Score, fx.WantScore, got.Reasons)
			}
		})
	}
}

func TestLegacyCaseComplexityClassifierParity(t *testing.T) {
	runClassifierFixtures(t, LegacyCaseComplexityClassifier{}, filepath.Join("testdata", "evidence", "classify_legacy_*.json"))
}

func TestDeterministicCaseComplexityClassifierSpec(t *testing.T) {
	runClassifierFixtures(t, DeterministicCaseComplexityClassifier{}, filepath.Join("testdata", "evidence", "classify_simple.json"))
	runClassifierFixtures(t, DeterministicCaseComplexityClassifier{}, filepath.Join("testdata", "evidence", "classify_moderate_restricoes.json"))
	runClassifierFixtures(t, DeterministicCaseComplexityClassifier{}, filepath.Join("testdata", "evidence", "classify_complex_clinical_stack.json"))
}
