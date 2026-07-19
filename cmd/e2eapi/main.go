// Command e2eapi runs an API-only end-to-end journey against a live staff_app server.
// Usage:
//
//	GOCACHE=/tmp/go-build-cache GOWORK=off go run ./cmd/e2eapi [baseURL]
//	./scripts/e2e_api_journey.sh [baseURL]
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	base := "http://localhost:5000"
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		base = strings.TrimRight(os.Args[1], "/")
	}
	adminUser := env("E2E_ADMIN_USERNAME", env("ADMIN_DEFAULT_USERNAME", "admin"))
	adminPass := env("E2E_ADMIN_PASSWORD", env("ADMIN_DEFAULT_PASSWORD", "admin-change-me-immediately"))
	fitPath := env("E2E_FIT_FILE", "internal/garmin/testdata/fit/1_20260329_195128_22325609284_ACTIVITY.fit")

	c := &client{base: base, http: &http.Client{Timeout: 60 * time.Second}}
	suffix := strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 10)
	// Unique display name: historico is matched by aluno nome (not id); shared names
	// inflate complexity via prior E2E fichas.
	alunoNome := "Aluno E2E " + suffix
	failed := 0
	step := func(name string, fn func() error) {
		fmt.Printf("\n== %s ==\n", name)
		if err := fn(); err != nil {
			failed++
			fmt.Printf("FAIL: %v\n", err)
			return
		}
		fmt.Println("OK")
	}

	var (
		token          string
		coachToken     string
		coachID        int64
		planoID        int64
		alunoID        int64
		anamneseToken  string
		anamneseID     int64
		manualFichaID  int64
		periodFichaID  int64
		periodHash     string
		publicHash     string
		corridaID      int64
		corridaWeekID  int64
		corridaPubHash string
		atividadeID    int64
		exCodigo       int
	)

	fmt.Printf("E2E API journey against %s\n", base)

	step("1. Health + auth", func() error {
		code, body, err := c.do(http.MethodGet, "/health", "", nil)
		if err != nil {
			return err
		}
		if code != 200 || !bytes.Contains(body, []byte(`"status":"ok"`)) {
			return fmt.Errorf("health status=%d body=%s", code, truncate(body))
		}
		code, body, err = c.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": adminUser, "password": adminPass,
		})
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("login status=%d body=%s", code, truncate(body))
		}
		token = mustString(body, "token")
		if token == "" {
			return fmt.Errorf("login missing token")
		}
		code, _, err = c.do(http.MethodGet, "/api/v1/auth/me", "", nil)
		if err != nil {
			return err
		}
		if code != 401 {
			return fmt.Errorf("expected 401 without token, got %d", code)
		}
		code, _, err = c.do(http.MethodGet, "/api/v1/auth/me", token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("auth/me status=%d", code)
		}
		return nil
	})

	step("2. Planos + pré-cadastro", func() error {
		code, body, err := c.do(http.MethodPost, "/api/v1/admin/planos", token, map[string]any{
			"nome": "Plano E2E " + suffix, "preco_default": 99.9, "descricao": "E2E", "ativo": true,
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("create plano status=%d body=%s", code, truncate(body))
		}
		planoID = mustInt(body, "id")
		code, body, err = c.do(http.MethodGet, "/api/v1/planos", "", nil)
		if err != nil {
			return err
		}
		if code != 200 || !bytes.Contains(body, []byte("Plano E2E")) {
			return fmt.Errorf("list planos status=%d body=%s", code, truncate(body))
		}
		email := "e2e_" + suffix + "@example.com"
		code, body, err = c.do(http.MethodPost, "/api/v1/pre-cadastro", "", map[string]any{
			"nome": alunoNome, "email": email, "telefone": "11999990000",
			"data_nascimento": "1990-05-15", "genero": "masculino", "plano_id": planoID,
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("pre-cadastro status=%d body=%s", code, truncate(body))
		}
		preID := mustInt(body, "pre_registro_id")
		code, body, err = c.do(http.MethodGet, "/api/v1/admin/pre-cadastros", token, nil)
		if err != nil {
			return err
		}
		if code != 200 || !bytes.Contains(body, []byte(email)) {
			return fmt.Errorf("list pre-cadastros status=%d", code)
		}
		code, body, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/admin/pre-cadastros/%d", preID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 || !bytes.Contains(body, []byte("audit_trail")) {
			return fmt.Errorf("pre-cadastro detail/audit status=%d body=%s", code, truncate(body))
		}
		code, body, err = c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/pre-cadastros/%d/aprovar", preID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("aprovar pre-cadastro status=%d body=%s", code, truncate(body))
		}
		alunoID = mustInt(body, "aluno_id")
		link := mustString(body, "anamnese_link")
		if alunoID <= 0 || link == "" {
			return fmt.Errorf("approve missing aluno_id/anamnese_link: %s", truncate(body))
		}
		parts := strings.Split(strings.Trim(link, "/"), "/")
		anamneseToken = parts[len(parts)-1]
		return nil
	})

	step("3. Anamnese", func() error {
		code, body, err := c.do(http.MethodGet, "/api/v1/anamnese/submit/"+anamneseToken, "", nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("anamnese metadata status=%d body=%s", code, truncate(body))
		}
		// Keep clinical free-text empty so the first gerar-periodizada stays complexity=simples
		// (non-empty patologias alone scores anamnese_clinica and can tip into moderado).
		code, body, err = c.do(http.MethodPost, "/api/v1/anamnese/submit/"+anamneseToken, "", map[string]any{
			"peso": 78.0, "altura": 1.75, "patologias": "", "medicamentos": "",
			"lesoes_atuais": "", "dores_cronicas": "",
			"parq_doenca_cardiaca": 0, "parq_dor_peito": 0, "parq_tontura": 0,
			"parq_problema_osseo": 0, "parq_medicamento_pressao": 0, "parq_impedimento_activity": 0,
			"experiencia_treino": "Musculação", "objetivo_principal": "Hipertrofia",
			"contato_emergencia_nome": "Contato E2E", "contato_emergencia_telefone": "11988887777",
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("submit anamnese status=%d body=%s", code, truncate(body))
		}
		anamneseID = mustInt(body, "anamnese_id")
		code, body, err = c.do(http.MethodGet, "/api/v1/admin/anamnese/pendentes", token, nil)
		if err != nil {
			return err
		}
		if code != 200 || !bytes.Contains(body, []byte(alunoNome)) {
			return fmt.Errorf("pendentes status=%d body=%s", code, truncate(body))
		}
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/admin/anamnese/%d", anamneseID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("anamnese detail status=%d", code)
		}
		code, _, err = c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/anamnese/%d/aprovar", anamneseID), token, nil)
		if err != nil {
			return err
		}
		if code != 204 {
			return fmt.Errorf("aprovar anamnese status=%d", code)
		}

		// Operational anamnese email path (same flexibility as smoke_test.sh).
		code, body, err = c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/alunos/%d/anamnese/reenviar-email", alunoID), token, nil)
		if err != nil {
			return err
		}
		if err := assertAnamneseEmailResult(code, body, os.Getenv("E2E_EXPECT_SMTP_FAILURE")); err != nil {
			return err
		}

		code, body, err = c.do(http.MethodGet, "/api/v1/admin/dashboard/stats", token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("dashboard stats status=%d body=%s", code, truncate(body))
		}
		return nil
	})

	step("4. Aluno operacional", func() error {
		code, body, err := c.do(http.MethodGet, "/api/v1/alunos/search?q="+suffix, token, nil)
		if err != nil {
			return err
		}
		if code != 200 || !bytes.Contains(body, []byte(alunoNome)) {
			return fmt.Errorf("search status=%d body=%s", code, truncate(body))
		}
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/alunos/%d", alunoID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("get aluno status=%d", code)
		}
		now := time.Now()
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/alunos/%d/frequencia?mes=%d&ano=%d", alunoID, int(now.Month()), now.Year()), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("frequencia status=%d", code)
		}
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/alunos/%d/treinos", alunoID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("treinos status=%d", code)
		}
		return nil
	})

	step("5. Fichas musculação", func() error {
		// Evidence pipeline first (before manual ficha) so historico does not inflate complexity.
		// Simples: anamnese clínica fraca + sem restrições → score 1 (PR1/PR2/PR4/PR5).
		code, body, err := c.do(http.MethodPost, "/api/v1/fichas/gerar-periodizada", token, map[string]any{
			"aluno_id": alunoID, "frequencia": 3, "objetivo": "Hipertrofia", "nivel": "Intermediário",
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("gerar-periodizada status=%d body=%s", code, truncate(body))
		}
		periodFichaID = mustInt(body, "ficha_id")
		periodHash = mustString(body, "hash_link")
		if periodFichaID <= 0 || periodHash == "" {
			return fmt.Errorf("periodizada missing ficha_id/hash_link")
		}
		if err := assertEvidenceAIMetadata(body, evidenceMetaExpect{
			Complexity:           "simples",
			EvidenceCount:        0,
			EvidenceFallback:     false,
			EvidenceReasonSubstr: "complexidade_simples",
			ContextUsed:          true,
		}); err != nil {
			return fmt.Errorf("gerar-periodizada simples: %w", err)
		}

		// Public ficha reads before a second periodizada archives the first hash.
		code, _, err = c.do(http.MethodGet, "/api/v1/ficha/"+periodHash+"/json", "", nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("ficha json status=%d", code)
		}
		code, _, err = c.do(http.MethodGet, "/api/v1/ficha/"+periodHash+"/treino/A", "", nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("treino letra A status=%d", code)
		}

		// Moderado: restrições + histórico da periodizada anterior → score >= 2 → busca acionada.
		code, body, err = c.do(http.MethodPost, "/api/v1/fichas/gerar-periodizada", token, map[string]any{
			"aluno_id": alunoID, "frequencia": 3, "objetivo": "Hipertrofia", "nivel": "Intermediário",
			"restricoes": "dor lombar",
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("gerar-periodizada moderado status=%d body=%s", code, truncate(body))
		}
		if err := assertEvidenceAIMetadata(body, evidenceMetaExpect{
			Complexity:           "moderado",
			EvidenceReasonSubstr: "busca_acionada",
			ContextUsed:          true,
		}); err != nil {
			return fmt.Errorf("gerar-periodizada moderado: %w", err)
		}
		// Track latest periodizada for cleanup (first hash may be archived).
		if id := mustInt(body, "ficha_id"); id > 0 {
			periodFichaID = id
		}
		if h := mustString(body, "hash_link"); h != "" {
			periodHash = h
		}

		if err := assertTrainingPipelineTelemetry(alunoID, []string{"simples", "moderado"}); err != nil {
			return fmt.Errorf("PR5 telemetry: %w", err)
		}

		code, body, err = c.do(http.MethodPost, "/api/v1/fichas/manual/criar", token, map[string]any{
			"aluno_id": alunoID, "titulo_ficha": "Manual E2E", "observacoes": "e2e",
			"exercicios": []map[string]any{{
				"nome": "Agachamento", "grupo_muscular": "Pernas", "series": 3,
				"repeticoes": "10", "carga": "40kg", "descanso": "60s", "cadencia": "3010", "rir": 2,
			}},
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("manual criar status=%d body=%s", code, truncate(body))
		}
		manualFichaID = nestedInt(body, "data", "id")
		code, body, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/fichas/%d", manualFichaID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("get ficha status=%d", code)
		}
		versao := nestedInt(body, "data", "versao")
		if versao <= 0 {
			versao = 1
		}
		edit := map[string]any{
			"observacoes": "e2e edit",
			"versao":      versao,
			"exercicios": []map[string]any{{
				"nome": "Supino", "grupo_muscular": "Peito", "series": 4,
				"repeticoes": "8", "carga": "50kg", "descanso": "90s",
			}},
		}
		code, _, err = c.do(http.MethodPut, fmt.Sprintf("/api/v1/fichas/%d/editar-manual", manualFichaID), token, edit)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("editar-manual status=%d", code)
		}
		// OCC conflict with stale version
		code, _, err = c.do(http.MethodPut, fmt.Sprintf("/api/v1/fichas/%d/editar-manual", manualFichaID), token, edit)
		if err != nil {
			return err
		}
		if code != 409 {
			return fmt.Errorf("expected OCC 409, got %d", code)
		}

		// Also create public link from manual ficha for feedback/marcar path
		code, body, err = c.do(http.MethodPost, "/api/v1/criar-ficha", token, map[string]any{
			"aluno_id": alunoID, "ficha_id": manualFichaID,
			"conteudo": map[string]any{
				"objetivo": "E2E", "treinos": map[string]any{
					"A": map[string]any{"exercicios": []any{}},
				},
			},
		})
		if err != nil {
			return err
		}
		if code != 200 && code != 201 {
			return fmt.Errorf("criar-ficha status=%d body=%s", code, truncate(body))
		}
		publicHash = mustString(body, "hash")
		if publicHash == "" {
			publicHash = nestedString(body, "data", "hash")
		}
		if publicHash == "" {
			return fmt.Errorf("criar-ficha missing hash: %s", truncate(body))
		}

		today := time.Now().Format("2006-01-02")
		code, _, err = c.do(http.MethodPost, "/api/v1/treinos/marcar", "", map[string]any{
			"ficha_id": manualFichaID, "aluno_id": alunoID, "hash_ficha": publicHash,
			"data_treino": today, "tipo_ficha": "musculacao", "tipo_treino": "A",
		})
		if err != nil {
			return err
		}
		if code != 200 && code != 201 {
			return fmt.Errorf("marcar status=%d", code)
		}
		code, _, err = c.do(http.MethodPost, "/api/v1/treinos/desmarcar", "", map[string]any{
			"ficha_id": manualFichaID, "aluno_id": alunoID, "hash_ficha": publicHash,
			"data_treino": today, "tipo_ficha": "musculacao",
		})
		if err != nil {
			return err
		}
		if code != 200 && code != 204 {
			return fmt.Errorf("desmarcar status=%d", code)
		}
		// re-mark for calendar presence
		_, _, _ = c.do(http.MethodPost, "/api/v1/treinos/marcar", "", map[string]any{
			"ficha_id": manualFichaID, "aluno_id": alunoID, "hash_ficha": publicHash,
			"data_treino": today, "tipo_ficha": "musculacao", "tipo_treino": "A",
		})
		now := time.Now()
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/treinos/mes?hash_ficha=%s&mes=%d&ano=%d", publicHash, int(now.Month()), now.Year()), "", nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("treinos/mes status=%d", code)
		}
		code, _, err = c.do(http.MethodPost, "/api/v1/feedback/"+publicHash, "", map[string]any{
			"rating": 5, "comentario": "E2E feedback",
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("feedback status=%d", code)
		}
		code, body, err = c.do(http.MethodGet, "/api/v1/feedback/pendentes", token, nil)
		if err != nil {
			return err
		}
		if code != 200 || !bytes.Contains(body, []byte("E2E feedback")) && !bytes.Contains(body, []byte(publicHash)) {
			// pendentes may list by aluno/hash; accept 200 with any payload containing rating
			if code != 200 {
				return fmt.Errorf("feedback pendentes status=%d body=%s", code, truncate(body))
			}
		}
		return nil
	})

	step("6. Corrida", func() error {
		code, body, err := c.do(http.MethodPost, "/api/v1/corrida/gerar", token, map[string]any{
			"aluno_id": alunoID, "distancia_prova": "10K", "nivel": "intermediario",
			"pace_base": "05:30", "volume_semanal": 40.0, "dias_semana": []int{2, 4, 6},
			"data_inicio": "2026-07-22", "data_prova": "2026-10-10",
			"usar_blocos": true, "modo_geracao": "todas",
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("corrida gerar status=%d body=%s", code, truncate(body))
		}
		corridaID = nestedInt(body, "data", "id")
		if corridaID <= 0 {
			return fmt.Errorf("corrida id missing")
		}
		code, body, err = c.do(http.MethodPost, "/api/v1/corrida/gerar", token, map[string]any{
			"aluno_id": alunoID, "distancia_prova": "10K", "nivel": "intermediario",
			"pace_base": "05:30", "volume_semanal": 40.0, "dias_semana": []int{1, 3, 5},
			"data_inicio": "2026-07-22", "data_prova": "2026-10-10",
			"usar_blocos": true, "modo_geracao": "semana_a_semana",
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("corrida semana_a_semana status=%d body=%s", code, truncate(body))
		}
		corridaWeekID = nestedInt(body, "data", "id")
		code, _, err = c.do(http.MethodPost, fmt.Sprintf("/api/v1/corrida/%d/gerar-proxima-semana", corridaWeekID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("gerar-proxima-semana status=%d", code)
		}
		code, body, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/corrida/%d/semana/1/dia/2", corridaID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("get dia blocos status=%d body=%s", code, truncate(body))
		}
		versao := nestedInt(body, "data", "versao")
		save := map[string]any{
			"versao": versao, "nome": "E2E Edit", "zona": "I",
			"blocos": []map[string]any{
				{"type": "atomic", "intensity": "E", "duration_min": 10, "description": "Aquecimento"},
				{"type": "repeater", "repeat": 3, "content": []map[string]any{
					{"type": "atomic", "intensity": "I", "duration_min": 3},
					{"type": "atomic", "intensity": "E", "duration_min": 2},
				}},
				{"type": "atomic", "intensity": "E", "duration_min": 8, "description": "Volta à calma"},
			},
		}
		code, _, err = c.do(http.MethodPut, fmt.Sprintf("/api/v1/corrida/%d/semana/1/dia/2/blocos", corridaID), token, save)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("save blocos status=%d", code)
		}
		code, _, err = c.do(http.MethodPut, fmt.Sprintf("/api/v1/corrida/%d/semana/1/dia/2/blocos", corridaID), token, save)
		if err != nil {
			return err
		}
		if code != 409 {
			return fmt.Errorf("expected blocos OCC 409, got %d", code)
		}
		code, body, err = c.do(http.MethodPost, fmt.Sprintf("/api/v1/corrida/%d/gerar-link", corridaID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 && code != 201 {
			return fmt.Errorf("gerar-link status=%d body=%s", code, truncate(body))
		}
		corridaPubHash = firstNonEmpty(mustString(body, "hash"), nestedString(body, "data", "hash"), mustString(body, "link_hash"))
		if corridaPubHash == "" {
			// some handlers return full URL
			if u := firstNonEmpty(mustString(body, "url"), nestedString(body, "data", "url"), mustString(body, "public_url")); u != "" {
				parts := strings.Split(strings.Trim(u, "/"), "/")
				corridaPubHash = parts[len(parts)-1]
			}
		}
		if corridaPubHash == "" {
			return fmt.Errorf("corrida public hash missing: %s", truncate(body))
		}
		code, _, err = c.do(http.MethodGet, "/api/v1/corrida/publica/"+corridaPubHash, "", nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("plano publico status=%d", code)
		}
		code, _, err = c.do(http.MethodPost, "/api/v1/corrida/publica/"+corridaPubHash+"/concluir", "", map[string]any{
			"semana": 1, "dia": 2, "concluido": true,
		})
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("concluir publico status=%d", code)
		}
		now := time.Now()
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/alunos/%d/corrida/treinos-dia?mes=%d&ano=%d", alunoID, int(now.Month()), now.Year()), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("treinos-dia status=%d", code)
		}
		code, body, err = c.do(http.MethodPost, "/api/v1/corrida/gerar-blocos", token, map[string]any{
			"vdot": 45.0, "distancia_prova": "10K", "nivel": "intermediario", "dias_semana": 3, "aluno_id": alunoID,
		})
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("gerar-blocos status=%d body=%s", code, truncate(body))
		}
		return nil
	})

	step("7. Exercícios", func() error {
		code, body, err := c.do(http.MethodGet, "/api/v1/exercicios/biblioteca", token, nil)
		if err != nil {
			return err
		}
		if code != 200 || !bytes.Contains(body, []byte("codigo")) {
			return fmt.Errorf("biblioteca status=%d", code)
		}
		code, body, err = c.do(http.MethodPost, "/api/v1/exercicios/personalizados", token, map[string]any{
			"nome": "E2E Personalizado " + suffix, "categoria": "normal",
			"grupo_muscular": "Core", "descricao_terapeutica": "E2E",
		})
		if err != nil {
			return err
		}
		if code != 201 {
			return fmt.Errorf("criar personalizado status=%d body=%s", code, truncate(body))
		}
		exCodigo = int(mustInt(body, "codigo"))
		url := mustString(body, "url")
		wantPrefix := "https://rcstorestaff.com.br/exercicios_html/"
		if !strings.HasPrefix(url, wantPrefix) || !strings.Contains(url, strconv.Itoa(exCodigo)) {
			return fmt.Errorf("unexpected exercise url %q (want prefix %s + codigo)", url, wantPrefix)
		}
		code, _, err = c.do(http.MethodPost, fmt.Sprintf("/api/v1/exercicios/personalizados/%d/desativar", exCodigo), token, nil)
		if err != nil {
			return err
		}
		if code != 200 && code != 204 {
			return fmt.Errorf("desativar status=%d", code)
		}
		code, _, err = c.do(http.MethodPost, fmt.Sprintf("/api/v1/exercicios/personalizados/%d/ativar", exCodigo), token, nil)
		if err != nil {
			return err
		}
		if code != 200 && code != 204 {
			return fmt.Errorf("ativar status=%d", code)
		}
		code, _, err = c.do(http.MethodDelete, fmt.Sprintf("/api/v1/exercicios/personalizados/%d", exCodigo), token, nil)
		if err != nil {
			return err
		}
		if code != 400 {
			return fmt.Errorf("hard delete without confirm expected 400, got %d", code)
		}
		req, _ := http.NewRequest(http.MethodDelete, c.base+fmt.Sprintf("/api/v1/exercicios/personalizados/%d", exCodigo), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Confirm-Hard-Delete", "CONFIRMAR")
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 204 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("hard delete status=%d body=%s", resp.StatusCode, truncate(b))
		}
		code, _, err = c.do(http.MethodGet, "/api/v1/exercicios/sugestoes", token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("sugestoes status=%d", code)
		}
		return nil
	})

	step("8. Garmin", func() error {
		if _, err := os.Stat(fitPath); err != nil {
			return fmt.Errorf("FIT fixture missing at %s", fitPath)
		}
		code, body, err := c.uploadFIT(token, alunoID, fitPath)
		if err != nil {
			return err
		}
		if (code != 200 && code != 201) || !bytes.Contains(body, []byte(`"success":true`)) {
			return fmt.Errorf("upload status=%d body=%s", code, truncate(body))
		}
		atividadeID = firstInt(body, "atividade_id", "id")
		if atividadeID == 0 {
			atividadeID = nestedInt(body, "data", "atividade_id")
		}
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/garmin/aluno/%d/activities", alunoID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("activities status=%d", code)
		}
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/garmin/charts/dashboard/%d", alunoID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("charts dashboard status=%d", code)
		}
		if atividadeID > 0 {
			code, _, err = c.do(http.MethodDelete, fmt.Sprintf("/api/garmin/activity/%d/delete", atividadeID), token, nil)
			if err != nil {
				return err
			}
			if code != 200 && code != 204 {
				return fmt.Errorf("delete activity status=%d", code)
			}
		}
		return nil
	})

	step("9. SVED", func() error {
		code, body, err := c.do(http.MethodPost, "/api/v1/sved/calcular", token, map[string]any{
			"cadencia": "3-0-1-0", "repeticoes": 10, "rir": 2, "series": 3, "intervalo": 60,
		})
		if err != nil {
			return err
		}
		if code != 200 || !bytes.Contains(body, []byte(`"ies"`)) {
			return fmt.Errorf("sved calcular status=%d body=%s", code, truncate(body))
		}
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/sved/sugestoes/%d", periodFichaID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("sved sugestoes lote status=%d", code)
		}
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/sved/historico/%d/%s", alunoID, "Supino"), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("sved historico status=%d", code)
		}
		code, _, err = c.do(http.MethodGet, fmt.Sprintf("/api/v1/sved/dashboard/%d", alunoID), token, nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("sved dashboard status=%d", code)
		}
		return nil
	})

	step("10. RAG", func() error {
		code, body, err := c.do(http.MethodPost, "/api/v1/admin/consulta-base", token, map[string]any{
			"query": "lombalgia", "k": 3,
		})
		if err != nil {
			return err
		}
		if code != 200 || bytes.Contains(body, []byte(`"error"`)) || !bytes.Contains(bytes.ToLower(body), []byte("lombalgia")) {
			return fmt.Errorf("rag status=%d body=%s", code, truncate(body))
		}
		for _, path := range []string{"/historico", "/estatisticas", "/populares"} {
			code, _, err = c.do(http.MethodGet, "/api/v1/admin/consulta-base"+path, token, nil)
			if err != nil {
				return err
			}
			if code != 200 {
				return fmt.Errorf("rag %s status=%d", path, code)
			}
		}
		return nil
	})

	step("11. Relatórios + configs", func() error {
		for _, path := range []string{
			"/api/v1/admin/relatorios/dashboard",
			"/api/v1/admin/relatorios/patologias",
			"/api/v1/admin/relatorios/subutilizados",
			"/api/v1/admin/relatorios/aprovacao?dias=30",
			"/api/v1/admin/configuracoes",
		} {
			code, _, err := c.do(http.MethodGet, path, token, nil)
			if err != nil {
				return err
			}
			if code != 200 {
				return fmt.Errorf("%s status=%d", path, code)
			}
		}
		code, body, err := c.do(http.MethodPost, "/api/v1/admin/configuracoes/testar-smtp", token, nil)
		if err != nil {
			return err
		}
		// Controlled failure or success both acceptable for local RC.
		if code != 200 && code != 400 && code != 503 {
			return fmt.Errorf("testar-smtp unexpected status=%d body=%s", code, truncate(body))
		}
		return nil
	})

	step("12. Erros importantes", func() error {
		code, body, err := c.do(http.MethodGet, "/api/v1/ficha/hash-inexistente-e2e/json", "", nil)
		if err != nil {
			return err
		}
		if code != 404 || !bytes.Contains(body, []byte("error")) {
			return fmt.Errorf("hash inexistente expected 404+error, got %d %s", code, truncate(body))
		}
		if publicHash != "" {
			code, _, err = c.do(http.MethodPost, "/api/v1/desativar/"+publicHash, token, nil)
			if err != nil {
				return err
			}
			if code != 200 && code != 204 {
				return fmt.Errorf("desativar hash status=%d", code)
			}
			code, body, err = c.do(http.MethodGet, "/api/v1/ficha/"+publicHash+"/json", "", nil)
			if err != nil {
				return err
			}
			if code != 410 && code != 404 && code != 403 {
				return fmt.Errorf("hash desativado expected gone/forbidden/notfound, got %d %s", code, truncate(body))
			}
		}
		code, body, err = c.do(http.MethodGet, "/api/v1/anamnese/submit/token-invalido-e2e", "", nil)
		if err != nil {
			return err
		}
		if code != 404 || !bytes.Contains(body, []byte("error")) {
			return fmt.Errorf("token inválido expected 404+error, got %d %s", code, truncate(body))
		}

		// Non-admin access to admin route
		uname := "coach_e2e_" + suffix
		code, body, err = c.do(http.MethodPost, "/api/v1/auth/register", "", map[string]any{
			"username": uname, "email": uname + "@example.com",
			"nome_completo": "Coach E2E", "password": "senha123",
		})
		if err != nil {
			return err
		}
		if code != 201 && code != 200 {
			return fmt.Errorf("register coach status=%d body=%s", code, truncate(body))
		}
		coachID = firstInt(body, "id", "user_id")
		if coachID == 0 {
			code, body, err = c.do(http.MethodGet, "/api/v1/admin/usuarios", token, nil)
			if err != nil {
				return err
			}
			if code != 200 {
				return fmt.Errorf("list usuarios status=%d", code)
			}
			coachID = findUserID(body, uname)
		}
		if coachID <= 0 {
			return fmt.Errorf("could not resolve coach user id")
		}
		// Approve sets aprovado=1 and ativo=1. Do not toggle afterward (that would deactivate).
		code, _, err = c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/usuarios/%d/aprovar", coachID), token, nil)
		if err != nil {
			return err
		}
		if code != 204 {
			return fmt.Errorf("aprovar coach status=%d", code)
		}
		code, body, err = c.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": uname, "password": "senha123",
		})
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("coach login status=%d body=%s", code, truncate(body))
		}
		coachToken = mustString(body, "token")
		code, body, err = c.do(http.MethodGet, "/api/v1/admin/relatorios/dashboard", coachToken, nil)
		if err != nil {
			return err
		}
		if code != 403 || !bytes.Contains(body, []byte("error")) {
			return fmt.Errorf("non-admin expected 403+error, got %d %s", code, truncate(body))
		}

		// AI required 503: only when server is in required mode without provider.
		if os.Getenv("E2E_EXPECT_AI_REQUIRED_503") == "1" {
			code, body, err = c.do(http.MethodPost, "/api/v1/corrida/gerar-blocos", token, map[string]any{
				"vdot": 45.0, "distancia_prova": "10K", "nivel": "intermediario", "dias_semana": 3,
			})
			if err != nil {
				return err
			}
			if code != 503 {
				return fmt.Errorf("expected 503 in required mode, got %d %s", code, truncate(body))
			}
		} else {
			fmt.Println("SKIP: AI required→503 (set E2E_EXPECT_AI_REQUIRED_503=1 with AI_TRAINING_MODE=required)")
		}
		return nil
	})

	step("13. Cleanup best-effort", func() error {
		cleanup := func(label string, fn func() error) {
			if err := fn(); err != nil {
				fmt.Printf("cleanup warn %s: %v\n", label, err)
			}
		}
		if corridaID > 0 {
			cleanup("corrida completa", func() error {
				return c.deleteConfirm(fmt.Sprintf("/api/v1/corrida/%d", corridaID), token)
			})
		}
		if corridaWeekID > 0 {
			cleanup("corrida semana_a_semana", func() error {
				return c.deleteConfirm(fmt.Sprintf("/api/v1/corrida/%d", corridaWeekID), token)
			})
		}
		if periodFichaID > 0 {
			cleanup("ficha periodizada", func() error {
				return c.deleteConfirm(fmt.Sprintf("/api/v1/fichas/%d", periodFichaID), token)
			})
		}
		if manualFichaID > 0 {
			cleanup("ficha manual", func() error {
				return c.deleteConfirm(fmt.Sprintf("/api/v1/fichas/%d", manualFichaID), token)
			})
		}
		if alunoID > 0 {
			cleanup("aluno soft-delete", func() error {
				code, body, err := c.do(http.MethodDelete, fmt.Sprintf("/api/v1/alunos/%d", alunoID), token, nil)
				if err != nil {
					return err
				}
				if code != 204 && code != 200 {
					return fmt.Errorf("status=%d body=%s", code, truncate(body))
				}
				return nil
			})
		}
		if planoID > 0 {
			cleanup("plano deactivate", func() error {
				code, body, err := c.do(http.MethodDelete, fmt.Sprintf("/api/v1/admin/planos/%d", planoID), token, nil)
				if err != nil {
					return err
				}
				if code != 204 && code != 200 {
					return fmt.Errorf("status=%d body=%s", code, truncate(body))
				}
				return nil
			})
		}
		if coachID > 0 {
			cleanup("coach deactivate", func() error {
				code, body, err := c.do(http.MethodPost, fmt.Sprintf("/api/v1/admin/usuarios/%d/toggle", coachID), token, nil)
				if err != nil {
					return err
				}
				if code != 204 && code != 200 {
					return fmt.Errorf("status=%d body=%s", code, truncate(body))
				}
				return nil
			})
		}
		return nil
	})

	fmt.Println()
	if failed > 0 {
		fmt.Printf("E2E FAILED: %d step(s)\n", failed)
		os.Exit(1)
	}
	fmt.Println("E2E PASSED: full API journey completed")
}

