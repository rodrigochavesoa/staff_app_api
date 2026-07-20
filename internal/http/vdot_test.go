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

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func TestVDOTEndpoints(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-vdot-test-*")
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

	// Insert dummy student 1 and student 2 (required for testing cross-student deletion)
	_, err = db.ExecContext(ctx, "INSERT INTO alunos (id, nome, idade, sexo, email) VALUES (1, 'Test Student 1', 25, 'F', 'student1@test.com')")
	if err != nil {
		t.Fatalf("failed to insert dummy student 1: %v", err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO alunos (id, nome, idade, sexo, email) VALUES (2, 'Test Student 2', 30, 'M', 'student2@test.com')")
	if err != nil {
		t.Fatalf("failed to insert dummy student 2: %v", err)
	}

	cfg := &config.Config{CorsOrigins: []string{"*"}}
	router := NewRouter(cfg, depsForTestDB(db))
	authHeader := testAuthHeader(t, db, cfg)

	// 1. Test POST /api/v1/alunos/1/vdot (Success, High PSE = 9 -> Confidence = 85)
	reqBody := `{"tempo_segundos": 720, "pse": 9, "fonte": "manual", "observacoes": "Bom teste"}`
	req, _ := http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos/1/vdot", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("POST /vdot expected status 201, got %d. Body: %s", w.Code, w.Body.String())
	}

	var test1 domain.Teste3km
	if err := json.Unmarshal(w.Body.Bytes(), &test1); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if test1.ID == 0 || test1.VDOT != 47.9 || test1.FTPPaceSegundos != 254 {
		t.Errorf("unexpected created test data: %+v", test1)
	}
	if test1.IndiceConfianca == nil || *test1.IndiceConfianca != 85 {
		t.Errorf("expected IndiceConfianca to be 85 for PSE 9, got %+v", test1.IndiceConfianca)
	}

	// 2. Test POST with moderate PSE = 6 -> Confidence = 70
	reqBodyModPSE := `{"tempo_segundos": 900, "pse": 6}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos/1/vdot", bytes.NewBufferString(reqBodyModPSE))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var test2 domain.Teste3km
	_ = json.Unmarshal(w.Body.Bytes(), &test2)
	if test2.IndiceConfianca == nil || *test2.IndiceConfianca != 70 {
		t.Errorf("expected IndiceConfianca to be 70 for PSE 6, got %+v", test2.IndiceConfianca)
	}

	// 3. Test POST with out-of-bounds PSE = 15 -> should be set to nil and yield Confidence = 50
	reqBodyInvalidPSE := `{"tempo_segundos": 600, "pse": 15}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos/1/vdot", bytes.NewBufferString(reqBodyInvalidPSE))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var test3 domain.Teste3km
	_ = json.Unmarshal(w.Body.Bytes(), &test3)
	if test3.PSE != nil {
		t.Errorf("expected PSE to be nil for out of bounds value 15, got %d", *test3.PSE)
	}
	if test3.IndiceConfianca == nil || *test3.IndiceConfianca != 50 {
		t.Errorf("expected IndiceConfianca to be 50 for invalid/nil PSE, got %+v", test3.IndiceConfianca)
	}

	// 4. Test POST with extremely long observations -> should be sliced to 500 chars
	longObs := strings.Repeat("A", 600)
	reqBodyLongObs := `{"tempo_segundos": 720, "observacoes": "` + longObs + `"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos/1/vdot", bytes.NewBufferString(reqBodyLongObs))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var test4 domain.Teste3km
	_ = json.Unmarshal(w.Body.Bytes(), &test4)
	if len(test4.Observacoes) != 500 {
		t.Errorf("expected observacoes to be sliced to 500 characters, got length %d", len(test4.Observacoes))
	}

	// 5. Test JSON Error Response (Invalid time seconds range)
	reqBodyInvalidTime := `{"tempo_segundos": 120}` // 2 minutes, too fast
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/alunos/1/vdot", bytes.NewBufferString(reqBodyInvalidTime))
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /vdot invalid time expected status 400, got %d", w.Code)
	}
	var errResp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("expected JSON error response, got error decoding: %v. Body: %s", err, w.Body.String())
	}
	if errResp["error"] == "" {
		t.Errorf("expected non-empty JSON error message, got: %+v", errResp)
	}

	// 6. Test Cross-Student Deletion Prevention
	// Aluno 2 trying to delete Aluno 1's test1 ID -> should return 404 Not Found
	req, _ = http.NewRequestWithContext(ctx, "DELETE", "/api/v1/alunos/2/vdot/"+strconv.FormatInt(test1.ID, 10), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found when cross-deleting test, got %d", w.Code)
	}

	// Verify test1 was NOT deleted
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos/1/vdot", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var tests []*domain.Teste3km
	_ = json.Unmarshal(w.Body.Bytes(), &tests)
	found := false
	for _, t := range tests {
		if t.ID == test1.ID {
			found = true
		}
	}
	if !found {
		t.Error("test was incorrectly deleted during unauthorized cross-student DELETE attempt")
	}

	// 7. Normal Authorized Deletion (Aluno 1 deleting its own test1)
	req, _ = http.NewRequestWithContext(ctx, "DELETE", "/api/v1/alunos/1/vdot/"+strconv.FormatInt(test1.ID, 10), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("DELETE expected status 204, got %d", w.Code)
	}

	// Verify test1 is now deleted
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/alunos/1/vdot", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var testsAfter []*domain.Teste3km
	_ = json.Unmarshal(w.Body.Bytes(), &testsAfter)
	foundAfter := false
	for _, t := range testsAfter {
		if t.ID == test1.ID {
			foundAfter = true
		}
	}
	if foundAfter {
		t.Error("test was not deleted after authorized DELETE request")
	}
}
