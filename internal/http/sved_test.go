package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"staff_app/internal/config"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func TestSVEDFlow(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-sved-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sqlite.Connect(dbPath)
	if err != nil {
		t.Fatalf("failed to init database: %v", err)
	}
	defer db.Close()

	ctx := t.Context()
	cfg := &config.Config{
		SecretKey:   "super-secret-key-for-test-purposes",
		CorsOrigins: []string{"*"},
	}

	router := NewRouter(cfg, db)

	// Seed Alunos and Planos
	_, err = db.ExecContext(ctx, "INSERT INTO planos (id, nome, preco_default, ativo) VALUES (1, 'Plano Premium', 299.00, 1)")
	if err != nil {
		t.Fatalf("failed to seed plans: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO alunos (id, nome, idade, sexo, email, ativo, plano_id, plano_ativo, turma)
		VALUES (1, 'Test Student', 25, 'M', 'test@example.com', 1, 1, 1, 'Turma Alpha')
	`)
	if err != nil {
		t.Fatalf("failed to seed student: %v", err)
	}

	authHeader := testAuthHeader(t, db, cfg)

	// 1. Test POST /api/v1/sved/calcular
	t.Run("Calcular SVED Metrics", func(t *testing.T) {
		payload := map[string]any{
			"reps":     10,
			"cadencia": "4010",
			"descanso": "60s",
			"rir":      2,
			"series":   3,
		}

		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/sved/calcular", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp["success"] != true {
			t.Errorf("expected success=true, got %v", resp["success"])
		}

		tut := resp["tut_total"].(float64)
		if tut != 150 {
			t.Errorf("expected tut_total=150, got %f", tut)
		}

		dens := resp["densidade"].(float64)
		if dens != 2.5 {
			t.Errorf("expected densidade=2.5, got %f", dens)
		}

		ies := resp["ies"].(float64)
		// unified math: densidade (2.5) * (10 - rir (2)) * 2.5 = 50.0
		if ies != 50.0 {
			t.Errorf("expected ies=50.0, got %f", ies)
		}
	})

	// 2. Seed sheets for history and suggestion testing
	t.Run("SVED History and Dashboard", func(t *testing.T) {
		// Insert first older sheet
		_, err = db.ExecContext(ctx, `
			INSERT INTO fichas_treino_web (
				id, aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal,
				duracao_treino, restricoes, feedback, turma, lista_exercicios, data_criacao,
				ficha_json, tipo_ficha, num_treinos, versao, ies_score, volume_sved, densidade, tut_total
			) VALUES (
				10, 'Test Student', 25, 'M', 'Hipertrofia', 'Musculação', 'Intermediário', 3,
				60, 'Nenhuma', 'Feedback', 'Turma Alpha', 'exercicios_com_grupos', '2026-07-16 10:00:00',
				'{"exercicios":[{"nome":"Supino Reto","grupo_muscular":"Peito","series":3,"repeticoes":"10","cadencia":"4010","descanso":60,"rir":2,"bloco":"principal"}]}',
				'manual', 1, 1, 50.0, 128, 2.5, 150
			)
		`)
		if err != nil {
			t.Fatalf("failed to insert sheet 10: %v", err)
		}

		// Insert second newer sheet (with RIR=0 to test progression suggestion)
		_, err = db.ExecContext(ctx, `
			INSERT INTO fichas_treino_web (
				id, aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal,
				duracao_treino, restricoes, feedback, turma, lista_exercicios, data_criacao,
				ficha_json, tipo_ficha, num_treinos, versao, ies_score, volume_sved, densidade, tut_total
			) VALUES (
				11, 'Test Student', 25, 'M', 'Hipertrofia', 'Musculação', 'Intermediário', 3,
				60, 'Nenhuma', 'Feedback', 'Turma Alpha', 'exercicios_com_grupos', '2026-07-17 10:00:00',
				'{"exercicios":[{"nome":"Supino Reto","grupo_muscular":"Peito","series":3,"repeticoes":"10","cadencia":"4010","descanso":60,"rir":0,"bloco":"principal"}]}',
				'manual', 1, 1, 62.5, 128, 2.5, 150
			)
		`)
		if err != nil {
			t.Fatalf("failed to insert sheet 11: %v", err)
		}

		// Test GET /api/v1/sved/historico/{aluno_id}/{exercicio_nome}
		reqHist, _ := http.NewRequest(http.MethodGet, "/api/v1/sved/historico/1/Supino Reto", nil)
		reqHist.Header.Set("Authorization", authHeader)
		rrHist := httptest.NewRecorder()
		router.ServeHTTP(rrHist, reqHist)

		if rrHist.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", rrHist.Code, rrHist.Body.String())
		}

		var respHist map[string]any
		_ = json.Unmarshal(rrHist.Body.Bytes(), &respHist)
		histList := respHist["historico"].([]any)
		if len(histList) != 2 {
			t.Errorf("expected 2 history entries, got %d", len(histList))
		}

		// Test GET /api/v1/sved/sugestao/{ficha_id}/{exercicio_nome}
		// Ficha 11 is the most recent (2026-07-17), so suggestion should evaluate hist[0] (from Ficha 11) which has RIR=0
		reqSug, _ := http.NewRequest(http.MethodGet, "/api/v1/sved/sugestao/11/Supino Reto", nil)
		reqSug.Header.Set("Authorization", authHeader)
		rrSug := httptest.NewRecorder()
		router.ServeHTTP(rrSug, reqSug)

		if rrSug.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", rrSug.Code, rrSug.Body.String())
		}

		var respSug map[string]any
		_ = json.Unmarshal(rrSug.Body.Bytes(), &respSug)
		sugVal := respSug["sugestao"].(map[string]any)
		if sugVal["tipo"] != "aumentar_reps" {
			t.Errorf("expected suggestion type 'aumentar_reps' due to RIR 0, got %v", sugVal["tipo"])
		}

		// Test GET /api/v1/sved/dashboard/{aluno_id}
		reqDash, _ := http.NewRequest(http.MethodGet, "/api/v1/sved/dashboard/1", nil)
		reqDash.Header.Set("Authorization", authHeader)
		rrDash := httptest.NewRecorder()
		router.ServeHTTP(rrDash, reqDash)

		if rrDash.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", rrDash.Code, rrDash.Body.String())
		}

		var respDash map[string]any
		_ = json.Unmarshal(rrDash.Body.Bytes(), &respDash)
		statsGerais := respDash["stats_gerais"].(map[string]any)
		// Average of 50.0 and 62.5 is 56.25 -> rounded to 56.3
		if statsGerais["ies_medio_geral"] != 56.3 {
			t.Errorf("expected average IES 56.3, got %v", statsGerais["ies_medio_geral"])
		}
	})

	// 3. Test scenario with periodized multi-workout sheet
	t.Run("SVED Periodized Sheet Processing", func(t *testing.T) {
		_, err = db.ExecContext(ctx, `
			INSERT INTO fichas_treino_web (
				id, aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal,
				duracao_treino, restricoes, feedback, turma, lista_exercicios, data_criacao,
				ficha_json, tipo_ficha, num_treinos, versao, ies_score, volume_sved, densidade, tut_total
			) VALUES (
				12, 'Test Student', 25, 'M', 'Hipertrofia', 'Musculação', 'Intermediário', 3,
				60, 'Nenhuma', 'Feedback', 'Turma Alpha', 'exercicios_com_grupos', '2026-07-18 10:00:00',
				'{"tipo":"periodizada","treinos":[{"letra":"A","nome":"Treino A","exercicios":[{"nome":"Supino Reto","grupo_muscular":"Peito","series":4,"repeticoes":"10","cadencia":"4010","descanso":60,"rir":2,"bloco":"principal"},{"nome":"Remada Baixa","grupo_muscular":"Costas"}]}]}',
				'periodizada', 1, 1, 66.7, 170, 3.33, 200
			)
		`)
		if err != nil {
			t.Fatalf("failed to insert periodized sheet: %v", err)
		}

		// Now it should return 3 history items (including the one from the periodized sheet)
		reqHist, _ := http.NewRequest(http.MethodGet, "/api/v1/sved/historico/1/Supino Reto", nil)
		reqHist.Header.Set("Authorization", authHeader)
		rrHist := httptest.NewRecorder()
		router.ServeHTTP(rrHist, reqHist)

		if rrHist.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d. Body: %s", rrHist.Code, rrHist.Body.String())
		}

		var respHist map[string]any
		_ = json.Unmarshal(rrHist.Body.Bytes(), &respHist)
		histList := respHist["historico"].([]any)
		if len(histList) != 3 {
			t.Errorf("expected 3 history entries, got %d", len(histList))
		}

		// Verify the periodized one: series=4, reps=10, rest=60, rir=2
		// tutTotalEx = 4 * 10 * 5 = 200s
		// densidade = 200 / 60 = 3.3333
		// iesScore = 3.3333 * 8 * 2.5 = 66.6667 -> 66.7
		item := histList[0].(map[string]any) // index 0 is latest (2026-07-18)
		if item["series"].(float64) != 4 {
			t.Errorf("expected series=4, got %f", item["series"].(float64))
		}
		if item["ies_score"].(float64) != 66.7 {
			t.Errorf("expected ies_score=66.7, got %f", item["ies_score"].(float64))
		}

		reqBatch, _ := http.NewRequest(http.MethodGet, "/api/v1/sved/sugestoes/12", nil)
		reqBatch.Header.Set("Authorization", authHeader)
		rrBatch := httptest.NewRecorder()
		router.ServeHTTP(rrBatch, reqBatch)

		if rrBatch.Code != http.StatusOK {
			t.Fatalf("expected batch status 200, got %d. Body: %s", rrBatch.Code, rrBatch.Body.String())
		}

		var respBatch map[string]any
		if err := json.Unmarshal(rrBatch.Body.Bytes(), &respBatch); err != nil {
			t.Fatalf("failed to parse batch response: %v", err)
		}
		if respBatch["success"] != true {
			t.Fatalf("expected batch success=true, got %v", respBatch["success"])
		}
		if respBatch["total_exercicios"].(float64) != 2 {
			t.Fatalf("expected 2 batch suggestions, got %v", respBatch["total_exercicios"])
		}

		sugestoes := respBatch["sugestoes"].([]any)
		first := sugestoes[0].(map[string]any)
		if first["exercicio"] != "Supino Reto" {
			t.Fatalf("expected first batch exercise Supino Reto, got %v", first["exercicio"])
		}
		if first["ies_score"].(float64) != 66.7 {
			t.Fatalf("expected batch ies_score=66.7, got %v", first["ies_score"])
		}
		if first["historico_count"].(float64) != 3 {
			t.Fatalf("expected batch historico_count=3, got %v", first["historico_count"])
		}

		second := sugestoes[1].(map[string]any)
		if second["exercicio"] != "Remada Baixa" {
			t.Fatalf("expected second batch exercise Remada Baixa, got %v", second["exercicio"])
		}
		warnings := second["warnings"].([]any)
		if len(warnings) == 0 {
			t.Fatal("expected warnings for exercise with missing SVED fields")
		}
	})
}
