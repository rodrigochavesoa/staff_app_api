package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func TestExerciciosReabilitacaoFlow(t *testing.T) {
	logger.Setup("development", false)

	// Setup temp database
	tempDir, err := os.MkdirTemp("", "http-exercicios-test-*")
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
	router := NewRouter(cfg, db)

	// 1. Create a therapeutic exercise (POST /api/v1/exercicios/personalizados)
	exTerapPayload := `{
		"nome": "Agachamento Unilateral Terapêutico",
		"categoria": "terapeutico",
		"descricao_terapeutica": "Agachamento unilateral com apoio",
		"indicacoes": "Fortalecimento patelar",
		"contraindicacoes": "Dor aguda",
		"grupo_muscular": "Pernas",
		"musculo_foco": "Quadríceps",
		"tipo_exercicio": "Mobilidade",
		"intensidade": "Leve",
		"nivel_prioridade": 1,
		"fonte_cientifica": "Artigo Clinico 2024",
		"url_secundaria": "https://youtube.com/watch?v=1",
		"notas_profissional": "3 séries de 10"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/exercicios/personalizados", bytes.NewBufferString(exTerapPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var createdTerap domain.ExercicioReabilitacao
	if err := json.NewDecoder(w.Body).Decode(&createdTerap); err != nil {
		t.Fatalf("failed to decode created exercise: %v", err)
	}

	if createdTerap.Codigo != 5000 {
		t.Errorf("expected generated code 5000, got %d", createdTerap.Codigo)
	}

	// 2. Create a normal exercise
	exNormalPayload := `{
		"nome": "Supino Reto com Halteres",
		"categoria": "normal",
		"descricao_terapeutica": "Supino reto com halteres no banco",
		"grupo_muscular": "Peito",
		"tipo_exercicio": "Força"
	}`

	req = httptest.NewRequest(http.MethodPost, "/api/v1/exercicios/personalizados", bytes.NewBufferString(exNormalPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 created, got %d", w.Code)
	}

	var createdNormal domain.ExercicioReabilitacao
	if err := json.NewDecoder(w.Body).Decode(&createdNormal); err != nil {
		t.Fatalf("failed to decode created normal exercise: %v", err)
	}

	if createdNormal.Codigo != 6000 {
		t.Errorf("expected generated code 6000, got %d", createdNormal.Codigo)
	}

	// 3. Duplicate name check (should fail)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/exercicios/personalizados", bytes.NewBufferString(exNormalPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 bad request for duplicate name, got %d", w.Code)
	}

	// 4. List database exercises (GET /api/v1/exercicios/personalizados)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/exercicios/personalizados?categoria=terapeutico", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 ok, got %d", w.Code)
	}

	var list []domain.ExercicioReabilitacao
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}

	if len(list) != 1 || list[0].Codigo != 5000 {
		t.Errorf("expected 1 therapeutic exercise (code 5000), got %d exercises", len(list))
	}

	// 5. Get detail of exercise (GET /api/v1/exercicios/personalizados/{codigo})
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/exercicios/personalizados/%d", createdTerap.Codigo), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 ok, got %d", w.Code)
	}

	var detail domain.ExercicioReabilitacao
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("failed to decode detail response: %v", err)
	}

	if detail.Nome != createdTerap.Nome {
		t.Errorf("expected name %s, got %s", createdTerap.Nome, detail.Nome)
	}

	// 6. Edit exercise (PUT /api/v1/exercicios/personalizados/{codigo})
	updatePayload := `{
		"nome": "Agachamento Unilateral Modificado",
		"categoria": "terapeutico",
		"descricao_terapeutica": "Descrição atualizada",
		"grupo_muscular": "Pernas"
	}`
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/exercicios/personalizados/%d", createdTerap.Codigo), bytes.NewBufferString(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 ok, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify changed name
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/exercicios/personalizados/%d", createdTerap.Codigo), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	_ = json.NewDecoder(w.Body).Decode(&detail)
	if detail.Nome != "Agachamento Unilateral Modificado" {
		t.Errorf("expected updated name, got %s", detail.Nome)
	}

	// 7. Test deactivation and activation
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/exercicios/personalizados/%d/desativar", createdTerap.Codigo), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("failed to deactivate: %d", w.Code)
	}

	// Verify deactivated
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/exercicios/personalizados/%d", createdTerap.Codigo), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	_ = json.NewDecoder(w.Body).Decode(&detail)
	if detail.Status != "inativo" {
		t.Errorf("expected status 'inativo', got %s", detail.Status)
	}

	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/exercicios/personalizados/%d/ativar", createdTerap.Codigo), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("failed to activate: %d", w.Code)
	}

	// 8. Unique lists and stats
	req = httptest.NewRequest(http.MethodGet, "/api/v1/exercicios/grupos", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var groups []string
	_ = json.NewDecoder(w.Body).Decode(&groups)
	if len(groups) != 2 { // "Pernas" and "Peito"
		t.Errorf("expected 2 groups, got %d (%v)", len(groups), groups)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/exercicios/personalizados/estatisticas", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var stats map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&stats)
	if int(stats["total"].(float64)) != 2 {
		t.Errorf("expected total 2 in stats, got %v", stats["total"])
	}

	// 9. Hard Delete safety check (X-Confirm-Hard-Delete)
	// Delete without header (should fail)
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/exercicios/personalizados/%d", createdNormal.Codigo), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 bad request for hard delete without confirmation, got %d", w.Code)
	}

	// Delete with header (should succeed)
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/exercicios/personalizados/%d", createdNormal.Codigo), nil)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Confirm-Hard-Delete", "CONFIRMAR")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 ok for hard delete with confirmation, got %d", w.Code)
	}

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/exercicios/personalizados/%d", createdNormal.Codigo), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 not found for deleted exercise, got %d", w.Code)
	}
}

func TestExerciciosBibliotecaUnificada(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "http-biblioteca-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	t.Chdir(tempDir)

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
	router := NewRouter(cfg, db)

	// Create a database exercise
	exPayload := `{
		"nome": "Corrida Estacionária Clínica",
		"categoria": "terapeutico",
		"grupo_muscular": "Cardio",
		"tipo_exercicio": "Aeróbico"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/exercicios/personalizados", bytes.NewBufferString(exPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to seed exercise: %d", w.Code)
	}

	// 1. Query library without CSV file (should return only DB exercise)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/exercicios/biblioteca", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 ok, got %d", w.Code)
	}

	var respNoCSV struct {
		Exercicios        []combinedExercise `json:"exercicios"`
		Total             int                `json:"total"`
		TotalNormais      int                `json:"total_normais"`
		TotalTerapeuticos int                `json:"total_terapeuticos"`
	}
	if err := json.NewDecoder(w.Body).Decode(&respNoCSV); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if respNoCSV.Total != 1 || respNoCSV.TotalTerapeuticos != 1 {
		t.Errorf("expected 1 therapeutic exercise, got %d", respNoCSV.Total)
	}

	// 2. Setup mock CSV directory and file
	csvDir := filepath.Join("data", "csv")
	err = os.MkdirAll(csvDir, 0755)
	if err != nil {
		t.Fatalf("failed to create data/csv dir: %v", err)
	}

	csvContent := "Código,Nome do Exercício,Grupo Muscular\n1234,Abdominal Supra,Abdômen\n5678,Rosca Direta,Braços\n"
	err = os.WriteFile(filepath.Join(csvDir, "exercicios_com_grupos.csv"), []byte(csvContent), 0644)
	if err != nil {
		t.Fatalf("failed to write mock CSV file: %v", err)
	}

	// 3. Query library with CSV file present (should return 3 exercises combined)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/exercicios/biblioteca", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 ok, got %d", w.Code)
	}

	var respCSV struct {
		Exercicios        []combinedExercise `json:"exercicios"`
		Total             int                `json:"total"`
		TotalNormais      int                `json:"total_normais"`
		TotalTerapeuticos int                `json:"total_terapeuticos"`
	}
	_ = json.NewDecoder(w.Body).Decode(&respCSV)

	if respCSV.Total != 3 {
		t.Errorf("expected 3 total combined exercises, got %d", respCSV.Total)
	}

	if respCSV.TotalNormais != 2 || respCSV.TotalTerapeuticos != 1 {
		t.Errorf("mismatch combined counts: normais=%d, terapeuticos=%d", respCSV.TotalNormais, respCSV.TotalTerapeuticos)
	}

	// Verify sorting (descending by code): 5678 -> 5000 -> 1234
	if respCSV.Exercicios[0].Codigo != 5678 || respCSV.Exercicios[1].Codigo != 5000 || respCSV.Exercicios[2].Codigo != 1234 {
		t.Errorf("expected descending sort order: 5678, 5000, 1234; got: %d, %d, %d",
			respCSV.Exercicios[0].Codigo, respCSV.Exercicios[1].Codigo, respCSV.Exercicios[2].Codigo)
	}

	// 4. Query with search term filters
	req = httptest.NewRequest(http.MethodGet, "/api/v1/exercicios/biblioteca?busca=Abdominal", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var respFilter struct {
		Exercicios []combinedExercise `json:"exercicios"`
	}
	_ = json.NewDecoder(w.Body).Decode(&respFilter)
	if len(respFilter.Exercicios) != 1 || respFilter.Exercicios[0].Nome != "Abdominal Supra" {
		t.Errorf("failed to filter by search term, got: %+v", respFilter.Exercicios)
	}
}

