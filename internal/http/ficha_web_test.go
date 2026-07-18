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

func TestFichaWebEndpointsLegacyContracts(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-ficha-web-test-*")
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
	_, err = db.ExecContext(ctx, "INSERT INTO fichas_treino_web (id, aluno, ficha_json) VALUES (100, 'Carlos Aluno', '{\"frequencia\": 3}')")
	if err != nil {
		t.Fatalf("failed to insert legacy training plan: %v", err)
	}

	cfg := &config.Config{CorsOrigins: []string{"*"}}
	router := NewRouter(cfg, db)
	authHeader := testAuthHeader(t, db, cfg)

	// 0. POST /api/v1/criar-ficha - Failure (providing invalid array "conteudo" payload)
	invalidPayload := `{
		"ficha_id": 100,
		"aluno_id": 1,
		"conteudo": [1, 2, 3]
	}`
	req, _ := http.NewRequestWithContext(ctx, "POST", "/api/v1/criar-ficha", bytes.NewBufferString(invalidPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for array payload, got %d. Body: %s", w.Code, w.Body.String())
	}

	// 1. POST /api/v1/criar-ficha - Success (providing custom "conteudo" payload)
	payload := `{
		"ficha_id": 100,
		"aluno_id": 1,
		"conteudo": {"musculos": ["peito", "triceps"], "frequencia": 3}
	}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/criar-ficha", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var created CreateFichaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode response: %v. Body: %s", err, w.Body.String())
	}

	if created.Hash == "" || created.Url == "" || created.UrlCompleta == "" || created.DiasValidade != 30 {
		t.Errorf("unexpected created link details: %+v", created)
	}

	// Parse initial expiration time
	oldExp, err := time.Parse(time.RFC3339, created.ExpiraEm)
	if err != nil {
		t.Fatalf("failed to parse initial expiration time: %v", err)
	}

	// 2. GET /api/v1/ficha/{hash}/json - Success (verify parsed content structure and metadata nested details)
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/ficha/"+created.Hash+"/json", nil)
	req.Header.Set("User-Agent", "TestChrome")
	req.RemoteAddr = "192.168.1.50"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var publicResp FichaPublicaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &publicResp); err != nil {
		t.Fatalf("failed to decode public link response: %v. Body: %s", err, w.Body.String())
	}

	if publicResp.Hash != created.Hash {
		t.Errorf("expected hash %q, got %q", created.Hash, publicResp.Hash)
	}

	// Check that Content is parsed as a JSON map/object, not a raw string
	var contentMap map[string]any
	if err := json.Unmarshal(publicResp.Conteudo, &contentMap); err != nil {
		t.Fatalf("failed to unmarshal parsed content mapping: %v", err)
	}
	if contentMap["frequencia"].(float64) != 3 {
		t.Errorf("expected frequency to be 3, got %v", contentMap["frequencia"])
	}

	// Check metadata and nested student details (must include age/sex)
	if publicResp.Metadata.Acessos != 1 || publicResp.Metadata.Aluno.ID != 1 || publicResp.Metadata.Aluno.Nome != "Carlos Aluno" {
		t.Errorf("unexpected metadata details: %+v", publicResp.Metadata)
	}
	if publicResp.Metadata.Aluno.Idade != 25 || publicResp.Metadata.Aluno.Sexo != "M" {
		t.Errorf("missing age/sex in student metadata: %+v", publicResp.Metadata.Aluno)
	}

	// 3. POST /api/v1/renovar/{hash} - Success (passing optional "dias" in payload, testing renewal antecipada math)
	renewPayload := `{"dias": 15}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/renovar/"+created.Hash, bytes.NewBufferString(renewPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for renew, got %d. Body: %s", w.Code, w.Body.String())
	}

	var renewResp RenewFichaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &renewResp); err != nil {
		t.Fatalf("failed to decode renew response: %v. Body: %s", err, w.Body.String())
	}

	if renewResp.Hash != created.Hash || renewResp.DiasAdicionados != 15 || renewResp.ExpiraEm == "" {
		t.Errorf("unexpected renew details: %+v", renewResp)
	}

	// Verify renewal math: oldExp + 15 days = newExp
	newExp, err := time.Parse(time.RFC3339, renewResp.ExpiraEm)
	if err != nil {
		t.Fatalf("failed to parse new expiration time: %v", err)
	}
	diffHours := newExp.Sub(oldExp).Hours()
	if int(diffHours) != 15*24 {
		t.Errorf("expected exactly 15 days (360 hours) added to old expiration, got difference of %f hours", diffHours)
	}

	// 4. GET /api/v1/aluno/{aluno_id}/fichas - List before deactivation (default include_expired=false)
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/aluno/1/fichas", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var listResp1 AlunoFichasResponse
	_ = json.Unmarshal(w.Body.Bytes(), &listResp1)
	if listResp1.AlunoID != 1 || listResp1.Total != 1 || len(listResp1.Fichas) != 1 {
		t.Errorf("expected 1 active link for student, got: %+v", listResp1)
	}

	// 5. POST /api/v1/desativar/{hash} - Success (returns JSON body with aligned message)
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/desativar/"+created.Hash, nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for deactivate, got %d. Body: %s", w.Code, w.Body.String())
	}

	var deactivateResp DeactivateFichaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &deactivateResp); err != nil {
		t.Fatalf("failed to decode deactivate response: %v. Body: %s", err, w.Body.String())
	}

	if deactivateResp.Hash != created.Hash || deactivateResp.Message != "Ficha desativada com sucesso" {
		t.Errorf("unexpected deactivate details: %+v", deactivateResp)
	}

	// 6. GET /api/v1/aluno/{aluno_id}/fichas - List after deactivation (default include_expired=false) -> should return 0 links
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/aluno/1/fichas", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var listResp2 AlunoFichasResponse
	_ = json.Unmarshal(w.Body.Bytes(), &listResp2)
	if listResp2.Total != 0 || len(listResp2.Fichas) != 0 {
		t.Errorf("expected 0 active links after soft deactivation, got: %+v", listResp2)
	}

	// 7. GET /api/v1/aluno/{aluno_id}/fichas - List with include_expired=true -> should return the deactivated link
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/aluno/1/fichas?include_expired=true", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var listResp3 AlunoFichasResponse
	_ = json.Unmarshal(w.Body.Bytes(), &listResp3)
	if listResp3.Total != 1 || len(listResp3.Fichas) != 1 {
		t.Errorf("expected 1 link when include_expired=true, got: %+v", listResp3)
	}

	// 8. Test GET /api/v1/ficha/{hash}/treino/{letra}
	// Let's create an active link with treinos content first
	payloadLetra := `{
		"ficha_id": 100,
		"aluno_id": 1,
		"conteudo": {
			"frequencia_semanal": 2,
			"treinos": {
				"A": {"nome_treino": "Treino A Superior"},
				"B": {"nome_treino": "Treino B Inferior"}
			}
		}
	}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/criar-ficha", bytes.NewBufferString(payloadLetra))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create ficha for letter test: %v", w.Body.String())
	}
	var createdLetra CreateFichaResponse
	_ = json.Unmarshal(w.Body.Bytes(), &createdLetra)

	// Hit public route GET /api/v1/ficha/{hash}/treino/A - Success
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/ficha/"+createdLetra.Hash+"/treino/A", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK for letter A, got %d. Body: %s", w.Code, w.Body.String())
	}

	var letraResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &letraResp)
	if letraResp["letra"] != "A" {
		t.Errorf("expected letra A in response, got %+v", letraResp)
	}
	treinoContent, ok := letraResp["treino"].(map[string]any)
	if !ok || treinoContent["nome_treino"] != "Treino A Superior" {
		t.Errorf("unexpected training details in response: %+v", letraResp)
	}

	// Hit public route GET /api/v1/ficha/{hash}/treino/C - Failure 404
	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/ficha/"+createdLetra.Hash+"/treino/C", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for non-existing letter C, got %d", w.Code)
	}

	// Periodizada snapshots store treinos as an array with letra fields.
	payloadArray := `{
		"ficha_id": 100,
		"aluno_id": 1,
		"conteudo": {
			"tipo": "periodizada",
			"treinos": [
				{"letra":"A","nome":"A - Push","exercicios":[]},
				{"letra":"B","nome":"B - Pull","exercicios":[]}
			]
		}
	}`
	req, _ = http.NewRequestWithContext(ctx, "POST", "/api/v1/criar-ficha", bytes.NewBufferString(payloadArray))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create array-form ficha for letter test: %v", w.Body.String())
	}
	var createdArray CreateFichaResponse
	_ = json.Unmarshal(w.Body.Bytes(), &createdArray)

	req, _ = http.NewRequestWithContext(ctx, "GET", "/api/v1/ficha/"+createdArray.Hash+"/treino/B", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for array-form letter B, got %d body=%s", w.Code, w.Body.String())
	}
	var arrayLetraResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &arrayLetraResp)
	treinoB, _ := arrayLetraResp["treino"].(map[string]any)
	if arrayLetraResp["letra"] != "B" || treinoB["nome"] != "B - Pull" {
		t.Fatalf("unexpected array-form letter response: %+v", arrayLetraResp)
	}
}
