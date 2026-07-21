package http

import (
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

func TestRelatoriosFlow(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-relatorios-test-*")
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

	router := NewRouter(cfg, depsForTestDB(db))

	// Seed Plan
	_, err = db.ExecContext(ctx, "INSERT INTO planos (id, nome, preco_default, ativo) VALUES (1, 'Plano Premium', 299.00, 1)")
	if err != nil {
		t.Fatalf("failed to seed plans: %v", err)
	}

	// Seed Student
	_, err = db.ExecContext(ctx, `
		INSERT INTO alunos (id, nome, idade, sexo, email, ativo, plano_id, plano_ativo, turma)
		VALUES (10, 'John Doe', 30, 'M', 'john@example.com', 1, 1, 1, 'Turma Beta')
	`)
	if err != nil {
		t.Fatalf("failed to seed student: %v", err)
	}

	// Seed Anamnese with Pathology "Lombalgia"
	_, err = db.ExecContext(ctx, `
		INSERT INTO anamneses (id, aluno_id, status_aprovacao, patologias, ativa)
		VALUES (100, 10, 'aprovada', 'Lombalgia, Cardiopatia', 1)
	`)
	if err != nil {
		t.Fatalf("failed to seed anamnese: %v", err)
	}

	// Seed Exercises
	_, err = db.ExecContext(ctx, `
		INSERT INTO exercicios_reabilitacao (codigo, nome, categoria, indicacoes, status)
		VALUES (5010, 'Alongamento Lombar', 'terapeutico', 'Lombalgia', 'ativo'),
		       (5020, 'Fortalecimento Abdominal', 'terapeutico', 'Lombalgia', 'ativo'),
		       (6010, 'Supino Reto', 'normal', 'N/A', 'ativo')
	`)
	if err != nil {
		t.Fatalf("failed to seed exercises: %v", err)
	}

	// Seed Ficha with the target exercises
	_, err = db.ExecContext(ctx, `
		INSERT INTO fichas_treino_web (
			id, aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal,
			duracao_treino, restricoes, feedback, turma, lista_exercicios, data_criacao,
			ficha_json, tipo_ficha, num_treinos, versao, ies_score, volume_sved, densidade, tut_total
		) VALUES (
			50, 'John Doe', 30, 'M', 'Hipertrofia', 'Musculação', 'Intermediário', 3,
			60, 'Nenhuma', 'Feedback', 'Turma Beta', 'exercicios_com_grupos', '2026-07-17 10:00:00',
			'{"exercicios":[{"nome":"Alongamento Lombar","grupo_muscular":"Lombar","series":3,"repeticoes":"10","cadencia":"4010","descanso":60,"rir":2,"bloco":"principal"}]}',
			'manual', 1, 1, 33.3, 128, 2.5, 150
		)
	`)
	if err != nil {
		t.Fatalf("failed to seed training sheet: %v", err)
	}

	// Seed Suggestions in sugestoes_exercicios_rehab
	_, err = db.ExecContext(ctx, `
		INSERT INTO sugestoes_exercicios_rehab (
			id, nome_exercicio, tipo_exercicio, nivel_prioridade, frequencia_sugestao,
			justificativa_clinica, status, data_sugestao
		) VALUES (
			200, 'Alongamento Lombar', 'Alongamento', 1, 3, 'Alívio lombar', 'aprovado', '2026-07-17 11:00:00'
		), (
			201, 'Fortalecimento Abdominal', 'Fortalecimento', 2, 2, 'Core stability', 'pendente', '2026-07-17 11:05:00'
		), (
			202, 'Crucifixo Inverso', 'Fortalecimento', 3, 1, 'Upper back', 'rejeitado', '2026-07-17 11:10:00'
		)
	`)
	if err != nil {
		t.Fatalf("failed to seed suggestions: %v", err)
	}

	// Generate Admin JWT token (reports are under r.Use(AdminOnly))
	authHeader := testAuthHeader(t, db, cfg)

	// 1. Test GET /api/v1/admin/relatorios/dashboard
	t.Run("Get Dashboard Resumo", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/relatorios/dashboard", nil)
		req.Header.Set("Authorization", authHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resumo domain.RelatoriosDashboardResumo
		if err := json.NewDecoder(rr.Body).Decode(&resumo); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resumo.TotalExerciciosAtivos != 3 {
			t.Errorf("expected 3 active exercises, got %d", resumo.TotalExerciciosAtivos)
		}
		if resumo.SugestoesPendentes != 1 {
			t.Errorf("expected 1 pending suggestion, got %d", resumo.SugestoesPendentes)
		}
	})

	// 2. Test GET /api/v1/admin/relatorios/patologias
	t.Run("Get Patologias Coverage", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/relatorios/patologias", nil)
		req.Header.Set("Authorization", authHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var list []domain.RelatorioPatologiaItem
		if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if len(list) < 2 {
			t.Errorf("expected at least 2 pathologies (Lombalgia and Cardiopatia), got %d", len(list))
		}
	})

	// 3. Test GET /api/v1/admin/relatorios/subutilizados
	t.Run("Get Exercicios Subutilizados", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/relatorios/subutilizados?min_recomendacoes=1", nil)
		req.Header.Set("Authorization", authHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var list []domain.ExercicioSubutilizadoItem
		if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if len(list) == 0 {
			t.Error("expected at least 1 subutilized exercise, got 0")
		}
	})

	// 4. Test GET /api/v1/admin/relatorios/aprovacao
	t.Run("Get Relatorio Aprovacao", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/relatorios/aprovacao?dias=30", nil)
		req.Header.Set("Authorization", authHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var list []domain.RelatorioAprovacaoItem
		if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}

		if len(list) != 3 {
			t.Errorf("expected 3 suggestions grouped, got %d", len(list))
		}
	})

	// 5. Test Access Denied for non-admin user (without is_admin privilege)
	t.Run("Access Denied for Non-Admin User", func(t *testing.T) {
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

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/relatorios/dashboard", nil)
		req.Header.Set("Authorization", nonAdminAuthHeader)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})
}