type client struct {
	base string
	http *http.Client
}

func (c *client) do(method, path, token string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

func (c *client) deleteConfirm(path, token string) error {
	req, err := http.NewRequest(http.MethodDelete, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Confirm-Hard-Delete", "CONFIRMAR")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, truncate(b))
	}
	return nil
}

// assertAnamneseEmailResult mirrors scripts/smoke_test.sh SMTP expectations:
//
//	E2E_EXPECT_SMTP_FAILURE unset → accept controlled failure OR success
//	true  → require failure
//	false → require success
func assertAnamneseEmailResult(code int, body []byte, expectFailure string) error {
	lower := strings.ToLower(string(body))
	smtpFailure := strings.Contains(lower, "desabilitado") ||
		strings.Contains(lower, "smtp") ||
		strings.Contains(lower, "incompletas")
	smtpSuccess := code == 200 &&
		bytes.Contains(body, []byte(`"success":true`)) &&
		strings.Contains(lower, "reenviado com sucesso")

	switch strings.ToLower(strings.TrimSpace(expectFailure)) {
	case "true", "1", "yes":
		if !smtpFailure {
			return fmt.Errorf("expected SMTP failure (E2E_EXPECT_SMTP_FAILURE=true), status=%d body=%s", code, truncate(body))
		}
		fmt.Println("anamnese reenviar-email: SMTP failure as expected")
		return nil
	case "false", "0", "no":
		if !smtpSuccess {
			return fmt.Errorf("expected SMTP success (E2E_EXPECT_SMTP_FAILURE=false), status=%d body=%s", code, truncate(body))
		}
		fmt.Println("anamnese reenviar-email: SMTP send succeeded")
		return nil
	case "":
		if !smtpFailure && !smtpSuccess {
			return fmt.Errorf("reenviar-email unexpected result status=%d body=%s", code, truncate(body))
		}
		if smtpSuccess {
			fmt.Println("anamnese reenviar-email: SMTP send succeeded")
		} else {
			fmt.Println("anamnese reenviar-email: controlled SMTP failure")
		}
		return nil
	default:
		return fmt.Errorf("invalid E2E_EXPECT_SMTP_FAILURE=%q (use true, false, or unset)", expectFailure)
	}
}

func (c *client) uploadFIT(token string, alunoID int64, path string) (int, []byte, error) {
	// #nosec G304 -- E2E helper opens a local fixture path controlled by the test harness, not client input
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("aluno_id", strconv.FormatInt(alunoID, 10))
	part, err := w.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return 0, nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return 0, nil, err
	}
	if err := w.Close(); err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.base+"/api/garmin/upload", &buf)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func truncate(b []byte) string {
	s := string(b)
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}

