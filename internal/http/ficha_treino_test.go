package http

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func TestFichaTreinoFlow(t *testing.T) {
	logger.Setup("development", false)

	// Create temp database
	tempDir, err := os.MkdirTemp("", "http-ficha-treino-test-*")
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

	// Create a new router
	router := NewRouter(cfg, db)

	// Seed Alunos and Planos
	_, err = db.ExecContext(ctx, "INSERT INTO planos (id, nome, preco_default, ativo) VALUES (1, 'Plano Premium', 299.00, 1)")
	if err != nil {
		t.Fatalf("failed to seed plans: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO alunos (id, nome, idade, sexo, email, ativo, plano_id, plano_ativo, turma)
		VALUES (1, 'Test Student', 25, 'M', 'test@example.com', 1, 1, 1, 'Turma Alpha')
	`)
	if err != nil {
		t.Fatalf("failed to seed student: %v", err)
	}

	authHeader := testAuthHeader(t, db, cfg)

	// 1. Test POST /api/v1/fichas/manual/criar
	t.Run("Create Manual Ficha", func(t *testing.T) {
		payload := CreateManualFichaRequest{
			AlunoID:     1,
			TituloFicha: "Ficha Manual Test",
			Observacoes: "Focar em ombros",
			Exercicios: []ExercicioPrescritoRequest{
				{
					Nome:          "Desenvolvimento Halteres",
					GrupoMuscular: "Ombros",
					Series:        4,
					Repeticoes:    "10",
					Carga:         "14kg",
					Descanso:      "60s",
					Observacoes:   "Controlar descida",
				},
			},
		}

		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/fichas/manual/criar", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		status := resp["status"].(string)
		if status != "success" {
			t.Errorf("expected success status, got %s", status)
		}

		data := resp["data"].(map[string]any)
		fichaID := int64(data["id"].(float64))
		if fichaID <= 0 {
			t.Errorf("expected positive ficha id, got %d", fichaID)
		}

		// Verify database
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM fichas_treino_web WHERE id = ?", fichaID).Scan(&count)
		if err != nil || count != 1 {
			t.Errorf("expected 1 record in DB, got count %d, err %v", count, err)
		}
	})

	// 2. Test GET /api/v1/fichas/{id} and PUT /api/v1/fichas/{id}/editar-manual
	t.Run("Get and Edit Manual Ficha (with OCC and link sync)", func(t *testing.T) {
		// Insert a known sheet and a corresponding public link
		_, err := db.ExecContext(ctx, `
			INSERT INTO fichas_treino_web (id, aluno, idade, sexo, objetivo, modalidade, nivel, frequencia_semanal, duracao_treino, restricoes, feedback, turma, lista_exercicios, data_criacao, ficha_json, tipo_ficha, num_treinos, versao)
			VALUES (20, 'Test Student', 25, 'M', 'Hipertrofia', 'Manual', 'N/A', 0, 0, 'Obs Antiga', 'Ficha criada manualmente', 'Ficha 20', 'manual_custom', '2026-07-17 00:00:00', '{"exercicios":[]}', 'manual', 1, 1)
		`)
		if err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO fichas_web (id, hash, ficha_id, aluno_id, conteudo_json, criado_em, expira_em, acessos, ativo)
			VALUES (200, 'hash20', 20, 1, '{"exercicios":[]}', '2026-07-17 00:00:00', '2026-08-17 00:00:00', 0, 1)
		`)
		if err != nil {
			t.Fatalf("failed to seed public link: %v", err)
		}

		// Fetch to verify GET
		reqGet, _ := http.NewRequest(http.MethodGet, "/api/v1/fichas/20", nil)
		reqGet.Header.Set("Authorization", authHeader)
		rrGet := httptest.NewRecorder()
		router.ServeHTTP(rrGet, reqGet)

		if rrGet.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rrGet.Code)
		}

		// Edit PUT
		editPayload := EditManualFichaRequest{
			Observacoes: "Obs Nova",
			Exercicios: []ExercicioPrescritoRequest{
				{
					Nome:          "Supino Reto",
					GrupoMuscular: "Peito",
					Series:        4,
					Repeticoes:    "8",
					Carga:         "20kg",
					Descanso:      "90s",
					Observacoes:   "",
				},
			},
			ParametrosTreino: &EditFichaParams{
				Perfil:     "Avançado",
				Foco:       "Força",
				Frequencia: 3,
				Duracao:    90,
			},
			Versao: 1, // Correct version
		}

		body, _ := json.Marshal(editPayload)
		reqEdit, _ := http.NewRequest(http.MethodPut, "/api/v1/fichas/20/editar-manual", bytes.NewReader(body))
		reqEdit.Header.Set("Content-Type", "application/json")
		reqEdit.Header.Set("Authorization", authHeader)

		rrEdit := httptest.NewRecorder()
		router.ServeHTTP(rrEdit, reqEdit)

		if rrEdit.Code != http.StatusOK {
			t.Errorf("expected status 200 on edit, got %d. Body: %s", rrEdit.Code, rrEdit.Body.String())
		}

		// Verify database version is 2
		var versao int
		var restricoes, fichaJSON string
		err = db.QueryRowContext(ctx, "SELECT versao, restricoes, ficha_json FROM fichas_treino_web WHERE id = 20").Scan(&versao, &restricoes, &fichaJSON)
		if err != nil {
			t.Fatalf("failed to query updated sheet: %v", err)
		}

		if versao != 2 {
			t.Errorf("expected version 2 (incremented), got %d", versao)
		}
		if restricoes != "Obs Nova" {
			t.Errorf("expected restricoes 'Obs Nova', got %s", restricoes)
		}
		if !strings.Contains(fichaJSON, "Supino Reto") {
			t.Errorf("expected ficha_json to contain 'Supino Reto', got %s", fichaJSON)
		}

		// Verify public link content sync
		var linkJSON string
		err = db.QueryRowContext(ctx, "SELECT conteudo_json FROM fichas_web WHERE id = 200").Scan(&linkJSON)
		if err != nil {
			t.Fatalf("failed to query public link: %v", err)
		}
		if !strings.Contains(linkJSON, "Supino Reto") {
			t.Errorf("expected public link JSON to sync and contain 'Supino Reto', got %s", linkJSON)
		}

		// Test OCC Conflict: try to edit again but with the old version (1 instead of 2) via payload
		conflictPayload := editPayload
		conflictPayload.Versao = 1 // Now 1 is obsolete
		bodyConflict, _ := json.Marshal(conflictPayload)
		reqConflict, _ := http.NewRequest(http.MethodPut, "/api/v1/fichas/20/editar-manual", bytes.NewReader(bodyConflict))
		reqConflict.Header.Set("Content-Type", "application/json")
		reqConflict.Header.Set("Authorization", authHeader)
		rrConflict := httptest.NewRecorder()
		router.ServeHTTP(rrConflict, reqConflict)

		if rrConflict.Code != http.StatusConflict {
			t.Errorf("expected status 409 on obsolete payload version, got %d. Body: %s", rrConflict.Code, rrConflict.Body.String())
		}

		// Test OCC Conflict: try to edit again but with a mismatching If-Match header
		reqIfMatchConflict, _ := http.NewRequest(http.MethodPut, "/api/v1/fichas/20/editar-manual", bytes.NewReader(body))
		reqIfMatchConflict.Header.Set("Content-Type", "application/json")
		reqIfMatchConflict.Header.Set("Authorization", authHeader)
		reqIfMatchConflict.Header.Set("If-Match", "999") // Wrong version header
		rrIfMatchConflict := httptest.NewRecorder()
		router.ServeHTTP(rrIfMatchConflict, reqIfMatchConflict)

		if rrIfMatchConflict.Code != http.StatusConflict {
			t.Errorf("expected status 409 on If-Match header mismatch, got %d. Body: %s", rrIfMatchConflict.Code, rrIfMatchConflict.Body.String())
		}

		// We'll call the repository directly to simulate concurrent update conflict
		repo := sqlite.NewFichaTreinoRepository(db)
		oldFicha := &domain.FichaTreinoWeb{
			ID:         20,
			Versao:     1, // Outdated version
			Aluno:      "Test Student",
			Restricoes: "Conflict test",
		}
		err = repo.Update(ctx, oldFicha)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected OCC conflict error (sql.ErrNoRows), got: %v", err)
		}
	})

	// 3. Test DELETE /api/v1/fichas/{id}
	t.Run("Delete Ficha (Hard Delete)", func(t *testing.T) {
		// Try to delete without Header
		reqDelNoHeader, _ := http.NewRequest(http.MethodDelete, "/api/v1/fichas/20", nil)
		reqDelNoHeader.Header.Set("Authorization", authHeader)
		rrDelNoHeader := httptest.NewRecorder()
		router.ServeHTTP(rrDelNoHeader, reqDelNoHeader)

		if rrDelNoHeader.Code != http.StatusBadRequest {
			t.Errorf("expected 400 when missing confirmation header, got %d", rrDelNoHeader.Code)
		}

		// Delete with correct header
		reqDel, _ := http.NewRequest(http.MethodDelete, "/api/v1/fichas/20", nil)
		reqDel.Header.Set("Authorization", authHeader)
		reqDel.Header.Set("X-Confirm-Hard-Delete", "CONFIRMAR")
		rrDel := httptest.NewRecorder()
		router.ServeHTTP(rrDel, reqDel)

		if rrDel.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rrDel.Code)
		}

		// Verify deleted from both tables
		var countSheets, countLinks int
		_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM fichas_treino_web WHERE id = 20").Scan(&countSheets)
		_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM fichas_web WHERE id = 200").Scan(&countLinks)

		if countSheets != 0 {
			t.Errorf("expected sheet to be deleted, count: %d", countSheets)
		}
		if countLinks != 0 {
			t.Errorf("expected related public link to be deleted, count: %d", countLinks)
		}
	})

	// 4. Test POST /api/v1/fichas/gerar-periodizada
	t.Run("Generate Periodized Ficha", func(t *testing.T) {
		// Validations: Frequency out of bounds
		payloadInvalid := GerarFichaPeriodizadaRequest{
			AlunoID:    1,
			Frequencia: 7, // Invalid
			Objetivo:   "Hipertrofia",
			Nivel:      "Iniciante",
		}
		bodyInv, _ := json.Marshal(payloadInvalid)
		reqInv, _ := http.NewRequest(http.MethodPost, "/api/v1/fichas/gerar-periodizada", bytes.NewReader(bodyInv))
		reqInv.Header.Set("Content-Type", "application/json")
		reqInv.Header.Set("Authorization", authHeader)
		rrInv := httptest.NewRecorder()
		router.ServeHTTP(rrInv, reqInv)

		if rrInv.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for out of bounds frequency, got %d", rrInv.Code)
		}

		// Valid generation: frequency = 3
		payloadValid := GerarFichaPeriodizadaRequest{
			AlunoID:    1,
			Frequencia: 3,
			Objetivo:   "Hipertrofia",
			Nivel:      "Intermediário",
		}
		bodyVal, _ := json.Marshal(payloadValid)
		reqVal, _ := http.NewRequest(http.MethodPost, "/api/v1/fichas/gerar-periodizada", bytes.NewReader(bodyVal))
		reqVal.Header.Set("Content-Type", "application/json")
		reqVal.Header.Set("Authorization", authHeader)
		rrVal := httptest.NewRecorder()
		router.ServeHTTP(rrVal, reqVal)

		if rrVal.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d. Body: %s", rrVal.Code, rrVal.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rrVal.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		hashLink := resp["hash_link"].(string)
		fichaID := int64(resp["ficha_id"].(float64))
		data := resp["data"].(map[string]any)
		aiMetadata := data["ai_metadata"].(map[string]any)
		if aiMetadata["ai_used"].(bool) {
			t.Fatal("expected local deterministic generation when no AI providers are configured")
		}
		if aiMetadata["provider"].(string) != "local" {
			t.Fatalf("expected provider local, got %v", aiMetadata["provider"])
		}
		if aiMetadata["safety_validated"].(bool) != true {
			t.Fatal("expected local generation to be marked as safety_validated")
		}

		// Check public link table for 90 days validity
		var expiraEmStr string
		var ativo int
		err = db.QueryRowContext(ctx, "SELECT expira_em, ativo FROM fichas_web WHERE hash = ?", hashLink).Scan(&expiraEmStr, &ativo)
		if err != nil {
			t.Fatalf("failed to find public link: %v", err)
		}

		if ativo != 1 {
			t.Errorf("expected public link to be active")
		}

		expTime, err := time.Parse(time.RFC3339, expiraEmStr)
		if err != nil {
			expTime, err = time.Parse("2006-01-02 15:04:05", expiraEmStr)
		}
		if err != nil {
			t.Fatalf("failed to parse expiration: %v", err)
		}

		duration := time.Until(expTime)
		days := int(duration.Hours() / 24)
		if days < 88 || days > 91 {
			t.Errorf("expected ~90 days validity, got %d days (expiration: %s)", days, expiraEmStr)
		}

		// Verify archiving: generate another periodized sheet for the same student
		reqVal2, _ := http.NewRequest(http.MethodPost, "/api/v1/fichas/gerar-periodizada", bytes.NewReader(bodyVal))
		reqVal2.Header.Set("Content-Type", "application/json")
		reqVal2.Header.Set("Authorization", authHeader)
		rrVal2 := httptest.NewRecorder()
		router.ServeHTTP(rrVal2, reqVal2)

		if rrVal2.Code != http.StatusCreated {
			t.Fatalf("expected 201 on second generation, got %d", rrVal2.Code)
		}

		var resp2 map[string]any
		if err := json.Unmarshal(rrVal2.Body.Bytes(), &resp2); err != nil {
			t.Fatalf("failed to unmarshal second response: %v", err)
		}
		fichaID2 := int64(resp2["ficha_id"].(float64))

		// Verify that the second sheet has ficha_anterior_id = first sheet's ID
		var fichaAnteriorID sql.NullInt64
		err = db.QueryRowContext(ctx, "SELECT ficha_anterior_id FROM fichas_treino_web WHERE id = ?", fichaID2).Scan(&fichaAnteriorID)
		if err != nil {
			t.Fatalf("failed to query ficha_anterior_id: %v", err)
		}
		if !fichaAnteriorID.Valid || fichaAnteriorID.Int64 != fichaID {
			t.Errorf("expected second sheet to have ficha_anterior_id = %d, got %v", fichaID, fichaAnteriorID)
		}

		// Verify first sheet (fichaID) is now archived (data_arquivamento is NOT NULL)
		var dataArquivamento sql.NullString
		err = db.QueryRowContext(ctx, "SELECT data_arquivamento FROM fichas_treino_web WHERE id = ?", fichaID).Scan(&dataArquivamento)
		if err != nil {
			t.Fatalf("failed to query archived sheet: %v", err)
		}

		if !dataArquivamento.Valid {
			t.Errorf("expected first sheet to be archived (data_arquivamento not null)")
		}

		// Verify first public link is inactive
		var activeLinkCount int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM fichas_web WHERE hash = ? AND ativo = 1", hashLink).Scan(&activeLinkCount)
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}
		if activeLinkCount != 0 {
			t.Errorf("expected first public link to be deactivated, but got active count: %d", activeLinkCount)
		}
	})

	// 5. Test GET /api/v1/metodos/{metodo}
	t.Run("Get Metodos Info", func(t *testing.T) {
		// Slug version
		reqSlug, _ := http.NewRequest(http.MethodGet, "/api/v1/metodos/drop_set", nil)
		reqSlug.Header.Set("Authorization", authHeader)
		rrSlug := httptest.NewRecorder()
		router.ServeHTTP(rrSlug, reqSlug)

		if rrSlug.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rrSlug.Code)
		}

		var resp map[string]any
		_ = json.Unmarshal(rrSlug.Body.Bytes(), &resp)
		metodo := resp["metodo"].(map[string]any)
		if metodo["nome"] != "Drop-set (Série Descendente)" {
			t.Errorf("expected Drop-set, got %s", metodo["nome"])
		}

		// Portuguese version
		reqPt, _ := http.NewRequest(http.MethodGet, "/api/v1/metodos/Drop-set", nil)
		reqPt.Header.Set("Authorization", authHeader)
		rrPt := httptest.NewRecorder()
		router.ServeHTTP(rrPt, reqPt)

		if rrPt.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rrPt.Code)
		}
	})
}
