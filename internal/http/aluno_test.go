package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func TestAlunoEndpoints(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-aluno-test-*")
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

	var ctx context.Context = t.Context()

	cfg := &config.Config{CorsOrigins: []string{"*"}}
	router := NewRouter(cfg, depsForTestDB(db))
	authHeader := testAuthHeader(t, db, cfg)

	// 1. POST /api/v1/alunos - Success
	payload := `{"nome": "Carlos João", "idade": 32, "sexo": "M", "email": "carlos@test.com", "objetivo": "Maratona"}`
	req, _ := http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var created domain.Aluno
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if created.ID == 0 || created.Nome != "Carlos João" || created.Idade != 32 || !created.Ativo {
		t.Errorf("unexpected created student data: %+v", created)
	}

	// 2. POST /api/v1/alunos - Validation (Empty Name)
	payloadEmptyName := `{"nome": "", "idade": 32, "sexo": "M", "email": "carlos2@test.com"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos", bytes.NewBufferString(payloadEmptyName))
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty name, got %d", w.Code)
	}

	// 3. POST /api/v1/alunos - Validation (Invalid Age)
	payloadInvalidAge := `{"nome": "Maria", "idade": -5, "sexo": "F", "email": "maria@test.com"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos", bytes.NewBufferString(payloadInvalidAge))
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for negative age, got %d", w.Code)
	}

	// 4. POST /api/v1/alunos - Validation (UTF-8 Safe Slicing of long fields)
	// Let's pass a very long goal containing Portuguese accents like "áóãç"
	accentedLongGoal := strings.Repeat("ação", 100) // 400 characters (runs into rune slicing limit of 250 runes)
	payloadLongGoal := `{"nome": "Maria Silva", "idade": 25, "sexo": "F", "email": "maria@test.com", "objetivo": "` + accentedLongGoal + `"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos", bytes.NewBufferString(payloadLongGoal))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created with sliced goal, got %d. Body: %s", w.Code, w.Body.String())
	}

	var created2 domain.Aluno
	_ = json.Unmarshal(w.Body.Bytes(), &created2)

	if utf8.RuneCountInString(created2.Objetivo) != 250 {
		t.Errorf("expected student goal to be safely sliced to exactly 250 runes, got %d runes", utf8.RuneCountInString(created2.Objetivo))
	}
	// Verify it does not end with a corrupted UTF-8 byte
	if !utf8.ValidString(created2.Objetivo) {
		t.Error("safely sliced goal contains invalid UTF-8 sequences")
	}

	// 5. GET /api/v1/alunos/{id} - Success
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos/"+strconv.FormatInt(created.ID, 10), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var retrieved domain.Aluno
	_ = json.Unmarshal(w.Body.Bytes(), &retrieved)
	if retrieved.ID != created.ID || retrieved.Nome != "Carlos João" {
		t.Errorf("mismatch on retrieved student: %+v", retrieved)
	}

	// 6. PUT /api/v1/alunos/{id} - Update Details
	updatePayload := `{"nome": "Carlos Editado", "idade": 33, "sexo": "M", "email": "carlos_new@test.com"}`
	req, _ = http.NewRequestWithContext(ctx, "PUT", "/api/v1/alunos/"+strconv.FormatInt(created.ID, 10), bytes.NewBufferString(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for update, got %d. Body: %s", w.Code, w.Body.String())
	}

	var updated domain.Aluno
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Nome != "Carlos Editado" || updated.Idade != 33 || updated.Email != "carlos_new@test.com" {
		t.Errorf("update fields mismatch: %+v", updated)
	}

	// 7. GET /api/v1/alunos - List Active Students
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var list []*domain.Aluno
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Errorf("expected 2 active students in list, got %d", len(list))
	}

	// 8. DELETE /api/v1/alunos/{id} - Soft Delete
	req, _ = http.NewRequestWithContext(ctx, "DELETE", "/api/v1/alunos/"+strconv.FormatInt(created.ID, 10), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content for delete, got %d", w.Code)
	}

	// Verify only 1 active student remains
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var listAfterDelete []*domain.Aluno
	_ = json.Unmarshal(w.Body.Bytes(), &listAfterDelete)
	if len(listAfterDelete) != 1 {
		t.Errorf("expected 1 active student in list after soft delete, got %d", len(listAfterDelete))
	}

	// 9. POST /api/v1/alunos/{id}/reativar - Reactivate Student
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos/"+strconv.FormatInt(created.ID, 10)+"/reativar", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content for reactivate, got %d", w.Code)
	}

	// Verify both students active again
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var listAfterReactivate []*domain.Aluno
	_ = json.Unmarshal(w.Body.Bytes(), &listAfterReactivate)
	if len(listAfterReactivate) != 2 {
		t.Errorf("expected 2 active students in list after reactivation, got %d", len(listAfterReactivate))
	}
}
