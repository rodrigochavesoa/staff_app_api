package http

import (
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

func TestMeFichasTreino(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "http-me-fichas-treino-test-*")
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
		SecretKey:   "test-secret-me-fichas-treino",
	}
	adminHeader := testAuthHeader(t, db, cfg)
	router := NewRouter(cfg, depsForTestDB(db))

	userRepo := sqlite.NewUserRepository(db)
	alunoRepo := sqlite.NewAlunoRepository(db)

	linkedUser, linkedToken := createPortalUser(t, userRepo, cfg, "treino-aluno", "treino.aluno@example.com", false)
	usuarioID := linkedUser.ID
	linkedAluno := &domain.Aluno{
		Nome:      "Aluno Treino Portal",
		Idade:     27,
		Sexo:      "M",
		Email:     "aluno.treino@example.com",
		UsuarioID: &usuarioID,
		Ativo:     true,
	}
	if err := alunoRepo.Create(t.Context(), linkedAluno); err != nil {
		t.Fatalf("failed to create linked aluno: %v", err)
	}

	_, unlinkedToken := createPortalUser(t, userRepo, cfg, "treino-sem-vinculo", "treino.sem@example.com", false)

	t.Run("list_empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/fichas-treino", nil)
		req.Header.Set("Authorization", "Bearer "+linkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		var resp struct {
			AlunoID      int64                        `json:"aluno_id"`
			Total        int                          `json:"total"`
			FichasTreino []*domain.FichaTreinoListItem `json:"fichas_treino"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.AlunoID != linkedAluno.ID || resp.Total != 0 || len(resp.FichasTreino) != 0 {
			t.Fatalf("unexpected empty list payload: %+v", resp)
		}
	})

	ownID, otherID := seedFichasTreinoForPortal(t, db, linkedAluno.Nome)

	t.Run("list_with_data_no_ficha_json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/fichas-treino", nil)
		req.Header.Set("Authorization", "Bearer "+linkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		raw := w.Body.String()
		if strings.Contains(raw, `"ficha_json"`) {
			t.Fatalf("list must not include ficha_json: %s", raw)
		}

		var resp struct {
			AlunoID      int64                        `json:"aluno_id"`
			Total        int                          `json:"total"`
			FichasTreino []*domain.FichaTreinoListItem `json:"fichas_treino"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Total != 1 || len(resp.FichasTreino) != 1 {
			t.Fatalf("expected 1 active own ficha, got %+v", resp)
		}
		if resp.FichasTreino[0].ID != ownID {
			t.Fatalf("expected own ficha id %d, got %d", ownID, resp.FichasTreino[0].ID)
		}
		if resp.FichasTreino[0].TipoFicha != "manual" || resp.FichasTreino[0].Objetivo != "Hipertrofia" {
			t.Fatalf("unexpected list item: %+v", resp.FichasTreino[0])
		}
	})

	t.Run("get_own_id_200", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/me/fichas-treino/%d", ownID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+linkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		var resp struct {
			Status string `json:"status"`
			Data   struct {
				ID    int64  `json:"id"`
				Aluno string `json:"aluno"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.Status != "success" || resp.Data.ID != ownID || resp.Data.Aluno != linkedAluno.Nome {
			t.Fatalf("unexpected detail: %+v", resp)
		}
	})

	t.Run("get_other_aluno_id_404", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/me/fichas-treino/%d", otherID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+linkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("unlinked_user_404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/fichas-treino", nil)
		req.Header.Set("Authorization", "Bearer "+unlinkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("staff_get_other_ficha_403_for_non_admin", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/fichas/%d", otherID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+linkedToken)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	t.Run("admin_get_any_ficha_200", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/fichas/%d", otherID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", adminHeader)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for admin, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

func seedFichasTreinoForPortal(t *testing.T, db *sqlite.DB, ownNome string) (ownID, otherID int64) {
	t.Helper()
	insertSQL := `
		INSERT INTO fichas_treino_web (
			aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal,
			duracao_treino, restricoes, feedback, turma, lista_exercicios, data_criacao,
			ficha_json, tipo_ficha, num_treinos, versao, data_arquivamento
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	res, err := db.ExecContext(t.Context(), insertSQL,
		ownNome, 27, "M", "Hipertrofia", "musculacao", "intermediario", 4,
		60, "", "", "A", "[]", "2026-06-15 10:00:00",
		`{"secreto":"nao-listar"}`, "manual", 3, 1, nil,
	)
	if err != nil {
		t.Fatalf("failed to insert own ficha: %v", err)
	}
	ownID, err = res.LastInsertId()
	if err != nil {
		t.Fatalf("failed to get own ficha id: %v", err)
	}

	_, err = db.ExecContext(t.Context(), insertSQL,
		ownNome, 27, "M", "Arquivada", "musculacao", "intermediario", 3,
		60, "", "", "A", "[]", "2026-01-01 10:00:00",
		`{}`, "manual", 2, 1, "2026-02-01 10:00:00",
	)
	if err != nil {
		t.Fatalf("failed to insert archived ficha: %v", err)
	}

	res, err = db.ExecContext(t.Context(), insertSQL,
		"Outro Aluno Treino", 30, "F", "Emagrecimento", "musculacao", "iniciante", 3,
		45, "", "", "B", "[]", "2026-06-20 10:00:00",
		`{"outro":true}`, "manual", 2, 1, nil,
	)
	if err != nil {
		t.Fatalf("failed to insert other ficha: %v", err)
	}
	otherID, err = res.LastInsertId()
	if err != nil {
		t.Fatalf("failed to get other ficha id: %v", err)
	}
	return ownID, otherID
}
