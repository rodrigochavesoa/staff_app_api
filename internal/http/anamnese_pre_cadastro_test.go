package http

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestAnamnesePreCadastroFlow(t *testing.T) {
	logger.Setup("development", false)

	// Setup temp database
	tempDir, err := os.MkdirTemp("", "http-anamnese-test-*")
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

	// Create test admin and token
	adminHeader := testAuthHeader(t, db, cfg)
	router := NewRouter(cfg, db)

	// Create a test plan first
	planoRepo := sqlite.NewPlanoRepository(db)
	plan := &domain.Plano{
		Nome:         "Plano Corrida Anual",
		PrecoDefault: 120.00,
		Descricao:    "Plano anual de teste",
		Ativo:        true,
	}
	if err := planoRepo.Create(t.Context(), plan); err != nil {
		t.Fatalf("failed to create test plan: %v", err)
	}

	// 1. Submit a PreCadastro (Public)
	preCadastroPayload := fmt.Sprintf(`{
		"nome": "Pedro Silva",
		"email": "pedrosilva@example.com",
		"telefone": "11988887777",
		"data_nascimento": "1995-10-20",
		"genero": "masculino",
		"plano_id": %d
	}`, plan.ID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pre-cadastro", bytes.NewBufferString(preCadastroPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var preCadastroResp struct {
		Success       bool  `json:"success"`
		PreRegistroID int64 `json:"pre_registro_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&preCadastroResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if preCadastroResp.PreRegistroID <= 0 {
		t.Fatalf("expected positive pre_registro_id, got %d", preCadastroResp.PreRegistroID)
	}

	// 2. Try duplicate PreCadastro (should fail with StatusConflict)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/pre-cadastro", bytes.NewBufferString(preCadastroPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 conflict, got %d", w.Code)
	}

	// 3. Get PreCadastro List (Protected)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/pre-cadastros", nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d", w.Code)
	}

	var listResp struct {
		Total        int                  `json:"total"`
		PreRegistros []domain.PreRegistro `json:"pre_registros"`
	}
	if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
		t.Fatalf("failed to decode pre-registration list: %v", err)
	}
	if listResp.Total != 1 {
		t.Fatalf("expected 1 pre-registration in list, got %d", listResp.Total)
	}

	// 4. Get PreCadastro detail and audit
	detailURL := fmt.Sprintf("/api/v1/admin/pre-cadastros/%d", preCadastroResp.PreRegistroID)
	req = httptest.NewRequest(http.MethodGet, detailURL, nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 details, got %d", w.Code)
	}

	var detailsResp struct {
		ID         int64  `json:"id"`
		Nome       string `json:"nome"`
		AuditTrail []any  `json:"audit_trail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&detailsResp); err != nil {
		t.Fatalf("failed to decode details response: %v", err)
	}
	if len(detailsResp.AuditTrail) != 1 {
		t.Fatalf("expected 1 audit trail entry, got %d", len(detailsResp.AuditTrail))
	}

	// 5. Approve PreCadastro (Protected)
	approveURL := fmt.Sprintf("/api/v1/admin/pre-cadastros/%d/aprovar", preCadastroResp.PreRegistroID)
	req = httptest.NewRequest(http.MethodPost, approveURL, nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for approval, got %d. Body: %s", w.Code, w.Body.String())
	}

	var approveResp struct {
		Success      bool   `json:"success"`
		AlunoID      int64  `json:"aluno_id"`
		AnamneseLink string `json:"anamnese_link"`
	}
	if err := json.NewDecoder(w.Body).Decode(&approveResp); err != nil {
		t.Fatalf("failed to decode approve response: %v", err)
	}
	if approveResp.AlunoID <= 0 || approveResp.AnamneseLink == "" {
		t.Fatalf("expected positive aluno_id and valid anamnese_link")
	}

	// Extract token from link
	parts := strings.Split(approveResp.AnamneseLink, "/")
	token := parts[len(parts)-1]

	// 6. Get Anamnese Metadata (Public)
	metaURL := fmt.Sprintf("/api/v1/anamnese/submit/%s", token)
	req = httptest.NewRequest(http.MethodGet, metaURL, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for metadata, got %d. Body: %s", w.Code, w.Body.String())
	}

	var metaResp struct {
		AlunoNome string `json:"aluno_nome"`
		AlunoID   int64  `json:"aluno_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&metaResp); err != nil {
		t.Fatalf("failed to decode metadata response: %v", err)
	}
	if metaResp.AlunoNome != "Pedro Silva" || metaResp.AlunoID != approveResp.AlunoID {
		t.Fatalf("metadata response mismatch: Name=%s, ID=%d", metaResp.AlunoNome, metaResp.AlunoID)
	}

	// 7. Submit Anamnese Form (Public)
	submitPayload := `{
		"peso": 80.5,
		"altura": 1.80,
		"patologias": "Asma controlada",
		"medicamentos": "Inalador eventual",
		"lesoes_atuais": "Dores musculares leves",
		"dores_cronicas": "Nenhuma",
		"parq_doenca_cardiaca": 0,
		"parq_dor_peito": 0,
		"parq_tontura": 0,
		"parq_problema_osseo": 1,
		"parq_medicamento_pressao": 0,
		"parq_impedimento_activity": 0,
		"experiencia_treino": "Musculação",
		"objetivo_principal": "Correr 5k",
		"contato_emergencia_nome": "Juliana Silva",
		"contato_emergencia_telefone": "11977776666"
	}`
	req = httptest.NewRequest(http.MethodPost, metaURL, bytes.NewBufferString(submitPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 created for submit, got %d. Body: %s", w.Code, w.Body.String())
	}

	var submitResp struct {
		Success    bool  `json:"success"`
		AnamneseID int64 `json:"anamnese_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&submitResp); err != nil {
		t.Fatalf("failed to decode submit response: %v", err)
	}
	if submitResp.AnamneseID <= 0 {
		t.Fatalf("expected positive anamnese_id, got %d", submitResp.AnamneseID)
	}

	// 8. Retrieve Pending Anamneses (Protected)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/anamnese/pendentes", nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for pending anamneses, got %d", w.Code)
	}

	var pendingResp struct {
		Total     int `json:"total"`
		Anamneses []struct {
			ID        int64   `json:"id"`
			AlunoNome string  `json:"aluno_nome"`
			RiskScore float64 `json:"risk_score"`
		} `json:"anamneses"`
	}
	if err := json.NewDecoder(w.Body).Decode(&pendingResp); err != nil {
		t.Fatalf("failed to decode pending list: %v", err)
	}
	if pendingResp.Total != 1 || pendingResp.Anamneses[0].AlunoNome != "Pedro Silva" {
		t.Fatalf("unexpected pending list response")
	}
	// Check the PAR-Q sum score legacy risk score: parq_problema_osseo = 1 => riskScore = 1
	if pendingResp.Anamneses[0].RiskScore != 1 {
		t.Fatalf("expected risk score 1 (sum of positive PAR-Q answers), got %f", pendingResp.Anamneses[0].RiskScore)
	}

	// 9. Get Anamnese Detail
	detailAnamURL := fmt.Sprintf("/api/v1/admin/anamnese/%d", submitResp.AnamneseID)
	req = httptest.NewRequest(http.MethodGet, detailAnamURL, nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 details, got %d", w.Code)
	}

	var detailsAnamResp struct {
		ID        int64   `json:"id"`
		AlunoNome string  `json:"aluno_nome"`
		IMC       float64 `json:"imc"`
	}
	if err := json.NewDecoder(w.Body).Decode(&detailsAnamResp); err != nil {
		t.Fatalf("failed to decode anamnese details response: %v", err)
	}
	// IMC = 80.5 / (1.80 * 1.80) = 80.5 / 3.24 = 24.8456
	if detailsAnamResp.IMC < 24.8 || detailsAnamResp.IMC > 24.9 {
		t.Fatalf("expected IMC around 24.84, got %f", detailsAnamResp.IMC)
	}

	// 10. Approve Anamnese (Protected)
	approveAnamURL := fmt.Sprintf("/api/v1/admin/anamnese/%d/aprovar", submitResp.AnamneseID)
	req = httptest.NewRequest(http.MethodPost, approveAnamURL, nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 approved, got %d", w.Code)
	}

	// 11. Delete Anamnese (Protected)
	req = httptest.NewRequest(http.MethodDelete, detailAnamURL, nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 deleted, got %d", w.Code)
	}
}
