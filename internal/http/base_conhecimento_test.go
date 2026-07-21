package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type mockEmbeddingProvider struct {
	embeddings []float32
	err        error
	called     int
}

func (m *mockEmbeddingProvider) GenerateEmbeddings(ctx context.Context, text string) ([]float32, error) {
	m.called++
	return m.embeddings, m.err
}

type mockVectorStore struct {
	docs   []domain.KnowledgeDocument
	err    error
	called int
}

func (m *mockVectorStore) SearchSimilar(ctx context.Context, vector []float32, k int) ([]domain.KnowledgeDocument, error) {
	m.called++
	return m.docs, m.err
}

func TestBaseConhecimentoFlow(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-knowledge-test-*")
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
	cfg := &config.Config{
		SecretKey:   "super-secret-key-for-test-purposes",
		CorsOrigins: []string{"*"},
	}

	// 1. Setup mock providers
	mockEmbed := &mockEmbeddingProvider{embeddings: []float32{0.1, 0.2, 0.3}}
	mockStore := &mockVectorStore{
		docs: []domain.KnowledgeDocument{
			{Rank: 1, Fonte: "Diretriz 2024", Conteudo: "Lombalgia crônica exige fortalecimento de core.", Tags: []string{"lombalgia", "core"}, Relevancia: 0.95},
		},
	}

	router := NewRouter(cfg, depsForTestDB(db), WithRAGProviders(mockEmbed, mockStore))
	ragRepo := sqlite.NewRAGRepository(db)

	// Seeding admin token
	adminAuthHeader := testAuthHeader(t, db, cfg)

	// Seeding non-admin user
	nonAdminPasswordHash, err := bcrypt.GenerateFromPassword([]byte("nonadmin-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	nonAdminUser := &domain.User{
		Username:     "regular_user",
		Email:        "user@example.com",
		PasswordHash: string(nonAdminPasswordHash),
		NomeCompleto: "Regular User",
		IsAdmin:      false,
		Ativo:        true,
		Aprovado:     true,
	}
	if err := sqlite.NewUserRepository(db).Create(ctx, nonAdminUser); err != nil {
		t.Fatalf("failed to create non-admin user: %v", err)
	}
	nonAdminToken, err := NewAuthHandler(sqlite.NewUserRepository(db), nil, cfg.SecretKey).signToken(nonAdminUser, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to sign non-admin token: %v", err)
	}
	nonAdminAuthHeader := "Bearer " + nonAdminToken

	// A. Test Access Control
	t.Run("Security - Block Unauthenticated", func(t *testing.T) {
		reqBody := `{"query":"Fortalecimento core","k":3}`
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/consulta-base", bytes.NewBufferString(reqBody))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rr.Code)
		}
	})

	t.Run("Security - Block Non-Admin", func(t *testing.T) {
		reqBody := `{"query":"Fortalecimento core","k":3}`
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/consulta-base", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", nonAdminAuthHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", rr.Code)
		}
	})

	// B. Test Caching (Hit and Miss)
	t.Run("Cache Miss - Hits Provider", func(t *testing.T) {
		mockEmbed.called = 0
		mockStore.called = 0

		reqBody := `{"query":"Alongamento lombar","k":3,"modalidade":"Musculação","objetivo":"Prevenção","perfil":"Iniciante"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/consulta-base", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", adminAuthHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var res map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if res["from_cache"].(bool) {
			t.Error("expected from_cache to be false")
		}
		if mockEmbed.called != 1 || mockStore.called != 1 {
			t.Errorf("expected provider called once, got embed=%d store=%d", mockEmbed.called, mockStore.called)
		}
	})

	t.Run("Cache Hit - Does Not Call Provider", func(t *testing.T) {
		mockEmbed.called = 0
		mockStore.called = 0

		reqBody := `{"query":"Alongamento lombar","k":3,"modalidade":"Musculação","objetivo":"Prevenção","perfil":"Iniciante"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/consulta-base", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", adminAuthHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var res map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !res["from_cache"].(bool) {
			t.Error("expected from_cache to be true")
		}
		if res["cache_hits"].(float64) != 2 {
			t.Errorf("expected cache_hits to be 2, got %v", res["cache_hits"])
		}
		if mockEmbed.called != 0 || mockStore.called != 0 {
			t.Errorf("expected provider NOT called (cache hit), got embed=%d store=%d", mockEmbed.called, mockStore.called)
		}
	})

	// C. Test Compound Cache Key composition
	t.Run("Cache Compound Key - Same Query Different K", func(t *testing.T) {
		mockEmbed.called = 0
		mockStore.called = 0

		// Same query "Alongamento lombar" but k=5 instead of 3
		reqBody := `{"query":"Alongamento lombar","k":5,"modalidade":"Musculação","objetivo":"Prevenção","perfil":"Iniciante"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/consulta-base", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", adminAuthHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var res map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if res["from_cache"].(bool) {
			t.Error("expected from_cache to be false because k is different")
		}
		if mockEmbed.called != 1 {
			t.Error("expected provider to be called for different cache key")
		}
	})

	// D. Test Fallback logic
	t.Run("Fallback Local Documents", func(t *testing.T) {
		// Mock provider error to trigger fallback
		mockEmbed.err = errors.New("external embedding service offline")
		mockEmbed.called = 0

		// Seed local document in SQLite base_conhecimento_documentos
		err := ragRepo.SeedLocalDocument(ctx, "Manual Local 2025", "Fortalecimento de Joelho", "Exercício de cadeira extensora isométrica reduz dor patelofemoral...", "joelho,patela", "Musculação")
		if err != nil {
			t.Fatalf("failed to seed local document: %v", err)
		}

		reqBody := `{"query":"joelho","k":3,"modalidade":"Musculação"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/consulta-base", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", adminAuthHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var res map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if res["total_resultados"].(float64) < 1 {
			t.Error("expected local document to be matched and returned")
		}
		resultados := res["resultados"].([]any)
		firstDoc := resultados[0].(map[string]any)
		if firstDoc["fonte"].(string) != "Manual Local 2025" {
			t.Errorf("expected local source, got %s", firstDoc["fonte"])
		}
		if firstDoc["relevancia"].(float64) != 1.0 {
			t.Errorf("expected local match relevance to be 1.0, got %f", firstDoc["relevancia"])
		}
	})

	t.Run("Fallback HTTP 503 - No Providers or Local Documents", func(t *testing.T) {
		mockEmbed.err = errors.New("embedding server down")

		// Querying something that doesn't exist locally (no documents match "invalido")
		reqBody := `{"query":"invalido","k":3}`
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/consulta-base", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", adminAuthHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503 Service Unavailable, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	// E. Test Telemetry / History / Popular
	t.Run("Telemetry - Historico, Estatisticas, Populares", func(t *testing.T) {
		// Verify history GET
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/consulta-base/historico?limit=5", nil)
		req.Header.Set("Authorization", adminAuthHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var histRes map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &histRes); err != nil {
			t.Fatalf("failed to decode history response: %v", err)
		}
		historico := histRes["historico"].([]any)
		if len(historico) == 0 {
			t.Error("expected history list to not be empty")
		}

		// Verify statistics GET
		req, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/consulta-base/estatisticas", nil)
		req.Header.Set("Authorization", adminAuthHeader)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var statsRes map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &statsRes); err != nil {
			t.Fatalf("failed to decode stats: %v", err)
		}
		stats := statsRes["estatisticas"].(map[string]any)
		if stats["total_hits"].(float64) == 0 {
			t.Error("expected total_hits to be greater than 0")
		}
		if stats["taxa_hit_cache"].(float64) == 0 {
			t.Error("expected cache hit rate to be calculated")
		}

		// Verify popular GET
		req, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/consulta-base/populares", nil)
		req.Header.Set("Authorization", adminAuthHeader)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})
}
