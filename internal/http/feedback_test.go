package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"staff_app/internal/config"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func TestFeedbackHTTPHandler(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-feedback-test-*")
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

	// Insert student 1
	_, err = db.ExecContext(ctx, "INSERT INTO alunos (id, nome, idade, sexo, email) VALUES (1, 'Carlos Aluno', 25, 'M', 'carlos@test.com')")
	if err != nil {
		t.Fatalf("failed to insert student: %v", err)
	}

	// Insert legacy training plan
	_, err = db.ExecContext(ctx, "INSERT INTO fichas_treino_web (id, aluno, ficha_json) VALUES (10, 'Carlos Aluno', '{\"frequencia\": 3}')")
	if err != nil {
		t.Fatalf("failed to insert legacy training plan: %v", err)
	}

	// Insert legacy monolithic training plan row
	_, err = db.ExecContext(ctx, "INSERT INTO fichas (id, aluno_id, feedback_rating) VALUES (10, 1, 0)")
	if err != nil {
		t.Fatalf("failed to insert legacy monolithic training plan: %v", err)
	}

	// Create active public link
	expiration := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_web (hash, ficha_id, aluno_id, conteudo_json, expira_em, ativo, acessos)
		VALUES ('hash123', 10, 1, '{}', ?, 1, 0)
	`, expiration)
	if err != nil {
		t.Fatalf("failed to insert public link: %v", err)
	}

	// Create expired public link
	pastExpiration := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_web (hash, ficha_id, aluno_id, conteudo_json, expira_em, ativo, acessos)
		VALUES ('hashExpired', 10, 1, '{}', ?, 1, 0)
	`, pastExpiration)
	if err != nil {
		t.Fatalf("failed to insert expired public link: %v", err)
	}

	cfg := &config.Config{CorsOrigins: []string{"*"}}
	router := NewRouter(cfg, db)
	authHeader := testAuthHeader(t, db, cfg)

	// 1. GET /api/v1/feedback/{hash} - Verification (no feedback submitted yet)
	req, _ := http.NewRequestWithContext(ctx, "GET", "/api/v1/feedback/hash123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var verify1 VerifyResponse
	_ = json.Unmarshal(w.Body.Bytes(), &verify1)
	if verify1.HasFeedback {
		t.Errorf("expected has_feedback to be false initially")
	}

	// 2. POST /api/v1/feedback/{hash} - Invalid rating payload
	invalidPayload := `{"rating": 6, "comentario": "Invalido"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/feedback/hash123", bytes.NewBufferString(invalidPayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Code)
	}

	// 3. POST /api/v1/feedback/{hash} - Success Submission
	payload := `{"rating": 4, "comentario": "Gostei do treino"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/feedback/hash123", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var submitResp SubmitFeedbackResponse
	_ = json.Unmarshal(w.Body.Bytes(), &submitResp)
	if submitResp.FeedbackID == 0 || submitResp.Rating != 4 || submitResp.Message != "Feedback salvo com sucesso" {
		t.Errorf("unexpected submission response: %+v", submitResp)
	}

	// 4. POST /api/v1/feedback/{hash} - Conflict (already submitted)
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/feedback/hash123", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d. Body: %s", w.Code, w.Body.String())
	}

	var conflictMap map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &conflictMap)
	if conflictMap["error"] != "Feedback já enviado" || conflictMap["rating_anterior"].(float64) != 4 {
		t.Errorf("unexpected conflict response details: %+v", conflictMap)
	}

	// 6. GET /api/v1/feedback/{hash} - Verification (now has feedback)
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/feedback/hash123", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var verify2 VerifyResponse
	_ = json.Unmarshal(w.Body.Bytes(), &verify2)
	if !verify2.HasFeedback || verify2.Rating != 4 || verify2.Comentario != "Gostei do treino" || verify2.CreatedAt == "" {
		t.Errorf("unexpected verify details after submission: %+v", verify2)
	}

	// 7. GET /api/v1/feedback/pendentes - Retrieve pending feedbacks
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/feedback/pendentes", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for pending list, got %d", w.Code)
	}

	var pendingResp PendingFeedbacksResponse
	_ = json.Unmarshal(w.Body.Bytes(), &pendingResp)
	if pendingResp.Total != 1 || len(pendingResp.Feedbacks) != 1 {
		t.Fatalf("expected 1 pending feedback, got total: %d", pendingResp.Total)
	}

	fbPending := pendingResp.Feedbacks[0]
	if fbPending.AlunoNome != "Carlos Aluno" || fbPending.NotificacaoID == 0 {
		t.Errorf("unexpected pending feedback item details: %+v", fbPending)
	}

	// 8. POST /api/v1/feedback/notificacao/{notificacao_id}/marcar-lido - Mark notification as read
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/feedback/notificacao/1/marcar-lido", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for mark read, got %d. Body: %s", w.Code, w.Body.String())
	}

	var readResp MarkReadResponse
	_ = json.Unmarshal(w.Body.Bytes(), &readResp)
	if readResp.Message != "Notificação marcada como lida" {
		t.Errorf("expected notification read message confirmation, got: %q", readResp.Message)
	}

	// Confirm pending count is now 0
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/feedback/pendentes", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var pendingResp2 PendingFeedbacksResponse
	_ = json.Unmarshal(w.Body.Bytes(), &pendingResp2)
	if pendingResp2.Total != 0 {
		t.Errorf("expected 0 pending feedbacks after read mark, got %d", pendingResp2.Total)
	}

	// Mark non-existent notification read -> 404 Not Found
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/feedback/notificacao/99999/marcar-lido", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent notification mark read, got %d. Body: %s", w.Code, w.Body.String())
	}
}
