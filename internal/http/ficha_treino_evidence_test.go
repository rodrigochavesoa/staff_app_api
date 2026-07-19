package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"staff_app/internal/config"
	"staff_app/internal/platform/logger"
	"staff_app/internal/services"
	"staff_app/internal/sqlite"
)

type stubTrainingProvider struct {
	name string
	raw  string
}

func (s stubTrainingProvider) Name() string  { return s.name }
func (s stubTrainingProvider) Model() string { return "stub-test" }
func (s stubTrainingProvider) Generate(context.Context, *services.GenerationRequest) (string, error) {
	return s.raw, nil
}

func TestGerarPeriodizadaSimpleCaseEvidenceMetadata(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "http-ficha-evidence-test-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sqlite.Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	ctx := t.Context()
	cfg := &config.Config{
		SecretKey:      "super-secret-key-for-test-purposes",
		CorsOrigins:    []string{"*"},
		AITrainingMode: services.AITrainingModeAssistive,
	}
	router := NewRouter(cfg, db, WithTrainingProviders(stubTrainingProvider{
		name: "gemini",
		raw:  periodizedTrainingJSON("Supino Maquina"),
	}))

	_, err = db.ExecContext(ctx, "INSERT INTO planos (id, nome, preco_default, ativo) VALUES (1, 'Plano Premium', 299.00, 1)")
	if err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO alunos (id, nome, idade, sexo, email, ativo, plano_id, plano_ativo, turma)
		VALUES (1, 'Test Student', 25, 'M', 'test@example.com', 1, 1, 1, 'Turma Alpha')
	`)
	if err != nil {
		t.Fatalf("seed aluno: %v", err)
	}

	authHeader := testAuthHeader(t, db, cfg)
	payload := GerarFichaPeriodizadaRequest{
		AlunoID:    1,
		Frequencia: 3,
		Objetivo:   "Hipertrofia",
		Nivel:      "Intermediário",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/fichas/gerar-periodizada", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := resp["data"].(map[string]any)
	aiMetadata := data["ai_metadata"].(map[string]any)

	if aiMetadata["complexity"].(string) != "simples" {
		t.Fatalf("complexity=%v want simples", aiMetadata["complexity"])
	}
	if int(aiMetadata["evidence_count"].(float64)) != 0 {
		t.Fatalf("evidence_count=%v want 0", aiMetadata["evidence_count"])
	}
	if aiMetadata["context_used"].(bool) != true {
		t.Fatalf("context_used=%v want true", aiMetadata["context_used"])
	}
	if aiMetadata["ai_used"].(bool) != true {
		t.Fatalf("ai_used=%v want true with stub provider", aiMetadata["ai_used"])
	}
	reasons, ok := aiMetadata["evidence_reasons"].([]any)
	if !ok || len(reasons) == 0 {
		t.Fatalf("evidence_reasons missing or empty: %v", aiMetadata["evidence_reasons"])
	}
	if reasons[0].(string) != "complexidade_simples: busca não acionada" {
		t.Fatalf("evidence_reasons[0]=%v", reasons[0])
	}
	if _, ok := aiMetadata["confidence_score"].(float64); !ok {
		t.Fatalf("confidence_score missing: %v", aiMetadata["confidence_score"])
	}
	if aiMetadata["evidence_fallback_used"].(bool) != false {
		t.Fatalf("evidence_fallback_used=%v want false", aiMetadata["evidence_fallback_used"])
	}
}

func periodizedTrainingJSON(exerciseName string) string {
	return `{
		"tipo": "periodizada",
		"frequencia": 3,
		"objetivo": "Hipertrofia",
		"nivel": "Intermediário",
		"treinos": [
			{
				"letra": "A",
				"nome": "A - Peito",
				"exercicios": [
					{
						"nome": "` + exerciseName + `",
						"grupo_muscular": "Peitoral",
						"series": 3,
						"repeticoes": "10-12",
						"descanso": 60,
						"cadencia": "4010"
					}
				]
			}
		]
	}`
}
