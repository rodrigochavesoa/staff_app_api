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
	"time"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"

	"golang.org/x/crypto/bcrypt"
)

func TestMeFichasAndAlunoFichasAuthz(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "http-me-fichas-test-*")
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
		SecretKey:   "test-secret-me-fichas",
	}
	adminHeader := testAuthHeader(t, db, cfg)
	router := NewRouter(cfg, depsForTestDB(db))

	userRepo := sqlite.NewUserRepository(db)
	alunoRepo := sqlite.NewAlunoRepository(db)

	linkedUser, linkedToken := createPortalUser(t, userRepo, cfg, "portal-aluno", "portal@example.com", false)
	usuarioID := linkedUser.ID
	linkedAluno := &domain.Aluno{
		Nome:      "Aluno Portal",
		Idade:     28,
		Sexo:      "F",
		Email:     "aluno.portal@example.com",
		UsuarioID: &usuarioID,
		Ativo:     true,
	}
	if err := alunoRepo.Create(t.Context(), linkedAluno); err != nil {
		t.Fatalf("failed to create linked aluno: %v", err)
	}

	otherAluno := &domain.Aluno{
		Nome:  "Outro Aluno",
		Idade: 30,
		Sexo:  "M",
		Email: "outro@example.com",
		Ativo: true,
	}
	if err := alunoRepo.Create(t.Context(), otherAluno); err != nil {
		t.Fatalf("failed to create other aluno: %v", err)
	}

	_, unlinkedToken := createPortalUser(t, userRepo, cfg, "sem-vinculo", "sem@example.com", false)

	// Seed a ficha web for the linked aluno via admin create endpoint.
	_, err = db.ExecContext(t.Context(),
		"INSERT INTO fichas_treino_web (id, aluno, ficha_json) VALUES (200, 'Aluno Portal', '{\"frequencia\": 3}')")
	if err != nil {
		t.Fatalf("failed to insert ficha treino: %v", err)
	}
	createPayload := fmt.Sprintf(`{"ficha_id": 200, "aluno_id": %d, "conteudo": {"secreto": "nao-vazar"}}`, linkedAluno.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/criar-ficha", bytes.NewBufferString(createPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 create ficha, got %d. Body: %s", w.Code, w.Body.String())
	}

	t.Run("linked_aluno_me_fichas_200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/fichas", nil)
		req.Header.Set("Authorization", "Bearer "+linkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var resp AlunoFichasResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.AlunoID != linkedAluno.ID || resp.Total != 1 || len(resp.Fichas) != 1 {
			t.Fatalf("unexpected me/fichas payload: %+v", resp)
		}
		if resp.Fichas[0].ConteudoJSON != "" {
			t.Fatalf("list must not expose conteudo_json, got %q", resp.Fichas[0].ConteudoJSON)
		}
	})

	t.Run("unlinked_user_me_fichas_404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/fichas", nil)
		req.Header.Set("Authorization", "Bearer "+unlinkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d. Body: %s", w.Code, w.Body.String())
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error: %v", err)
		}
		if errResp.Error != "Aluno não vinculado a esta conta." {
			t.Fatalf("unexpected error message: %q", errResp.Error)
		}
	})

	t.Run("non_admin_other_aluno_fichas_403", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/aluno/%d/fichas", otherAluno.ID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+linkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d. Body: %s", w.Code, w.Body.String())
		}
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("failed to decode error: %v", err)
		}
		if errResp.Error != "Acesso negado." {
			t.Fatalf("unexpected error message: %q", errResp.Error)
		}
	})

	t.Run("admin_any_aluno_fichas_200", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/aluno/%d/fichas", linkedAluno.ID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", adminHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for admin, got %d. Body: %s", w.Code, w.Body.String())
		}
		var resp AlunoFichasResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.AlunoID != linkedAluno.ID || resp.Total != 1 {
			t.Fatalf("unexpected admin list payload: %+v", resp)
		}
	})

	t.Run("linked_aluno_own_staff_route_200", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/aluno/%d/fichas", linkedAluno.ID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+linkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for own aluno_id, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

func createPortalUser(t *testing.T, userRepo *sqlite.UserRepository, cfg *config.Config, username, email string, isAdmin bool) (*domain.User, string) {
	t.Helper()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("student-pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	user := &domain.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(passwordHash),
		NomeCompleto: username,
		IsAdmin:      isAdmin,
		Ativo:        true,
		Aprovado:     true,
	}
	if err := userRepo.Create(t.Context(), user); err != nil {
		t.Fatalf("failed to create user %s: %v", username, err)
	}
	token, err := NewAuthHandler(userRepo, nil, cfg.SecretKey).signToken(user, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to sign token for %s: %v", username, err)
	}
	return user, token
}
