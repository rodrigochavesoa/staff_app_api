package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"

	"golang.org/x/crypto/bcrypt"
)

func testAuthHeader(t *testing.T, db *sqlite.DB, cfg *config.Config) string {
	t.Helper()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte("admin-change-me-immediately"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash test admin password: %v", err)
	}

	user := &domain.User{
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordHash: string(passwordHash),
		NomeCompleto: "Administrador",
		IsAdmin:      true,
		Ativo:        true,
		Aprovado:     true,
	}
	if err := sqlite.NewUserRepository(db).Create(t.Context(), user); err != nil {
		t.Fatalf("failed to create test admin: %v", err)
	}

	token, err := NewAuthHandler(sqlite.NewUserRepository(db), cfg.SecretKey).signToken(user, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return "Bearer " + token
}

func TestAuthUsersPlansFlow(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "http-auth-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sqlite.Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{CorsOrigins: []string{"*"}}
	adminHeader := testAuthHeader(t, db, cfg)
	router := NewRouter(cfg, db)

	registerPayload := `{
		"username": "coach1",
		"email": "coach1@example.com",
		"password": "secret1",
		"nome_completo": "Coach Um"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(registerPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 register, got %d. Body: %s", w.Code, w.Body.String())
	}

	loginPendingPayload := `{"username":"coach1","password":"secret1"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(loginPendingPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for pending user login, got %d. Body: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/usuarios", nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 users list, got %d. Body: %s", w.Code, w.Body.String())
	}
	var usersResp struct {
		Usuarios []struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"usuarios"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &usersResp); err != nil {
		t.Fatalf("failed to decode users response: %v", err)
	}
	var coachID int64
	for _, user := range usersResp.Usuarios {
		if user.Username == "coach1" {
			coachID = user.ID
			break
		}
	}
	if coachID == 0 {
		t.Fatalf("registered coach was not listed: %+v", usersResp)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/usuarios/"+strconvFormatInt(coachID)+"/aprovar", nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 approve, got %d. Body: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(loginPendingPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 approved login, got %d. Body: %s", w.Code, w.Body.String())
	}
	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	if loginResp.Token == "" {
		t.Fatal("expected login token")
	}
	coachHeader := "Bearer " + loginResp.Token

	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", coachHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 me, got %d. Body: %s", w.Code, w.Body.String())
	}

	changePayload := `{"password_atual":"secret1","password_nova":"newsecret1"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/alterar-senha", bytes.NewBufferString(changePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", coachHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 change password, got %d. Body: %s", w.Code, w.Body.String())
	}

	planPayload := `{"nome":"Plano Mensal","preco_default":199.90,"descricao":"Base"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/planos", bytes.NewBufferString(planPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 create plan, got %d. Body: %s", w.Code, w.Body.String())
	}
	var plan domain.Plano
	if err := json.Unmarshal(w.Body.Bytes(), &plan); err != nil {
		t.Fatalf("failed to decode plan response: %v", err)
	}
	if plan.ID == 0 || plan.Nome != "Plano Mensal" || !plan.Ativo {
		t.Fatalf("unexpected plan response: %+v", plan)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/planos", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 public plans list, got %d. Body: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/planos/"+strconvFormatInt(plan.ID), nil)
	req.Header.Set("Authorization", adminHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 delete plan, got %d. Body: %s", w.Code, w.Body.String())
	}
}