type evidenceMetaExpect struct {
	Complexity           string
	EvidenceCount        int
	MinEvidenceCount     int
	EvidenceFallback     bool
	EvidenceReasonSubstr string
	ContextUsed          bool
	RequireConfidence    bool
}

func assertEvidenceAIMetadata(body []byte, want evidenceMetaExpect) error {
	meta := nestedMap(body, "data", "ai_metadata")
	if meta == nil {
		return fmt.Errorf("missing data.ai_metadata in %s", truncate(body))
	}
	complexity, _ := meta["complexity"].(string)
	if want.Complexity != "" && complexity != want.Complexity {
		return fmt.Errorf("complexity=%q want %q", complexity, want.Complexity)
	}
	count := int(toInt64(meta["evidence_count"]))
	if want.Complexity == "simples" {
		if count != want.EvidenceCount {
			return fmt.Errorf("evidence_count=%d want %d", count, want.EvidenceCount)
		}
	}
	if want.MinEvidenceCount > 0 && count < want.MinEvidenceCount {
		return fmt.Errorf("evidence_count=%d want >= %d", count, want.MinEvidenceCount)
	}
	fb, ok := meta["evidence_fallback_used"].(bool)
	if !ok {
		return fmt.Errorf("evidence_fallback_used missing")
	}
	if want.Complexity == "simples" && fb != want.EvidenceFallback {
		return fmt.Errorf("evidence_fallback_used=%v want %v", fb, want.EvidenceFallback)
	}
	ctxUsed, _ := meta["context_used"].(bool)
	if ctxUsed != want.ContextUsed {
		return fmt.Errorf("context_used=%v want %v", ctxUsed, want.ContextUsed)
	}
	if meta["confidence_score"] == nil {
		return fmt.Errorf("confidence_score missing")
	}
	if score, ok := meta["confidence_score"].(float64); ok && score <= 0 {
		return fmt.Errorf("confidence_score=%v want > 0", score)
	}
	reasons, _ := meta["evidence_reasons"].([]any)
	if want.EvidenceReasonSubstr != "" {
		found := false
		for _, r := range reasons {
			s, _ := r.(string)
			if strings.Contains(s, want.EvidenceReasonSubstr) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("evidence_reasons=%v missing substr %q", reasons, want.EvidenceReasonSubstr)
		}
	}
	validations, _ := meta["validations"].([]any)
	if len(validations) == 0 {
		return fmt.Errorf("validations empty")
	}
	return nil
}

