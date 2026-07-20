package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func staffAppRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestBlocosDinamicosCorridaFlow(t *testing.T) {
	logger.Setup("development", false)
	t.Chdir(staffAppRoot(t))

	tempDir := t.TempDir()
	db, err := sqlite.Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		CorsOrigins:    []string{"*"},
		SecretKey:      "super-secret-key-change-me",
		AITrainingMode: "off",
	}
	authHeader := testAuthHeader(t, db, cfg)
	router := NewRouter(cfg, depsForTestDB(db))

	alunoRepo := sqlite.NewAlunoRepository(db)
	aluno := &domain.Aluno{Nome: "Runner Blocos", Idade: 30, Sexo: "M", Email: "blocos@example.com", Ativo: true}
	if err := alunoRepo.Create(t.Context(), aluno); err != nil {
		t.Fatalf("create aluno: %v", err)
	}

	var weekByWeekID int64
	var completaID int64
	var templateID int64

	t.Run("Generate blocos_completa", func(t *testing.T) {
		payload := fmt.Sprintf(`{
			"aluno_id": %d,
			"distancia_prova": "10K",
			"nivel": "intermediario",
			"pace_base": "05:30",
			"volume_semanal": 45.0,
			"dias_semana": [2, 4, 6],
			"data_inicio": "2026-07-22",
			"data_prova": "2026-10-10",
			"usar_blocos": true,
			"modo_geracao": "todas"
		}`, aluno.ID)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			Data domain.PeriodizacaoCorrida `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		completaID = resp.Data.ID
		if resp.Data.ModoGeracao != "blocos_completa" {
			t.Fatalf("expected modo blocos_completa, got %q", resp.Data.ModoGeracao)
		}
		var pd domain.PlanoDetalhado
		if err := json.Unmarshal([]byte(resp.Data.PlanoJSON), &pd); err != nil {
			t.Fatalf("parse plano: %v", err)
		}
		if len(pd.Semanas) < 4 {
			t.Fatalf("expected full plan weeks, got %d", len(pd.Semanas))
		}
		if len(pd.Semanas[0].Treinos) == 0 || len(pd.Semanas[0].Treinos[0].Blocos) == 0 {
			t.Fatal("expected non-empty blocos on first workout")
		}
	})

	t.Run("Generate semana_a_semana and append next week", func(t *testing.T) {
		payload := fmt.Sprintf(`{
			"aluno_id": %d,
			"distancia_prova": "10K",
			"nivel": "intermediario",
			"pace_base": "05:30",
			"volume_semanal": 45.0,
			"dias_semana": [1, 3, 5, 6],
			"data_inicio": "2026-07-22",
			"data_prova": "2026-10-10",
			"usar_blocos": true,
			"modo_geracao": "semana_a_semana"
		}`, aluno.ID)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			Data domain.PeriodizacaoCorrida `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		weekByWeekID = resp.Data.ID
		if resp.Data.ModoGeracao != "semana_a_semana" {
			t.Fatalf("expected semana_a_semana, got %q", resp.Data.ModoGeracao)
		}
		var pd domain.PlanoDetalhado
		_ = json.Unmarshal([]byte(resp.Data.PlanoJSON), &pd)
		if len(pd.Semanas) != 1 {
			t.Fatalf("expected 1 week, got %d", len(pd.Semanas))
		}

		req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/corrida/%d/gerar-proxima-semana", weekByWeekID), nil)
		req2.Header.Set("Authorization", authHeader)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w2.Code, w2.Body.String())
		}
		var nextResp struct {
			Data struct {
				SemanaNumero        int `json:"semana_numero"`
				TotalSemanasGeradas int `json:"total_semanas_geradas"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w2.Body.Bytes(), &nextResp); err != nil {
			t.Fatalf("decode next: %v", err)
		}
		if nextResp.Data.SemanaNumero != 2 || nextResp.Data.TotalSemanasGeradas != 2 {
			t.Fatalf("unexpected next week response: %+v", nextResp.Data)
		}
	})

	t.Run("gerar-proxima-semana rejects template mode", func(t *testing.T) {
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
			t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			Data domain.PeriodizacaoCorrida `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		templateID = resp.Data.ID

		req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/corrida/%d/gerar-proxima-semana", templateID), nil)
		req2.Header.Set("Authorization", authHeader)
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)
		if w2.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w2.Code)
		}
	})

	t.Run("GET and PUT day blocks with OCC", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/corrida/%d/semana/1/dia/2", completaID), nil)
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		var getResp struct {
			Data struct {
				Versao int               `json:"versao"`
				Treino domain.TreinoJSON `json:"treino"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
			t.Fatalf("decode get: %v", err)
		}
		if len(getResp.Data.Treino.Blocos) == 0 {
			t.Fatal("expected blocs on get")
		}

		savePayload := fmt.Sprintf(`{
			"versao": %d,
			"nome": "Intervalados Editados",
			"zona": "I",
			"blocos": [
				{"type":"atomic","intensity":"E","duration_min":10,"description":"Aquecimento"},
				{"type":"repeater","repeat":3,"content":[
					{"type":"atomic","intensity":"I","duration_min":3},
					{"type":"atomic","intensity":"E","duration_min":2}
				]},
				{"type":"atomic","intensity":"E","duration_min":8,"description":"Volta à calma"}
			]
		}`, getResp.Data.Versao)
		reqPut := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/corrida/%d/semana/1/dia/2/blocos", completaID), bytes.NewBufferString(savePayload))
		reqPut.Header.Set("Content-Type", "application/json")
		reqPut.Header.Set("Authorization", authHeader)
		wPut := httptest.NewRecorder()
		router.ServeHTTP(wPut, reqPut)
		if wPut.Code != http.StatusOK {
			t.Fatalf("expected 200 on save, got %d body=%s", wPut.Code, wPut.Body.String())
		}

		// Stale version must conflict
		reqConflict := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/corrida/%d/semana/1/dia/2/blocos", completaID), bytes.NewBufferString(savePayload))
		reqConflict.Header.Set("Content-Type", "application/json")
		reqConflict.Header.Set("Authorization", authHeader)
		wConflict := httptest.NewRecorder()
		router.ServeHTTP(wConflict, reqConflict)
		if wConflict.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d body=%s", wConflict.Code, wConflict.Body.String())
		}

		bad := `{"versao":999,"blocos":[{"type":"repeater","repeat":2,"content":[]}]}`
		reqBad := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/corrida/%d/semana/1/dia/2/blocos", completaID), bytes.NewBufferString(bad))
		reqBad.Header.Set("Content-Type", "application/json")
		reqBad.Header.Set("Authorization", authHeader)
		wBad := httptest.NewRecorder()
		router.ServeHTTP(wBad, reqBad)
		if wBad.Code != http.StatusBadRequest || !strings.Contains(wBad.Body.String(), "validation_errors") {
			t.Fatalf("expected validation 400, got %d body=%s", wBad.Code, wBad.Body.String())
		}
	})

	t.Run("historico-stats empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/alunos/%d/corrida/historico-stats", aluno.ID), nil)
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		// Plans exist so tem_historico may be true via plano_json; ensure JSON shape.
		if !strings.Contains(w.Body.String(), `"aluno_id"`) {
			t.Fatalf("unexpected body: %s", w.Body.String())
		}
	})

	t.Run("gerar-blocos AI off fallback", func(t *testing.T) {
		payload := `{
			"vdot": 45,
			"distancia_prova": "10K",
			"nivel": "intermediario",
			"dias_semana": 4
		}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar-blocos", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"fallback_used":true`) && !strings.Contains(w.Body.String(), `"fallback_used": true`) {
			t.Fatalf("expected fallback_used true, body=%s", w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"provider":"local"`) && !strings.Contains(w.Body.String(), `"provider": "local"`) {
			t.Fatalf("expected local provider, body=%s", w.Body.String())
		}
	})

	t.Run("gerar-blocos AI assistive local enricher", func(t *testing.T) {
		cfgAssistive := &config.Config{
			CorsOrigins:    []string{"*"},
			SecretKey:      "super-secret-key-change-me",
			AITrainingMode: "assistive",
		}
		routerAssistive := NewRouter(cfgAssistive, depsForTestDB(db))
		payload := `{
			"vdot": 45,
			"distancia_prova": "10K",
			"nivel": "intermediario",
			"dias_semana": 4,
			"objetivo": "performance"
		}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar-blocos", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		routerAssistive.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, `"fallback_used":false`) && !strings.Contains(body, `"fallback_used": false`) {
			t.Fatalf("expected fallback_used false for assistive local enricher, body=%s", body)
		}
		if !strings.Contains(body, "local-blocks-enricher") {
			t.Fatalf("expected local-blocks-enricher model, body=%s", body)
		}
		if !strings.Contains(body, "zona E") && !strings.Contains(body, "zona T") && !strings.Contains(body, "zona M") {
			t.Fatalf("expected enriched intensity notes, body=%s", body)
		}
	})

	t.Run("gerar-blocos AI assistive downgrades from limitacoes text", func(t *testing.T) {
		cfgAssistive := &config.Config{
			CorsOrigins:    []string{"*"},
			SecretKey:      "super-secret-key-change-me",
			AITrainingMode: "assistive",
		}
		routerAssistive := NewRouter(cfgAssistive, depsForTestDB(db))
		payload := `{
			"vdot": 50,
			"distancia_prova": "5K",
			"nivel": "avancado",
			"dias_semana": 4,
			"limitacoes": "histórico de arritmia e dor no peito"
		}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar-blocos", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		routerAssistive.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "risco cardiorrespirat") {
			t.Fatalf("expected cardio risk warning, body=%s", body)
		}
		if strings.Contains(body, `"intensity":"I"`) || strings.Contains(body, `"intensity": "I"`) ||
			strings.Contains(body, `"intensity":"R"`) || strings.Contains(body, `"intensity": "R"`) {
			t.Fatalf("expected I/R removed under cardio risk, body=%s", body)
		}
	})

	t.Run("gerar-blocos AI required without non-local provider returns 503", func(t *testing.T) {
		cfgRequired := &config.Config{
			CorsOrigins:    []string{"*"},
			SecretKey:      "super-secret-key-change-me",
			AITrainingMode: "required",
		}
		routerRequired := NewRouter(cfgRequired, depsForTestDB(db))
		payload := `{
			"vdot": 45,
			"distancia_prova": "10K",
			"nivel": "intermediario",
			"dias_semana": 4
		}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/corrida/gerar-blocos", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		routerRequired.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "Nenhum provedor de IA de blocos disponível") {
			t.Fatalf("expected provider unavailable message, body=%s", w.Body.String())
		}
	})

	t.Run("flat template plan remains readable", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/corrida/%d", templateID), nil)
		req.Header.Set("Authorization", authHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), `"modo_geracao":"template"`) && !strings.Contains(w.Body.String(), `"modo_geracao": "template"`) {
			t.Fatalf("expected template mode preserved, body=%s", w.Body.String())
		}
	})

	// Ensure templates file exists for docker packaging expectations.
	if _, err := os.Stat("data/json/templates_daniels_blocos.json"); err != nil {
		t.Fatalf("templates file missing: %v", err)
	}
}