func TestExerciciosSugestoesModeration(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "http-sugestoes-test-*")
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
	router := NewRouter(cfg, db)

	// Seed 2 pending suggestions manually in the database
	ctx := t.Context()
	_, err = db.ExecContext(ctx, `
		INSERT INTO sugestoes_exercicios_rehab (id, nome_exercicio, tipo_exercicio, nivel_prioridade, frequencia_sugestao, rag_fonte, justificativa_clinica, status, data_sugestao)
		VALUES 
			(10, 'Caminhada Rápida Assistida', 'Cardio', 1, 5, 'Fonte Clinica 1', 'Melhoria cardiovascular', 'pendente', '2026-07-16 12:00:00'),
			(11, 'Mobilização Cervical Leve', 'Mobilidade', 3, 2, 'Fonte Clinica 2', 'Aumento de amplitude de movimento', 'pendente', '2026-07-16 12:30:00')
	`)
	if err != nil {
		t.Fatalf("failed to seed suggestions: %v", err)
	}

	// 1. List suggestions (GET /api/v1/exercicios/sugestoes)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/exercicios/sugestoes?ordem=frequencia", nil)
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 ok, got %d", w.Code)
	}

	var sugList []domain.SugestaoExercicioRehab
	if err := json.NewDecoder(w.Body).Decode(&sugList); err != nil {
		t.Fatalf("failed to decode suggestions: %v", err)
	}

	if len(sugList) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(sugList))
	}

	// Caminhada Rápida Assistida has higher frequency (5 vs 2), should be first
	if sugList[0].ID != 10 {
		t.Errorf("expected suggestion ID 10 first by frequency sort, got ID %d", sugList[0].ID)
	}

	// 2. Reject suggestion ID 11
	rejectBody := `{"motivo": "Exercício de alto risco para este grupo"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/exercicios/sugestoes/11/rejeitar", bytes.NewBufferString(rejectBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 ok for reject, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify ID 11 is rejected
	req = httptest.NewRequest(http.MethodGet, "/api/v1/exercicios/sugestoes", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	_ = json.NewDecoder(w.Body).Decode(&sugList)
	if len(sugList) != 1 || sugList[0].ID != 10 {
		t.Errorf("expected only ID 10 in pending list, got %+v", sugList)
	}

	// 3. Approve suggestion ID 10 (Transacional)
	approveBody := `{
		"nome": "Caminhada Rápida Aprovada",
		"descricao_terapeutica": "Descrição do profissional"
	}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/exercicios/sugestoes/10/aprovar", bytes.NewBufferString(approveBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 ok, got %d. Body: %s", w.Code, w.Body.String())
	}

	var approveResp struct {
		Status string `json:"status"`
		Codigo int    `json:"codigo"`
	}
	_ = json.NewDecoder(w.Body).Decode(&approveResp)

	if approveResp.Codigo != 5000 {
		t.Errorf("expected approved exercise code to be 5000, got %d", approveResp.Codigo)
	}

	// Verify exercise is created in database
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/exercicios/personalizados/%d", approveResp.Codigo), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("failed to retrieve approved exercise: %d", w.Code)
	}

	var createdEx domain.ExercicioReabilitacao
	_ = json.NewDecoder(w.Body).Decode(&createdEx)
	if createdEx.Nome != "Caminhada Rápida Aprovada" || createdEx.Categoria != "terapeutico" {
		t.Errorf("created exercise fields mismatch: %+v", createdEx)
	}
}