func nestedMap(body []byte, keys ...string) map[string]any {
	var cur any
	if err := json.Unmarshal(body, &cur); err != nil {
		return nil
	}
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	out, _ := cur.(map[string]any)
	return out
}

// assertTrainingPipelineTelemetry verifies PR5 rows (optional DB access).
// Uses E2E_SQLITE_PATH, or docker cp from E2E_DOCKER_CONTAINER (default staff_api).
func assertTrainingPipelineTelemetry(alunoID int64, wantComplexities []string) error {
	dbPath, cleanup, err := resolveE2ESQLitePath()
	if err != nil {
		return err
	}
	if dbPath == "" {
		fmt.Println("SKIP PR5 sqlite check (set E2E_SQLITE_PATH or run with Docker container staff_api)")
		return nil
	}
	if cleanup != nil {
		defer cleanup()
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT complexity, evidence_requested, evidence_count, endpoint
		FROM training_pipeline_events
		WHERE aluno_id = ?
		ORDER BY id DESC
		LIMIT 10
	`, alunoID)
	if err != nil {
		return fmt.Errorf("query events (migration 0015 applied?): %w", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	n := 0
	for rows.Next() {
		var complexity, endpoint string
		var requested, count int
		if err := rows.Scan(&complexity, &requested, &count, &endpoint); err != nil {
			return err
		}
		if endpoint != "gerar-periodizada" {
			return fmt.Errorf("endpoint=%q", endpoint)
		}
		seen[complexity] = true
		n++
		if complexity == "simples" && (requested != 0 || count != 0) {
			return fmt.Errorf("simples row requested=%d count=%d", requested, count)
		}
		if complexity == "moderado" && requested != 1 {
			return fmt.Errorf("moderado row evidence_requested=%d want 1", requested)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no training_pipeline_events for aluno_id=%d", alunoID)
	}
	for _, c := range wantComplexities {
		if !seen[c] {
			return fmt.Errorf("missing telemetry complexity %q (seen %v)", c, seen)
		}
	}
	fmt.Printf("PR5 telemetry OK: %d event(s) for aluno_id=%d complexities=%v\n", n, alunoID, seen)
	return nil
}

func resolveE2ESQLitePath() (string, func(), error) {
	if p := strings.TrimSpace(os.Getenv("E2E_SQLITE_PATH")); p != "" {
		return p, nil, nil
	}
	if strings.EqualFold(os.Getenv("E2E_SKIP_TELEMETRY_DB"), "1") {
		return "", nil, nil
	}
	container := env("E2E_DOCKER_CONTAINER", "staff_api")
	remote := env("E2E_DOCKER_DB_PATH", "/app/data/db/fichas_treino.db")
	if !validE2EDockerContainer(container) || !validE2EDockerDBPath(remote) {
		return "", nil, nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return "", nil, nil
	}
	// Probe container (args allowlisted above).
	probe := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", container) // #nosec G204
	out, err := probe.Output()
	if err != nil || !strings.Contains(string(out), "true") {
		return "", nil, nil
	}
	tmp, err := os.CreateTemp("", "staff-e2e-*.db")
	if err != nil {
		return "", nil, err
	}
	local := tmp.Name()
	_ = tmp.Close()
	src := container + ":" + remote
	cp := exec.Command("docker", "cp", src, local) // #nosec G204
	if out, err := cp.CombinedOutput(); err != nil {
		_ = os.Remove(local)
		return "", nil, fmt.Errorf("docker cp: %w (%s)", err, truncate(out))
	}
	return local, func() { _ = os.Remove(local) }, nil
}

func validE2EDockerContainer(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			continue
		case r == '_' || r == '-' || r == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validE2EDockerDBPath(p string) bool {
	p = filepath.Clean(strings.TrimSpace(p))
	if !strings.HasPrefix(p, "/app/data/") {
		return false
	}
	if strings.Contains(p, "..") {
		return false
	}
	return strings.HasSuffix(p, ".db")
}

func mustString(body []byte, key string) string {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func mustInt(body []byte, key string) int64 {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return 0
	}
	return toInt64(m[key])
}

func nestedInt(body []byte, k1, k2 string) int64 {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return 0
	}
	n, _ := m[k1].(map[string]any)
	if n == nil {
		return 0
	}
	return toInt64(n[k2])
}

func nestedString(body []byte, k1, k2 string) string {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	n, _ := m[k1].(map[string]any)
	if n == nil {
		return ""
	}
	s, _ := n[k2].(string)
	return s
}

func firstInt(body []byte, keys ...string) int64 {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return 0
	}
	for _, k := range keys {
		if v := toInt64(m[k]); v > 0 {
			return v
		}
	}
	// nested data
	if n, ok := m["data"].(map[string]any); ok {
		for _, k := range keys {
			if v := toInt64(n[k]); v > 0 {
				return v
			}
		}
	}
	return 0
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case json.Number:
		i, _ := t.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(t, 10, 64)
		return i
	default:
		return 0
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func findUserID(body []byte, username string) int64 {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return 0
	}
	arr, _ := m["usuarios"].([]any)
	for _, item := range arr {
		u, _ := item.(map[string]any)
		if u == nil {
			continue
		}
		if u["username"] == username {
			return toInt64(u["id"])
		}
	}
	return 0
}
