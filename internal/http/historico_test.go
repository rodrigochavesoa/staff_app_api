package http

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func TestHistoricoFrequenciaBuscaFlow(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-historico-test-*")
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

	// Seed Alunos and Planos
	_, err = db.ExecContext(ctx, "INSERT INTO planos (id, nome, preco_default, ativo) VALUES (1, 'Plano Premium de Corrida', 399.00, 1)")
	if err != nil {
		t.Fatalf("failed to seed plans: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO alunos (id, nome, idade, sexo, email, ativo, plano_id, plano_ativo, turma)
		VALUES (1, 'Rodrigo Chaves', 30, 'M', 'rodrigo@example.com', 1, 1, 1, 'Turma A')
	`)
	if err != nil {
		t.Fatalf("failed to seed student 1: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO alunos (id, nome, idade, sexo, email, ativo, plano_id, plano_ativo, turma)
		VALUES (2, 'Maria Silva', 25, 'F', 'maria@example.com', 0, 1, 1, 'Turma B')
	`)
	if err != nil {
		t.Fatalf("failed to seed student 2: %v", err)
	}

	// Seed fichas_treino_web
	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_treino_web (id, aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal, duracao_treino, restricoes, feedback, turma, ficha_json)
		VALUES (10, 'Rodrigo Chaves', 30, 'M', 'Hipertrofia', 'Musculação', 'Avançado', 3, 60, '', '', 'Turma A', '{"exercicios":[]}')
	`)
	if err != nil {
		t.Fatalf("failed to seed ficha_treino_web: %v", err)
	}

	// Seed fichas_web public link
	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_web (id, hash, ficha_id, aluno_id, conteudo_json, criado_em, expira_em, acessos, ativo)
		VALUES (100, 'hash123', 10, 1, '{"frequencia_semanal":3}', '2026-07-01 12:00:00', '2026-08-31 12:00:00', 0, 1)
	`)
	if err != nil {
		t.Fatalf("failed to seed fichas_web: %v", err)
	}

	// Seed periodizacao_corrida
	// DataInicio = Monday, 2026-07-06.
	// Semana 1: Training 1 on Wednesday (Dia 3, i.e., 2026-07-08) is Concluido
	planoJSON := `{
		"vdot": 45.0,
		"distancia_prova": 5.0,
		"duracao_semanas": 4,
		"zonas": {},
		"semanas": [
			{
				"numero": 1,
				"fase": "Base",
				"volume_total": 20.0,
				"treinos": [
					{
						"dia": 3,
						"tipo": "Corrida Fácil",
						"distancia": 5.0,
						"zona": "E",
						"pace_alvo": "05:30",
						"descricao": "Fácil",
						"concluido": true
					}
				]
			}
		]
	}`

	_, err = db.ExecContext(ctx, `
		INSERT INTO periodizacao_corrida (id, aluno_id, data_inicio, duracao_semanas, modo, semana_atual, status, distancia_prova, nivel, vdot, pace_base, volume_semanal, dias_disponiveis, plano_json, modo_geracao, data_ultima_geracao, dias_semana_selecionados, versao)
		VALUES (20, 1, '2026-07-06', 4, '5K_iniciante', 1, 'ativo', 5.0, 'iniciante', 45.0, 330, 20.0, 3, ?, 'template', '2026-07-06 12:00:00', '[1, 3, 5]', 1)
	`, planoJSON)
	if err != nil {
		t.Fatalf("failed to seed periodizacao_corrida: %v", err)
	}

	// Seed historico_fichas
	_, err = db.ExecContext(ctx, `
		INSERT INTO historico_fichas (id, aluno_id, tipo_ficha, versao, status, data_arquivamento, ficha_json)
		VALUES (99, 1, 'musculacao', 1, 'arquivada', '2026-05-10 14:22:00', '{"exercicios":[{"nome":"Supino"}]}')
	`)
	if err != nil {
		t.Fatalf("failed to seed historico_fichas: %v", err)
	}

	cfg := &config.Config{CorsOrigins: []string{"*"}}
	router := NewRouter(cfg, db)
	authHeader := testAuthHeader(t, db, cfg)

	// ----------------------------------------------------
	// 1. Test Student Search
	// ----------------------------------------------------
	// Case A: Successful search
	req, _ := http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos/search?q=Rodrigo", nil)
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /alunos/search expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var searchResp struct {
		Alunos []domain.AlunoSearchResponse `json:"alunos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &searchResp); err != nil {
		t.Fatalf("failed to unmarshal search response: %v", err)
	}

	if len(searchResp.Alunos) != 1 || searchResp.Alunos[0].Nome != "Rodrigo Chaves" {
		t.Errorf("unexpected search results: %+v", searchResp.Alunos)
	}

	// Case B: Search too short
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos/search?q=R", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short search, got %d", w.Code)
	}

	// Case C: Filter inactive
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos/search?q=Maria&ativo=false", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /alunos/search expected 200, got %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &searchResp)
	if len(searchResp.Alunos) != 1 || searchResp.Alunos[0].Nome != "Maria Silva" {
		t.Errorf("unexpected inactive search results: %+v", searchResp.Alunos)
	}

	// ----------------------------------------------------
	// 2. Test Mark Treino (musculação)
	// ----------------------------------------------------
	// Case A: Reject Corrida Type
	markPayload := `{"ficha_id":10,"data_treino":"2026-07-16","tipo_ficha":"corrida"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/treinos/marcar", bytes.NewBufferString(markPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /treinos/marcar with corrida expected 400, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Case B: Public Anonymous Musculacao marking (by hash)
	// This will trigger auto-detection sequence since tipo_treino is empty.
	// Since no previous sessions exist, it will auto-detect "A".
	markPayloadValid := `{"ficha_id":10,"hash_ficha":"hash123","data_treino":"2026-07-16","tipo_ficha":"musculacao","observacao":"Senti leve dor no ombro"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/treinos/marcar", bytes.NewBufferString(markPayloadValid))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /treinos/marcar expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	var successResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &successResp)
	if !strings.Contains(successResp["message"].(string), "Treino A marcado com sucesso") {
		t.Errorf("expected auto-detected Treino A, got message: %q", successResp["message"])
	}

	// Case C: Auto-detection sequence check: Mark second workout.
	// Since the last workout was "A", it should auto-detect "B".
	markPayloadNext := `{"ficha_id":10,"hash_ficha":"hash123","data_treino":"2026-07-17","tipo_ficha":"musculacao"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/treinos/marcar", bytes.NewBufferString(markPayloadNext))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /treinos/marcar next expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &successResp)
	if !strings.Contains(successResp["message"].(string), "Treino B marcado com sucesso") {
		t.Errorf("expected auto-detected Treino B, got message: %q", successResp["message"])
	}

	// Case D: UPSERT/Idempotency check: Reposting the same workout (FichaID=10, Date=2026-07-16)
	// This should overwrite, not raise duplicate constraints error.
	markPayloadDup := `{"ficha_id":10,"hash_ficha":"hash123","data_treino":"2026-07-16","tipo_ficha":"musculacao","observacao":"Overwritten observation"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/treinos/marcar", bytes.NewBufferString(markPayloadDup))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /treinos/marcar UPSERT expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify that the observation is updated in DB
	var savedObs string
	err = db.QueryRowContext(ctx, "SELECT observacao FROM treinos_realizados WHERE ficha_id = 10 AND data_treino = '2026-07-16'").Scan(&savedObs)
	if err != nil {
		t.Fatalf("failed to query saved observation: %v", err)
	}
	if savedObs != "Overwritten observation" {
		t.Errorf("expected observation to be 'Overwritten observation', got %q", savedObs)
	}

	// ----------------------------------------------------
	// 3. Test Monthly Frequency & Dates Calculation (includes both weight lifting & running)
	// ----------------------------------------------------
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos/1/frequencia?mes=7&ano=2026", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /alunos/1/frequencia expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var freq domain.FrequenciaMensalResponse
	if err := json.Unmarshal(w.Body.Bytes(), &freq); err != nil {
		t.Fatalf("failed to decode frequencia response: %v", err)
	}

	// We should have 3 workouts in total:
	// - Musculação: 2026-07-16 (B)
	// - Musculação: 2026-07-17 (B)
	// - Corrida: 2026-07-08 (Wednesday of Week 1, calculated from DataInicio=2026-07-06)
	if len(freq.DiasFrequencia) != 3 {
		t.Errorf("expected 3 completed workouts in July 2026, got %d: %+v", len(freq.DiasFrequencia), freq.DiasFrequencia)
	}

	// Verify dates
	hasRun := false
	hasWeight16 := false
	hasWeight17 := false

	for _, d := range freq.DiasFrequencia {
		if d.Data == "2026-07-08" && d.TipoFicha == "corrida" {
			hasRun = true
		}
		if d.Data == "2026-07-16" && d.TipoFicha == "musculacao" {
			hasWeight16 = true
		}
		if d.Data == "2026-07-17" && d.TipoFicha == "musculacao" {
			hasWeight17 = true
		}
	}

	if !hasRun {
		t.Error("missing completed run on Wednesday, 2026-07-08")
	}
	if !hasWeight16 || !hasWeight17 {
		t.Error("missing completed weight training workouts on 2026-07-16 or 2026-07-17")
	}

	// Estatisticas checks
	// totalRealizados = 3
	// planned musculação = 3 * 4 = 12 (from link hash)
	// planned running = 1 (Wednesday of Week 1 in July)
	// totalPlanned = 13
	if freq.EstatisticasMensais.TotalRealizados != 3 {
		t.Errorf("expected 3 realizados, got %d", freq.EstatisticasMensais.TotalRealizados)
	}

	// ----------------------------------------------------
	// 4. Test Unmark Treino
	// ----------------------------------------------------
	unmarkPayload := `{"ficha_id":10,"hash_ficha":"hash123","data_treino":"2026-07-17"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/treinos/desmarcar", bytes.NewBufferString(unmarkPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /treinos/desmarcar expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Confirm it is gone
	var count int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM treinos_realizados WHERE ficha_id = 10 AND data_treino = '2026-07-17'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query treinos_realizados count: %v", err)
	}
	if count != 0 {
		t.Error("unmark did not delete workout record from DB")
	}

	// ----------------------------------------------------
	// 5. Test GET /api/v1/historico/fichas/{id}/detalhes
	// ----------------------------------------------------
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/historico/fichas/99/detalhes", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /historico/fichas/99/detalhes expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var detailResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("failed to decode historical details: %v", err)
	}

	if detailResp["tipo_ficha"] != "musculacao" {
		t.Errorf("expected musculacao tipo_ficha, got %q", detailResp["tipo_ficha"])
	}

	// FichaJSON must be a unescaped map, not a string
	fichaMap, ok := detailResp["ficha_json"].(map[string]any)
	if !ok {
		t.Fatalf("expected ficha_json to be a parsed JSON object, got: %T (%+v)", detailResp["ficha_json"], detailResp["ficha_json"])
	}

	exercicios := fichaMap["exercicios"].([]any)
	if len(exercicios) != 1 || exercicios[0].(map[string]any)["nome"] != "Supino" {
		t.Errorf("unexpected unmarshaled ficha_json: %+v", fichaMap)
	}

	// ----------------------------------------------------
	// 6. Test Authenticated Mark/Unmark without hash_ficha
	// ----------------------------------------------------
	// A. Authenticated Mark without hash_ficha (but with explicit AlunoID)
	authMarkPayload := `{"ficha_id":10,"data_treino":"2026-07-18","tipo_ficha":"musculacao","aluno_id":1,"observacao":"Marcado pelo treinador"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/treinos/marcar", bytes.NewBufferString(authMarkPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /treinos/marcar (auth, no hash) expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify in DB that it has NO hash_ficha and correct AlunoID
	var dbHash sql.NullString
	var dbAlunoID int64
	err = db.QueryRowContext(ctx, "SELECT hash_ficha, aluno_id FROM treinos_realizados WHERE ficha_id = 10 AND data_treino = '2026-07-18'").Scan(&dbHash, &dbAlunoID)
	if err != nil {
		t.Fatalf("failed to query treinos_realizados for auth mark: %v", err)
	}
	if dbHash.Valid {
		t.Errorf("expected hash_ficha to be NULL, got %q", dbHash.String)
	}
	if dbAlunoID != 1 {
		t.Errorf("expected aluno_id to be 1, got %d", dbAlunoID)
	}

	// B. Authenticated Unmark without hash_ficha
	authUnmarkPayload := `{"ficha_id":10,"data_treino":"2026-07-18"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/treinos/desmarcar", bytes.NewBufferString(authUnmarkPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /treinos/desmarcar (auth, no hash) expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Confirm it is deleted
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM treinos_realizados WHERE ficha_id = 10 AND data_treino = '2026-07-18'").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query count for auth unmark: %v", err)
	}
	if count != 0 {
		t.Error("authenticated unmark did not delete workout record from DB")
	}

	// C. Test public GET /api/v1/treinos/mes with valid hash
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/treinos/mes?hash_ficha=hash123&mes=7&ano=2026", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK for valid hash_ficha, got %d. Body: %s", w.Code, w.Body.String())
	}

	var mesResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &mesResp)
	if mesResp["mes"].(float64) != 7 || mesResp["ano"].(float64) != 2026 {
		t.Errorf("unexpected month/year in response: %+v", mesResp)
	}
	if mesResp["total_realizados"].(float64) < 1 {
		t.Errorf("expected at least 1 completed workout (running), got %+v", mesResp)
	}

	// D. Test public GET /api/v1/treinos/mes with invalid hash
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/treinos/mes?hash_ficha=nonexistent&mes=7&ano=2026", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for invalid hash_ficha, got %d", w.Code)
	}
}
