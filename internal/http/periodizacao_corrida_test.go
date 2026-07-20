package http

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func TestPeriodizacaoCorridaFlow(t *testing.T) {
	logger.Setup("development", false)

	// Setup temp database
	tempDir, err := os.MkdirTemp("", "http-periodizacao-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sqlite.Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		CorsOrigins: []string{"*"},
		SecretKey:   "super-secret-key-change-me",
	}

	authHeader := testAuthHeader(t, db, cfg)
	router := NewRouter(cfg, depsForTestDB(db))

	// Create test student (aluno)
	alunoRepo := sqlite.NewAlunoRepository(db)
	aluno := &domain.Aluno{
		Nome:  "Maria Souza",
		Idade: 28,
		Sexo:  "F",
		Email: "maria@example.com",
		Ativo: true,
	}
	if err := alunoRepo.Create(t.Context(), aluno); err != nil {
		t.Fatalf("failed to create test student: %v", err)
	}

	// 1. Validate Input Constraints (Should Fail)
	t.Run("Input Validation Failures", func(t *testing.T) {
		invalidPayloads := []struct {
			payload string
			errMsg  string
		}{
			{
				payload: fmt.Sprintf(`{"aluno_id": %d, "distancia_prova": "3K", "nivel": "intermediario", "pace_base": "05:30", "volume_semanal": 45, "dias_semana": [2, 4, 6], "data_prova": "2026-10-15"}`, aluno.ID),
				errMsg:  "Distância de prova inválida",
			},
			{
				payload: fmt.Sprintf(`{"aluno_id": %d, "distancia_prova": "10K", "nivel": "iniciante", "pace_base": "05:30", "volume_semanal": 45, "dias_semana": [2, 4, 6], "data_prova": "2026-10-15"}`, aluno.ID),
				errMsg:  "volume semanal de 45.0km inválido",
			},
			{
				payload: fmt.Sprintf(`{"aluno_id": %d, "distancia_prova": "10K", "nivel": "intermediario", "pace_base": "05:30", "volume_semanal": 45, "dias_semana": [2, 4, 4], "data_prova": "2026-10-15"}`, aluno.ID),
				errMsg:  "selecionados não podem conter duplicados",
			},
			{
				payload: fmt.Sprintf(`{"aluno_id": %d, "distancia_prova": "10K", "nivel": "intermediario", "pace_base": "05:30", "volume_semanal": 45, "dias_semana": [2, 4, 6], "data_prova": "2026-07-22", "data_inicio": "2026-07-20"}`, aluno.ID),
				errMsg:  "Duração do plano de corrida deve ser entre 4 e 24 semanas. Calculado: 1 semanas.",
			},
			{
				payload: fmt.Sprintf(`{"aluno_id": %d, "distancia_prova": "10K", "nivel": "intermediario", "pace_base": "05:30", "volume_semanal": 45, "dias_semana": [2, 4, 6], "data_prova": "2026-07-15", "data_inicio": "2026-07-20"}`, aluno.ID),
				errMsg:  "Data da prova deve ser estritamente posterior à data de início",
			},
		}

		for _, tc := range invalidPayloads {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar", bytes.NewBufferString(tc.payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", authHeader)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 bad request, got %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), tc.errMsg) {
				t.Errorf("expected error message to contain %q, got: %s", tc.errMsg, w.Body.String())
			}
		}
	})

	// 2. Generate and Verify Daniels Core Rules (Success)
	var planID int64
	t.Run("Generate Plan and Verify Core Rules", func(t *testing.T) {
		// Calculate a 12-week plan (duration >= 12 triggers Recovery weeks)
		// Monday start: 2026-07-20. Prova week Monday: 2026-10-05 (which is 11 weeks difference, so 12 weeks total)
		// We set data_inicio = 2026-07-22 (Wednesday) and data_prova = 2026-10-10 (Saturday) to verify Monday alignment
		payload := fmt.Sprintf(`{
			"aluno_id": %d,
			"distancia_prova": "10K",
			"nivel": "intermediario",
			"pace_base": "05:30",
			"volume_semanal": 45.0,
			"dias_semana": [2, 4, 6],
			"data_inicio": "2026-07-22",
			"data_prova": "2026-10-10"
		}`, aluno.ID)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 created, got %d. Body: %s", w.Code, w.Body.String())
		}

		var resp struct {
			Status string                     `json:"status"`
			Data   domain.PeriodizacaoCorrida `json:"data"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		planID = resp.Data.ID
		if planID <= 0 {
			t.Fatalf("expected plan ID to be populated, got %d", planID)
		}
		if resp.Data.DuracaoSemanas != 12 {
			t.Errorf("expected 12 weeks duration due to Monday alignment, got %d", resp.Data.DuracaoSemanas)
		}

		// Read detailed plan to check recovery week and clamping
		reqDetail := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/corrida/%d", planID), nil)
		reqDetail.Header.Set("Authorization", authHeader)
		wDetail := httptest.NewRecorder()
		router.ServeHTTP(wDetail, reqDetail)

		if wDetail.Code != http.StatusOK {
			t.Fatalf("failed to fetch plan details: %d", wDetail.Code)
		}

		var detailResp struct {
			Status string `json:"status"`
			Data   struct {
				domain.PeriodizacaoCorrida
				PlanoDetalhado domain.PlanoDetalhado `json:"plano_detalhado"`
			} `json:"data"`
		}
		if err := json.NewDecoder(wDetail.Body).Decode(&detailResp); err != nil {
			t.Fatalf("failed to decode detail response: %v", err)
		}

		pd := detailResp.Data.PlanoDetalhado

		// Verify Recovery Week (Week 4: modulo 4 == 0 and not Taper)
		var recoveryWeek domain.SemanaJSON
		foundRecovery := false
		for _, s := range pd.Semanas {
			if s.Numero == 4 {
				recoveryWeek = s
				foundRecovery = true
				break
			}
		}

		if !foundRecovery {
			t.Fatal("week 4 not found in generated plan")
		}

		// Recovery week volume should be 70% of 45km = 31.5km
		if recoveryWeek.VolumeTotal != 31.5 {
			t.Errorf("expected recovery week volume to be 31.5 (70%% of 45), got %.1f", recoveryWeek.VolumeTotal)
		}

		// All runs in recovery week must be E (Easy) zone
		for _, tr := range recoveryWeek.Treinos {
			if tr.Zona != "E" {
				t.Errorf("expected workout in recovery week to be E zone, got %s", tr.Zona)
			}
		}

		// Verify beginner clamp (VDOT must be clamped to 45.0)
		// We create a beginner with very fast pace (04:00/km which estimates VDOT ~49)
		begPayload := fmt.Sprintf(`{
			"aluno_id": %d,
			"distancia_prova": "10K",
			"nivel": "iniciante",
			"pace_base": "04:00",
			"volume_semanal": 25.0,
			"dias_semana": [2, 4, 6],
			"data_inicio": "2026-07-20",
			"data_prova": "2026-10-12"
		}`, aluno.ID)

		reqBeg := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar", bytes.NewBufferString(begPayload))
		reqBeg.Header.Set("Content-Type", "application/json")
		reqBeg.Header.Set("Authorization", authHeader)
		wBeg := httptest.NewRecorder()
		router.ServeHTTP(wBeg, reqBeg)

		if wBeg.Code != http.StatusCreated {
			t.Fatalf("failed to create beginner plan: %d. Body: %s", wBeg.Code, wBeg.Body.String())
		}

		var begResp struct {
			Data domain.PeriodizacaoCorrida `json:"data"`
		}
		_ = json.NewDecoder(wBeg.Body).Decode(&begResp)

		if begResp.Data.VDOT != 45.0 {
			t.Errorf("expected beginner VDOT to be clamped to 45.0, got %.1f", begResp.Data.VDOT)
		}
	})

	// 3. Plan Archiving Logic
	t.Run("Verify Active Plan Archiving", func(t *testing.T) {
		// Generate another plan for Maria (this should archive the previous active beginner plan)
		payload := fmt.Sprintf(`{
			"aluno_id": %d,
			"distancia_prova": "5K",
			"nivel": "intermediario",
			"pace_base": "05:30",
			"volume_semanal": 30.0,
			"dias_semana": [2, 4, 6],
			"data_inicio": "2026-07-20",
			"data_prova": "2026-10-12"
		}`, aluno.ID)

		reqGen := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar", bytes.NewBufferString(payload))
		reqGen.Header.Set("Content-Type", "application/json")
		reqGen.Header.Set("Authorization", authHeader)
		wGen := httptest.NewRecorder()
		router.ServeHTTP(wGen, reqGen)

		if wGen.Code != http.StatusCreated {
			t.Fatalf("failed to create new plan: %d", wGen.Code)
		}

		var genResp struct {
			Data domain.PeriodizacaoCorrida `json:"data"`
		}
		_ = json.NewDecoder(wGen.Body).Decode(&genResp)

		newPlanID := genResp.Data.ID

		if genResp.Data.FichaAnteriorID == nil {
			t.Fatal("expected ficha_anterior_id to be set to the previous plan's ID")
		}

		// Read the previous plan and check its status is archived
		prevPlanRepo := sqlite.NewPeriodizacaoCorridaRepository(db)
		prevPlan, err := prevPlanRepo.GetByID(t.Context(), *genResp.Data.FichaAnteriorID)
		if err != nil {
			t.Fatalf("failed to read previous plan: %v", err)
		}

		if prevPlan.Status != "arquivado" {
			t.Errorf("expected previous plan status to be 'arquivado', got %s", prevPlan.Status)
		}
		if prevPlan.DataArquivamento == nil || *prevPlan.DataArquivamento == "" {
			t.Error("expected previous plan data_arquivamento to be set")
		}

		// Cleanup: delete this newly generated plan so we keep planID as the main one for editing/public tests
		reqDel := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/corrida/%d", newPlanID), nil)
		reqDel.Header.Set("Authorization", authHeader)
		reqDel.Header.Set("X-Confirm-Hard-Delete", "CONFIRMAR")
		wDel := httptest.NewRecorder()
		router.ServeHTTP(wDel, reqDel)
		if wDel.Code != http.StatusOK {
			t.Fatalf("failed to delete temporary plan: %d", wDel.Code)
		}
	})

	// 4. Trainer Edit
	t.Run("Trainer Edit Workout", func(t *testing.T) {
		editPayload := `{
			"semana": 1,
			"dia": 4,
			"tipo": "Tempo Run Customizado",
			"distancia": 9.5,
			"zona": "T",
			"pace_alvo": "04:25",
			"descricao": "Treino ajustado pelo treinador"
		}`

		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/corrida/%d/editar-treino", planID), bytes.NewBufferString(editPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Retrieve plan and check edits
		reqDetail := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/corrida/%d", planID), nil)
		reqDetail.Header.Set("Authorization", authHeader)
		wDetail := httptest.NewRecorder()
		router.ServeHTTP(wDetail, reqDetail)

		var detailResp struct {
			Data struct {
				domain.PeriodizacaoCorrida
				PlanoDetalhado domain.PlanoDetalhado `json:"plano_detalhado"`
			} `json:"data"`
		}
		_ = json.NewDecoder(wDetail.Body).Decode(&detailResp)

		found := false
		for _, s := range detailResp.Data.PlanoDetalhado.Semanas {
			if s.Numero == 1 {
				for _, tr := range s.Treinos {
					if tr.Dia == 4 {
						if tr.Tipo != "Tempo Run Customizado" || tr.Distancia != 9.5 || tr.PaceAlvo != "04:25" || tr.Descricao != "Treino ajustado pelo treinador" {
							t.Errorf("workout fields not updated correctly: %+v", tr)
						}
						found = true
					}
				}
			}
		}
		if !found {
			t.Fatal("edited workout not found in weeks array")
		}
	})

	// 5. Public Links & Student Access
	var publicHash string
	t.Run("Public Link Management", func(t *testing.T) {
		// Generate Public Link
		reqLink := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/corrida/%d/gerar-link", planID), nil)
		reqLink.Header.Set("Authorization", authHeader)
		wLink := httptest.NewRecorder()
		router.ServeHTTP(wLink, reqLink)

		if wLink.Code != http.StatusCreated {
			t.Fatalf("failed to generate public link: %d. Body: %s", wLink.Code, wLink.Body.String())
		}

		var linkResp struct {
			Data struct {
				Hash string `json:"hash"`
			} `json:"data"`
		}
		if err := json.NewDecoder(wLink.Body).Decode(&linkResp); err != nil {
			t.Fatalf("failed to parse link response: %v", err)
		}

		publicHash = linkResp.Data.Hash
		if publicHash == "" {
			t.Fatal("hash should not be empty")
		}

		// Access public link anonymously
		reqPub := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/corrida/publica/%s", publicHash), nil)
		wPub := httptest.NewRecorder()
		router.ServeHTTP(wPub, reqPub)

		if wPub.Code != http.StatusOK {
			t.Fatalf("failed to access public link: %d", wPub.Code)
		}

		var pubResp struct {
			Data struct {
				AlunoNome      string                `json:"aluno_name"`
				PlanoDetalhado domain.PlanoDetalhado `json:"plano_detalhado"`
			} `json:"data"`
		}
		_ = json.NewDecoder(wPub.Body).Decode(&pubResp)

		// Check student can mark workout as completed anonymously
		concluirPayload := `{"semana": 1, "dia": 2, "concluido": true}`
		reqConc := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/corrida/publica/%s/concluir", publicHash), bytes.NewBufferString(concluirPayload))
		reqConc.Header.Set("Content-Type", "application/json")
		wConc := httptest.NewRecorder()
		router.ServeHTTP(wConc, reqConc)

		if wConc.Code != http.StatusOK {
			t.Fatalf("failed to complete workout: %d. Body: %s", wConc.Code, wConc.Body.String())
		}

		// Verify state update
		reqDetail := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/corrida/%d", planID), nil)
		reqDetail.Header.Set("Authorization", authHeader)
		wDetail := httptest.NewRecorder()
		router.ServeHTTP(wDetail, reqDetail)

		var detailResp struct {
			Data struct {
				domain.PeriodizacaoCorrida
				PlanoDetalhado domain.PlanoDetalhado `json:"plano_detalhado"`
			} `json:"data"`
		}
		_ = json.NewDecoder(wDetail.Body).Decode(&detailResp)

		workoutDone := false
		for _, s := range detailResp.Data.PlanoDetalhado.Semanas {
			if s.Numero == 1 {
				for _, tr := range s.Treinos {
					if tr.Dia == 2 {
						workoutDone = tr.Concluido
					}
				}
			}
		}
		if !workoutDone {
			t.Error("expected workout to be marked as completed")
		}
	})

	// 6. Optimistic Concurrency Control (OCC)
	t.Run("Optimistic Lock and Retry Concurrency", func(t *testing.T) {
		// Launch 5 concurrent requests updating different or same workouts
		// Because SQLite transaction handles retries, they should all complete or return gracefully.
		var wg sync.WaitGroup
		concurrencyCount := 8
		wg.Add(concurrencyCount)

		for i := 0; i < concurrencyCount; i++ {
			go func(day int) {
				defer wg.Done()

				concluirPayload := fmt.Sprintf(`{"semana": 1, "dia": %d, "concluido": true}`, day)
				req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/corrida/publica/%s/concluir", publicHash), bytes.NewBufferString(concluirPayload))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Status should be 200 OK (due to auto-retry on 409) or 400 Bad Request if workout day doesn't exist
				if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
					t.Errorf("unexpected status code for concurrent complete request: %d. Body: %s", w.Code, w.Body.String())
				}
			}(2 + (i%3)*2) // alternates days 2, 4, 6
		}

		wg.Wait()

		// Read the final plan version
		pcRepo := sqlite.NewPeriodizacaoCorridaRepository(db)
		finalPlan, err := pcRepo.GetByID(t.Context(), planID)
		if err != nil {
			t.Fatalf("failed to fetch final plan: %v", err)
		}

		// The version should have incremented multiple times due to multiple successful updates
		if finalPlan.Versao <= 1 {
			t.Errorf("expected plan version to be incremented, got %d", finalPlan.Versao)
		}
	})

	// 6b. Get Daily Workouts Calendar (GET /api/v1/alunos/{id}/corrida/treinos-dia)
	t.Run("Get Daily Workouts Calendar", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/alunos/%d/corrida/treinos-dia?mes=7&ano=2026", aluno.ID), nil)
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp["mes"].(float64) != 7 || resp["ano"].(float64) != 2026 {
			t.Errorf("unexpected month/year in response: %+v", resp)
		}

		treinosDia, ok := resp["treinos_dia"].([]any)
		if !ok || len(treinosDia) == 0 {
			t.Errorf("expected non-empty treinos_dia array, got response: %+v", resp)
		}

		for _, item := range treinosDia {
			treino, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("expected treino item to be an object, got %T", item)
			}
			data, _ := treino["data"].(string)
			if !strings.HasPrefix(data, "2026-07-") {
				t.Fatalf("expected only July 2026 workouts, got date %q in response %+v", data, resp)
			}
		}

		reqOut := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/alunos/%d/corrida/treinos-dia?mes=6&ano=2026", aluno.ID), nil)
		reqOut.Header.Set("Authorization", authHeader)
		wOut := httptest.NewRecorder()
		router.ServeHTTP(wOut, reqOut)

		if wOut.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for out-of-month filter, got %d. Body: %s", wOut.Code, wOut.Body.String())
		}

		var outResp map[string]any
		if err := json.Unmarshal(wOut.Body.Bytes(), &outResp); err != nil {
			t.Fatalf("failed to parse out-of-month response: %v", err)
		}
		outTreinos, ok := outResp["treinos_dia"].([]any)
		if !ok {
			t.Fatalf("expected treinos_dia array in out-of-month response, got: %+v", outResp)
		}
		if len(outTreinos) != 0 {
			t.Fatalf("expected no June 2026 workouts, got response: %+v", outResp)
		}
	})

	// 7. Hard Delete Security (DELETE with X-Confirm-Hard-Delete)
	t.Run("Hard Delete Security", func(t *testing.T) {
		// Try to delete without header (should fail)
		reqNoHead := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/corrida/%d", planID), nil)
		reqNoHead.Header.Set("Authorization", authHeader)
		wNoHead := httptest.NewRecorder()
		router.ServeHTTP(wNoHead, reqNoHead)

		if wNoHead.Code != http.StatusBadRequest {
			t.Errorf("expected 400 bad request without confirmation header, got %d", wNoHead.Code)
		}

		// Try to delete with confirmation header (should succeed)
		reqDel := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/corrida/%d", planID), nil)
		reqDel.Header.Set("Authorization", authHeader)
		reqDel.Header.Set("X-Confirm-Hard-Delete", "CONFIRMAR")
		wDel := httptest.NewRecorder()
		router.ServeHTTP(wDel, reqDel)

		if wDel.Code != http.StatusOK {
			t.Errorf("expected 200 OK when deleting with confirmation header, got %d. Body: %s", wDel.Code, wDel.Body.String())
		}

		// Check if it is really deleted
		_, err = sqlite.NewPeriodizacaoCorridaRepository(db).GetByID(t.Context(), planID)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected plan to be deleted, but repository returned error: %v", err)
		}
	})
}
