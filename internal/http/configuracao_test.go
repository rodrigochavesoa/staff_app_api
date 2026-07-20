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

func TestConfiguracoesAndDashboardEndpoints(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-config-test-*")
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

	// Create some stats fixtures in the DB:
	// - 2 Alunos (1 active, 1 inactive)
	// - 1 Anamnese (for student 1, risk score 3, approved)
	// - 1 pending Pre-registro
	// - 2 Planos
	_, err = db.ExecContext(ctx, "INSERT INTO planos (id, nome, preco_default, ativo) VALUES (1, 'Plano Trimestral Teste', 299.90, 1)")
	if err != nil {
		t.Fatalf("failed to insert plano: %v", err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO planos (id, nome, preco_default, ativo) VALUES (2, 'Plano Mensal Teste', 119.90, 1)")
	if err != nil {
		t.Fatalf("failed to insert plano: %v", err)
	}

	_, err = db.ExecContext(ctx, "INSERT INTO alunos (id, nome, idade, sexo, email, ativo, plano_id, plano_ativo) VALUES (1, 'Aluno Ativo', 28, 'M', 'ativo@test.com', 1, 1, 1)")
	if err != nil {
		t.Fatalf("failed to insert active student: %v", err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO alunos (id, nome, idade, sexo, email, ativo, plano_id, plano_ativo) VALUES (2, 'Aluno Inativo', 35, 'F', 'inativo@test.com', 0, 2, 1)")
	if err != nil {
		t.Fatalf("failed to insert inactive student: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO anamneses (id, aluno_id, status_aprovacao, risk_score_cached, ativa)
		VALUES (1, 1, 'pendente', 3.0, 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert anamnese: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO pre_registros (id, nome, email, status, expira_em)
		VALUES (1, 'Pre Registro Pendente', 'pre@test.com', 'aguardando_aprovacao', '2026-07-20 18:00:00')
	`)
	if err != nil {
		t.Fatalf("failed to insert pre-registro: %v", err)
	}

	cfg := &config.Config{CorsOrigins: []string{"*"}}
	router := NewRouter(cfg, depsForTestDB(db))
	authHeader := testAuthHeader(t, db, cfg)

	// ----------------------------------------------------
	// 1. Test GET /api/v1/admin/configuracoes (List & Masking)
	// ----------------------------------------------------
	req, _ := http.NewRequestWithContext(ctx, "GET", "/api/v1/admin/configuracoes", nil)
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /configuracoes expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var listResp struct {
		Configuracoes []domain.Configuracao `json:"configuracoes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to unmarshal configurations list: %v", err)
	}

	var passwordConfig *domain.Configuracao
	for i, c := range listResp.Configuracoes {
		if c.Chave == "SMTP_PASSWORD" {
			passwordConfig = &listResp.Configuracoes[i]
		}
	}

	if passwordConfig == nil {
		t.Fatal("expected SMTP_PASSWORD key in configurations list")
	}

	if passwordConfig.Valor != "" && passwordConfig.Valor != "********" {
		t.Errorf("expected sensitive value to be masked, got: %q", passwordConfig.Valor)
	}

	// ----------------------------------------------------
	// 2. Test PUT /api/v1/admin/configuracoes (Save & Validation)
	// ----------------------------------------------------
	// Case A: Reject Unknown Key
	updateUnknownPayload := `{"configuracoes":{"INVALID_KEY_NAME":"some-value"}}`
	req, _ = http.NewRequestWithContext(ctx, "PUT", "/api/v1/admin/configuracoes", bytes.NewBufferString(updateUnknownPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("PUT /configuracoes expected 400 for unknown key, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "não permitida") {
		t.Errorf("expected error message to complain about key not allowed, got: %s", w.Body.String())
	}

	// Case B: Reject Invalid Types (boolean validation)
	updateInvalidBoolPayload := `{"configuracoes":{"SMTP_ENABLED":"not-a-boolean"}}`
	req, _ = http.NewRequestWithContext(ctx, "PUT", "/api/v1/admin/configuracoes", bytes.NewBufferString(updateInvalidBoolPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("PUT /configuracoes expected 400 for invalid boolean, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Case C: Reject Invalid SMTP Port Range
	updateInvalidPortPayload := `{"configuracoes":{"SMTP_PORT":"999999"}}`
	req, _ = http.NewRequestWithContext(ctx, "PUT", "/api/v1/admin/configuracoes", bytes.NewBufferString(updateInvalidPortPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("PUT /configuracoes expected 400 for invalid port, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Case D: Reject Invalid Email Format
	updateInvalidEmailPayload := `{"configuracoes":{"SMTP_FROM_EMAIL":"invalid-email-no-at"}}`
	req, _ = http.NewRequestWithContext(ctx, "PUT", "/api/v1/admin/configuracoes", bytes.NewBufferString(updateInvalidEmailPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("PUT /configuracoes expected 400 for invalid email, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Case E: Transactional Atomicity (If one invalid, none applied)
	updateMultiplePayload := `{"configuracoes":{
		"SMTP_HOST":"new-host.com",
		"SMTP_PORT":"-5"
	}}`
	req, _ = http.NewRequestWithContext(ctx, "PUT", "/api/v1/admin/configuracoes", bytes.NewBufferString(updateMultiplePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("PUT /configuracoes expected 400 for mixed update with invalid port, got %d", w.Code)
	}

	// Verify SMTP_HOST was NOT updated partially
	row := db.QueryRowContext(ctx, "SELECT valor FROM configuracoes_sistema WHERE chave = 'SMTP_HOST'")
	var hostVal string
	_ = row.Scan(&hostVal)
	if hostVal == "new-host.com" {
		t.Error("transactional atomic update failed! SMTP_HOST was partially updated despite invalid port in same request.")
	}

	// Case F: Valid Update
	validUpdatePayload := `{"configuracoes":{
		"SMTP_ENABLED":"true",
		"SMTP_HOST":"smtp.test.com",
		"SMTP_PORT":"587",
		"SMTP_USER":"test-user",
		"SMTP_PASSWORD":"test-password",
		"SMTP_FROM_EMAIL":"from@test.com",
		"SMTP_FROM_NAME":"RC Test"
	}}`
	req, _ = http.NewRequestWithContext(ctx, "PUT", "/api/v1/admin/configuracoes", bytes.NewBufferString(validUpdatePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /configuracoes expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify DB values and updated_by field (admin user ID is 1 from testAuthHeader)
	var savedHost, savedPort, savedUser, savedPass string
	var updatedBy sql.NullInt64
	err = db.QueryRowContext(ctx, `
		SELECT 
			(SELECT valor FROM configuracoes_sistema WHERE chave = 'SMTP_HOST'),
			(SELECT valor FROM configuracoes_sistema WHERE chave = 'SMTP_PORT'),
			(SELECT valor FROM configuracoes_sistema WHERE chave = 'SMTP_USER'),
			(SELECT valor FROM configuracoes_sistema WHERE chave = 'SMTP_PASSWORD'),
			(SELECT atualizado_por FROM configuracoes_sistema WHERE chave = 'SMTP_HOST')
	`).Scan(&savedHost, &savedPort, &savedUser, &savedPass, &updatedBy)
	if err != nil {
		t.Fatalf("failed to query saved configurations: %v", err)
	}

	if savedHost != "smtp.test.com" || savedPort != "587" || savedUser != "test-user" || savedPass != "test-password" {
		t.Errorf("configurations not saved correctly. Got: host=%q, port=%q, user=%q, pass=%q", savedHost, savedPort, savedUser, savedPass)
	}
	if !updatedBy.Valid || updatedBy.Int64 <= 0 {
		t.Errorf("expected atualizado_por to be updated with authenticated admin ID, got: %+v", updatedBy)
	}

	// Case G: Masked password ("********") keeps original saved value
	maskedUpdatePayload := `{"configuracoes":{
		"SMTP_PASSWORD":"********",
		"SMTP_HOST":"another-host.com"
	}}`
	req, _ = http.NewRequestWithContext(ctx, "PUT", "/api/v1/admin/configuracoes", bytes.NewBufferString(maskedUpdatePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /configuracoes with mask expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	err = db.QueryRowContext(ctx, `
		SELECT 
			(SELECT valor FROM configuracoes_sistema WHERE chave = 'SMTP_HOST'),
			(SELECT valor FROM configuracoes_sistema WHERE chave = 'SMTP_PASSWORD')
	`).Scan(&savedHost, &savedPass)
	if err != nil {
		t.Fatalf("failed to query configs: %v", err)
	}

	if savedHost != "another-host.com" {
		t.Errorf("expected host to be updated to another-host.com, got %q", savedHost)
	}
	if savedPass != "test-password" {
		t.Errorf("expected password to keep its value 'test-password' when '********' is sent, but got %q", savedPass)
	}

	// ----------------------------------------------------
	// 3. Test POST /api/v1/admin/configuracoes/testar-smtp
	// ----------------------------------------------------
	// Since connection will fail with bad server, check that we get a sanitized error response and no secrets leakage.
	testSMTPPayload := `{"to_email":"destination@test.com","host":"localhost-nonexistent-smtp.com","port":"25"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/admin/configuracoes/testar-smtp", bytes.NewBufferString(testSMTPPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /testar-smtp expected 400 on bad host, got %d. Body: %s", w.Code, w.Body.String())
	}

	var smtpErrResp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &smtpErrResp)
	if !strings.Contains(smtpErrResp["error"], "Falha ao enviar e-mail de teste") {
		t.Errorf("expected sanitized error message, got: %+v", smtpErrResp)
	}

	// ----------------------------------------------------
	// 4. Test GET /api/v1/admin/dashboard/stats (Metrics)
	// ----------------------------------------------------
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/admin/dashboard/stats", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /dashboard/stats expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var stats domain.DashboardStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to unmarshal dashboard stats: %v", err)
	}

	if stats.Alunos.Total != 2 || stats.Alunos.Ativos != 1 || stats.Alunos.Inativos != 1 || stats.Alunos.SemAnamnese != 1 {
		t.Errorf("unexpected student statistics: %+v", stats.Alunos)
	}

	if stats.Anamneses.Total != 1 || stats.Anamneses.PendentesAprovacao != 1 || stats.Anamneses.AltoRisco != 1 {
		t.Errorf("unexpected anamnese statistics: %+v", stats.Anamneses)
	}

	if stats.PreRegistrosPendentes != 1 {
		t.Errorf("expected 1 pending pre-registration, got %d", stats.PreRegistrosPendentes)
	}

	if len(stats.DistribuicaoPlanos) != 1 {
		t.Errorf("expected 1 plan in active student distribution, got %d", len(stats.DistribuicaoPlanos))
	} else {
		dp := stats.DistribuicaoPlanos[0]
		if dp.Nome != "Plano Trimestral Teste" || dp.QuantidadeAlunos != 1 {
			t.Errorf("unexpected active student distribution by plan: %+v", dp)
		}
	}
}
