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

func TestAnamneseReenviarEmailAndSettings(t *testing.T) {
	logger.Setup("development", false)

	// Setup temp database
	tempDir, err := os.MkdirTemp("", "http-anamnese-email-test-*")
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

	adminHeader := testAuthHeader(t, db, cfg)
	router := NewRouter(cfg, db)

	// Create a test plan
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

	// Create a test Aluno
	alunoRepo := sqlite.NewAlunoRepository(db)
	aluno := &domain.Aluno{
		Nome:       "Carlos Junior",
		Idade:      30,
		Sexo:       "masculino",
		Email:      "carlos@example.com",
		Telefone:   "11988887777",
		Turma:      "Turma Geral",
		PlanoID:    &plan.ID,
		PlanoValor: &plan.PrecoDefault,
		PlanoPago:  true,
		PlanoAtivo: true,
		Ativo:      true,
	}
	if err := alunoRepo.Create(t.Context(), aluno); err != nil {
		t.Fatalf("failed to create test student: %v", err)
	}

	// 1. Re-send Email when SMTP is disabled (Default)
	reenviarURL := fmt.Sprintf("/api/v1/admin/alunos/%d/anamnese/reenviar-email", aluno.ID)
	req := httptest.NewRequest(http.MethodPost, reenviarURL, nil)
	req.Header.Set("Authorization", adminHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Since SMTP_ENABLED is 'false' (default), it should fail with 500
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 internal server error because SMTP is disabled, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "desabilitado") {
		t.Fatalf("expected error to mention desabilitado, got: %s", w.Body.String())
	}

	// Verify EMAIL_FALHOU audit log is recorded for manual re-send on Aluno Carlos Junior
	var manualFailCount int
	_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM anamnese_tokens_audit WHERE aluno_id = ? AND evento = 'EMAIL_FALHOU'", aluno.ID).Scan(&manualFailCount)
	if manualFailCount != 1 {
		t.Fatalf("expected 1 EMAIL_FALHOU audit log for manual re-send on Aluno %d, got %d", aluno.ID, manualFailCount)
	}

	// 2. Set SMTP_ENABLED = true but empty SMTP host and port
	configRepo := sqlite.NewConfiguracaoRepository(db)
	smtpEnabledConf, _ := configRepo.GetByChave(t.Context(), "SMTP_ENABLED")
	smtpEnabledConf.Valor = "true"
	_ = configRepo.Update(t.Context(), smtpEnabledConf)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "incompletas") {
		t.Fatalf("expected error to mention incomplete configuration, got: %s", w.Body.String())
	}

	// 3. Test AUTO_GENERATE_ANAMNESE_ON_APPROVE = false
	autoGenConf, _ := configRepo.GetByChave(t.Context(), "AUTO_GENERATE_ANAMNESE_ON_APPROVE")
	autoGenConf.Valor = "false"
	_ = configRepo.Update(t.Context(), autoGenConf)

	// Submit a PreCadastro
	preCadastroPayload := fmt.Sprintf(`{
		"nome": "Carla Oliveira",
		"email": "carla@example.com",
		"telefone": "11977776666",
		"data_nascimento": "1998-05-15",
		"genero": "feminino",
		"plano_id": %d
	}`, plan.ID)

	reqPc := httptest.NewRequest(http.MethodPost, "/api/v1/pre-cadastro", bytes.NewBufferString(preCadastroPayload))
	reqPc.Header.Set("Content-Type", "application/json")
	wPc := httptest.NewRecorder()
	router.ServeHTTP(wPc, reqPc)

	var pcResp struct {
		PreRegistroID int64 `json:"pre_registro_id"`
	}
	_ = json.NewDecoder(wPc.Body).Decode(&pcResp)

	// Approve PreCadastro
	approveURL := fmt.Sprintf("/api/v1/admin/pre-cadastros/%d/aprovar", pcResp.PreRegistroID)
	reqApp := httptest.NewRequest(http.MethodPost, approveURL, nil)
	reqApp.Header.Set("Authorization", adminHeader)
	wApp := httptest.NewRecorder()
	router.ServeHTTP(wApp, reqApp)

	if wApp.Code != http.StatusOK {
		t.Fatalf("expected 200 ok on approve, got %d", wApp.Code)
	}

	var appResp struct {
		AlunoID      int64  `json:"aluno_id"`
		AnamneseLink string `json:"anamnese_link"`
	}
	_ = json.NewDecoder(wApp.Body).Decode(&appResp)

	if appResp.AnamneseLink != "" {
		t.Fatalf("expected anamnese_link to be empty because AUTO_GENERATE is false, got: %s", appResp.AnamneseLink)
	}

	// Verify no token was generated for this pre-registration
	tokensCount := 0
	_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM anamnese_tokens WHERE pre_registro_id = ?", pcResp.PreRegistroID).Scan(&tokensCount)
	if tokensCount != 0 {
		t.Fatalf("expected 0 tokens created, got %d", tokensCount)
	}

	// 4. Test AUTO_GENERATE_ANAMNESE_ON_APPROVE = true
	autoGenConf.Valor = "true"
	_ = configRepo.Update(t.Context(), autoGenConf)

	// Submit another PreCadastro
	preCadastroPayload2 := fmt.Sprintf(`{
		"nome": "Lucas Martins",
		"email": "lucas@example.com",
		"telefone": "11966665555",
		"data_nascimento": "2000-02-10",
		"genero": "masculino",
		"plano_id": %d
	}`, plan.ID)

	reqPc2 := httptest.NewRequest(http.MethodPost, "/api/v1/pre-cadastro", bytes.NewBufferString(preCadastroPayload2))
	reqPc2.Header.Set("Content-Type", "application/json")
	wPc2 := httptest.NewRecorder()
	router.ServeHTTP(wPc2, reqPc2)

	var pcResp2 struct {
		PreRegistroID int64 `json:"pre_registro_id"`
	}
	_ = json.NewDecoder(wPc2.Body).Decode(&pcResp2)

	// Approve PreCadastro
	approveURL2 := fmt.Sprintf("/api/v1/admin/pre-cadastros/%d/aprovar", pcResp2.PreRegistroID)
	reqApp2 := httptest.NewRequest(http.MethodPost, approveURL2, nil)
	reqApp2.Header.Set("Authorization", adminHeader)
	wApp2 := httptest.NewRecorder()
	router.ServeHTTP(wApp2, reqApp2)

	if wApp2.Code != http.StatusOK {
		t.Fatalf("expected 200 ok on approve, got %d", wApp2.Code)
	}

	var appResp2 struct {
		AlunoID      int64  `json:"aluno_id"`
		AnamneseLink string `json:"anamnese_link"`
	}
	_ = json.NewDecoder(wApp2.Body).Decode(&appResp2)

	if appResp2.AnamneseLink == "" {
		t.Fatalf("expected anamnese_link to be returned because AUTO_GENERATE is true")
	}

	// Verify token was generated
	_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM anamnese_tokens WHERE pre_registro_id = ?", pcResp2.PreRegistroID).Scan(&tokensCount)
	if tokensCount != 1 {
		t.Fatalf("expected 1 token created, got %d", tokensCount)
	}

	// Check audit table has "GERADO" event
	var event string
	_ = db.QueryRowContext(t.Context(), "SELECT evento FROM anamnese_tokens_audit WHERE pre_registro_id = ? ORDER BY id DESC LIMIT 1", pcResp2.PreRegistroID).Scan(&event)
	if event != "GERADO" {
		t.Fatalf("expected event GERADO, got: %s", event)
	}

	// 5. Test AUTO_SEND_ANAMNESE_EMAIL = true (should attempt auto-send and return email_enviado: false because SMTP is disabled)
	autoSendConf, _ := configRepo.GetByChave(t.Context(), "AUTO_SEND_ANAMNESE_EMAIL")
	autoSendConf.Valor = "true"
	_ = configRepo.Update(t.Context(), autoSendConf)

	// Submit third PreCadastro
	preCadastroPayload3 := fmt.Sprintf(`{
		"nome": "Mariana Souza",
		"email": "mariana@example.com",
		"telefone": "11955554444",
		"data_nascimento": "1994-08-25",
		"genero": "feminino",
		"plano_id": %d
	}`, plan.ID)

	reqPc3 := httptest.NewRequest(http.MethodPost, "/api/v1/pre-cadastro", bytes.NewBufferString(preCadastroPayload3))
	reqPc3.Header.Set("Content-Type", "application/json")
	wPc3 := httptest.NewRecorder()
	router.ServeHTTP(wPc3, reqPc3)

	var pcResp3 struct {
		PreRegistroID int64 `json:"pre_registro_id"`
	}
	_ = json.NewDecoder(wPc3.Body).Decode(&pcResp3)

	// Approve PreCadastro
	approveURL3 := fmt.Sprintf("/api/v1/admin/pre-cadastros/%d/aprovar", pcResp3.PreRegistroID)
	reqApp3 := httptest.NewRequest(http.MethodPost, approveURL3, nil)
	reqApp3.Header.Set("Authorization", adminHeader)
	wApp3 := httptest.NewRecorder()
	router.ServeHTTP(wApp3, reqApp3)

	if wApp3.Code != http.StatusOK {
		t.Fatalf("expected 200 ok on approve, got %d. Body: %s", wApp3.Code, wApp3.Body.String())
	}

	var appResp3 struct {
		AlunoID      int64  `json:"aluno_id"`
		AnamneseLink string `json:"anamnese_link"`
		EmailEnviado bool   `json:"email_enviado"`
		EmailErro    string `json:"email_erro"`
	}
	_ = json.NewDecoder(wApp3.Body).Decode(&appResp3)

	if appResp3.EmailEnviado {
		t.Fatalf("expected email_enviado to be false since SMTP is disabled, got true")
	}

	if !strings.Contains(appResp3.EmailErro, "desabilitado") && !strings.Contains(appResp3.EmailErro, "incompletas") {
		t.Fatalf("expected email_erro to mention 'desabilitado' or 'incompletas', got: %s", appResp3.EmailErro)
	}

	// Verify EMAIL_FALHOU audit log is recorded for this pre-registration
	var failEventCount int
	_ = db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM anamnese_tokens_audit WHERE pre_registro_id = ? AND evento = 'EMAIL_FALHOU'", pcResp3.PreRegistroID).Scan(&failEventCount)
	if failEventCount != 1 {
		t.Fatalf("expected 1 EMAIL_FALHOU audit log for pre-registration %d, got %d", pcResp3.PreRegistroID, failEventCount)
	}

	// Clean up settings to original state
	smtpEnabledConf.Valor = "false"
	_ = configRepo.Update(t.Context(), smtpEnabledConf)
	autoSendConf.Valor = "false"
	_ = configRepo.Update(t.Context(), autoSendConf)
}
