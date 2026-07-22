package http

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
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

func TestGenerateSecureTokenEntropyFailure(t *testing.T) {
	orig := randomRead
	t.Cleanup(func() { randomRead = orig })

	randomRead = func(b []byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}

	_, err := generateSecureToken()
	if err == nil {
		t.Fatal("expected error when rand.Read fails")
	}
	if !strings.Contains(err.Error(), "generate secure token") {
		t.Fatalf("expected wrapped error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "entropy unavailable") {
		t.Fatalf("expected entropy unavailable cause, got: %v", err)
	}
}

func TestPreCadastroApproveAutoGenerateAnamneseToken(t *testing.T) {
	logger.Setup("development", false)

	orig := randomRead
	t.Cleanup(func() { randomRead = orig })

	tempDir, err := os.MkdirTemp("", "http-pre-cadastro-token-*")
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
	router := NewRouter(cfg, depsForTestDB(db))

	planoRepo := sqlite.NewPlanoRepository(db)
	plan := &domain.Plano{
		Nome:         "Plano Teste Entropy",
		PrecoDefault: 99.00,
		Descricao:    "Plano para teste de token",
		Ativo:        true,
	}
	if err := planoRepo.Create(t.Context(), plan); err != nil {
		t.Fatalf("failed to create test plan: %v", err)
	}

	configRepo := sqlite.NewConfiguracaoRepository(db)
	autoGenConf, err := configRepo.GetByChave(t.Context(), "AUTO_GENERATE_ANAMNESE_ON_APPROVE")
	if err != nil {
		t.Fatalf("failed to get AUTO_GENERATE config: %v", err)
	}
	if autoGenConf == nil {
		t.Fatal("AUTO_GENERATE_ANAMNESE_ON_APPROVE config missing")
	}
	autoGenConf.Valor = "true"
	if err := configRepo.Update(t.Context(), autoGenConf); err != nil {
		t.Fatalf("failed to enable AUTO_GENERATE: %v", err)
	}

	t.Run("happy_path", func(t *testing.T) {
		randomRead = rand.Read
		t.Cleanup(func() { randomRead = orig })

		preID := createPendingPreCadastro(t, router, plan.ID, "Ana Happy", "ana.happy@example.com")
		code, body := approvePreCadastro(t, router, adminHeader, preID)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", code, body)
		}

		var resp struct {
			Success      bool   `json:"success"`
			AlunoID      int64  `json:"aluno_id"`
			AnamneseLink string `json:"anamnese_link"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("failed to decode approve response: %v", err)
		}
		if !resp.Success || resp.AlunoID <= 0 || resp.AnamneseLink == "" {
			t.Fatalf("expected success with aluno_id and anamnese_link, got: %s", body)
		}

		var tokensCount int
		if err := db.QueryRowContext(t.Context(),
			"SELECT COUNT(*) FROM anamnese_tokens WHERE pre_registro_id = ?", preID,
		).Scan(&tokensCount); err != nil {
			t.Fatalf("failed to count tokens: %v", err)
		}
		if tokensCount != 1 {
			t.Fatalf("expected 1 anamnese token, got %d", tokensCount)
		}
	})

	t.Run("entropy_failure", func(t *testing.T) {
		randomRead = func(b []byte) (int, error) {
			return 0, errors.New("entropy unavailable")
		}
		t.Cleanup(func() { randomRead = orig })

		preID := createPendingPreCadastro(t, router, plan.ID, "Bruno Fail", "bruno.fail@example.com")
		code, body := approvePreCadastro(t, router, adminHeader, preID)
		if code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d. Body: %s", code, body)
		}

		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(body), &errResp); err != nil {
			t.Fatalf("failed to decode error body: %v", err)
		}
		if errResp.Error != "Internal server error" {
			t.Fatalf("expected generic Internal server error, got: %q", errResp.Error)
		}
		if strings.Contains(body, "entropy") || strings.Contains(body, "generate secure token") {
			t.Fatalf("response must not leak cryptographic detail: %s", body)
		}

		var tokensCount int
		if err := db.QueryRowContext(t.Context(),
			"SELECT COUNT(*) FROM anamnese_tokens WHERE pre_registro_id = ?", preID,
		).Scan(&tokensCount); err != nil {
			t.Fatalf("failed to count tokens: %v", err)
		}
		if tokensCount != 0 {
			t.Fatalf("expected 0 anamnese tokens after entropy failure, got %d", tokensCount)
		}
	})
}

func createPendingPreCadastro(t *testing.T, router http.Handler, planoID int64, nome, email string) int64 {
	t.Helper()
	payload := fmt.Sprintf(`{
		"nome": %q,
		"email": %q,
		"telefone": "11999990000",
		"data_nascimento": "1990-01-01",
		"genero": "outro",
		"plano_id": %d
	}`, nome, email, planoID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pre-cadastro", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for pre-cadastro, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		PreRegistroID int64 `json:"pre_registro_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode pre-cadastro response: %v", err)
	}
	if resp.PreRegistroID <= 0 {
		t.Fatalf("expected positive pre_registro_id, got %d", resp.PreRegistroID)
	}
	return resp.PreRegistroID
}

func approvePreCadastro(t *testing.T, router http.Handler, adminHeader string, preID int64) (int, string) {
	t.Helper()
	approveURL := fmt.Sprintf("/api/v1/admin/pre-cadastros/%d/aprovar", preID)
	req := httptest.NewRequest(http.MethodPost, approveURL, nil)
	req.Header.Set("Authorization", adminHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}
