package http

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"staff_app/internal/config"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

func TestGarminCSVUploadFlow(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "http-garmin-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sqlite.Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, "INSERT INTO alunos (id, nome, idade, sexo, email) VALUES (1, 'Carlos Aluno', 25, 'M', 'carlos@test.com')")
	if err != nil {
		t.Fatalf("failed to insert student: %v", err)
	}

	cfg := &config.Config{
		CorsOrigins:     []string{"*"},
		GarminUploadDir: filepath.Join(tempDir, "uploads"),
		MaxUploadBytes:  50 * 1024 * 1024,
	}
	router := NewRouter(cfg, db)
	authHeader := testAuthHeader(t, db, cfg)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("aluno_id", "1"); err != nil {
		t.Fatalf("failed to write aluno_id: %v", err)
	}
	part, err := writer.CreateFormFile("file", "atividades.csv")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	csv := `Tipo de atividade;Data;Título;Distância;Calorias;Tempo;FC Média;FC máxima;Cadência de corrida média;Ritmo médio;Melhor ritmo;Subida total;Descida total;Potência média
Corrida;2026-07-15 06:30:00;Treino leve;5,20;360;00:31:12;142;171;164;06:00;05:10;45;40;230
`
	if _, err := part.Write([]byte(csv)); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/garmin/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var uploadResp struct {
		Success bool `json:"success"`
		Data    struct {
			AtividadeIDs []int64 `json:"atividade_ids"`
			FileType     string  `json:"file_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("failed to decode upload response: %v", err)
	}
	if !uploadResp.Success || uploadResp.Data.FileType != "csv" || len(uploadResp.Data.AtividadeIDs) != 1 {
		t.Fatalf("unexpected upload response: %+v", uploadResp)
	}
	activityID := uploadResp.Data.AtividadeIDs[0]

	req = httptest.NewRequest(http.MethodGet, "/api/garmin/aluno/1/activities", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d. Body: %s", w.Code, w.Body.String())
	}
	var listResp struct {
		Success bool `json:"success"`
		Data    struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if !listResp.Success || listResp.Data.Total != 1 {
		t.Fatalf("unexpected list response: %+v", listResp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/garmin/activity/"+strconvFormatInt(activityID), nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 activity, got %d. Body: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/garmin/aluno/1/stats", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 stats, got %d. Body: %s", w.Code, w.Body.String())
	}
	var statsResp struct {
		Success bool `json:"success"`
		Data    struct {
			TotalActivities int     `json:"total_activities"`
			TotalDistanceKM float64 `json:"total_distance_km"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &statsResp)
	if !statsResp.Success || statsResp.Data.TotalActivities != 1 || statsResp.Data.TotalDistanceKM != 5.2 {
		t.Fatalf("unexpected stats response: %+v", statsResp)
	}

	chartPaths := map[string]string{
		"/api/garmin/charts/distance/1":         "distance_timeline",
		"/api/garmin/charts/hr-zones/1":         "hr_zones",
		"/api/garmin/charts/activity-types/1":   "activity_types",
		"/api/garmin/charts/velocity-scatter/1": "velocity_scatter",
		"/api/garmin/charts/dashboard/1":        "dashboard",
		"/api/garmin/charts/hr-series/1":        "hr_series",
		"/api/garmin/charts/calories/1":         "calories",
	}
	for path, chartType := range chartPaths {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", authHeader)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d. Body: %s", path, w.Code, w.Body.String())
		}

		var chartResp struct {
			Success bool `json:"success"`
			Data    struct {
				ChartJSON string `json:"chart_json"`
				ChartType string `json:"chart_type"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &chartResp); err != nil {
			t.Fatalf("failed to decode chart response for %s: %v", path, err)
		}
		if !chartResp.Success || chartResp.Data.ChartType != chartType || chartResp.Data.ChartJSON == "" {
			t.Fatalf("unexpected chart response for %s: %+v", path, chartResp)
		}
		var fig map[string]any
		if err := json.Unmarshal([]byte(chartResp.Data.ChartJSON), &fig); err != nil {
			t.Fatalf("chart_json for %s is not valid JSON: %v", path, err)
		}
		if _, ok := fig["data"]; !ok {
			t.Fatalf("chart_json for %s has no data field: %s", path, chartResp.Data.ChartJSON)
		}
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/garmin/activity/"+strconvFormatInt(activityID)+"/delete", nil)
	req.Header.Set("Authorization", authHeader)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 delete, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestGarminFITUploadFixture(t *testing.T) {
	logger.Setup("development", false)

	files, err := filepath.Glob(filepath.Join("..", "garmin", "testdata", "fit", "*.fit"))
	if err != nil {
		t.Fatalf("failed to glob fit fixtures: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one FIT fixture")
	}
	fitData, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("failed to read fit fixture: %v", err)
	}

	tempDir, err := os.MkdirTemp("", "http-garmin-fit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sqlite.Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	_, err = db.ExecContext(context.Background(), "INSERT INTO alunos (id, nome, idade, sexo, email) VALUES (1, 'Carlos Aluno', 25, 'M', 'carlos.fit@test.com')")
	if err != nil {
		t.Fatalf("failed to insert student: %v", err)
	}

	cfg := &config.Config{
		CorsOrigins:     []string{"*"},
		GarminUploadDir: filepath.Join(tempDir, "uploads"),
		MaxUploadBytes:  50 * 1024 * 1024,
	}
	router := NewRouter(cfg, db)
	authHeader := testAuthHeader(t, db, cfg)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("aluno_id", "1"); err != nil {
		t.Fatalf("failed to write aluno_id: %v", err)
	}
	part, err := writer.CreateFormFile("file", filepath.Base(files[0]))
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write(fitData); err != nil {
		t.Fatalf("failed to write fit data: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/garmin/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", authHeader)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var uploadResp struct {
		Success bool `json:"success"`
		Data    struct {
			AtividadeID  int64  `json:"atividade_id"`
			TotalRecords int    `json:"total_records"`
			FileType     string `json:"file_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("failed to decode upload response: %v", err)
	}
	if !uploadResp.Success || uploadResp.Data.AtividadeID == 0 || uploadResp.Data.FileType != "fit" || uploadResp.Data.TotalRecords == 0 {
		t.Fatalf("unexpected FIT upload response: %+v", uploadResp)
	}
}

func strconvFormatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

func TestGarminUploadSecurity(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "http-garmin-security-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sqlite.Connect(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	_, err = db.ExecContext(ctx, "INSERT INTO alunos (id, nome, idade, sexo, email) VALUES (1, 'Carlos Aluno', 25, 'M', 'carlos.security@test.com')")
	if err != nil {
		t.Fatalf("failed to insert student: %v", err)
	}

	uploadDir := filepath.Join(tempDir, "uploads")
	cfg := &config.Config{
		CorsOrigins:     []string{"*"},
		GarminUploadDir: uploadDir,
		MaxUploadBytes:  1024 * 1024,
	}
	router := NewRouter(cfg, db)
	authHeader := testAuthHeader(t, db, cfg)

	// Test cases with malicious filenames
	testCases := []struct {
		name             string
		originalFilename string
		expectedCode     int
	}{
		{
			name:             "Path traversal UNIX",
			originalFilename: "../../../../evil.csv",
			expectedCode:     http.StatusCreated,
		},
		{
			name:             "Path traversal Windows",
			originalFilename: "..\\..\\..\\..\\evil2.csv",
			expectedCode:     http.StatusCreated,
		},
		{
			name:             "Filename with special characters",
			originalFilename: "my_activity$;[]`'\"&<>|;{}.csv",
			expectedCode:     http.StatusCreated,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			if err := writer.WriteField("aluno_id", "1"); err != nil {
				t.Fatalf("failed to write aluno_id: %v", err)
			}
			part, err := writer.CreateFormFile("file", tc.originalFilename)
			if err != nil {
				t.Fatalf("failed to create form file: %v", err)
			}
			csv := `Tipo de atividade;Data;Título;Distância;Calorias;Tempo
Corrida;2026-07-15 06:30:00;Treino leve;5.20;360;00:31:12
`
			if _, err := part.Write([]byte(csv)); err != nil {
				t.Fatalf("failed to write csv: %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("failed to close multipart writer: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/garmin/upload", body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("Authorization", authHeader)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.expectedCode {
				t.Errorf("expected status %d, got %d. Body: %s", tc.expectedCode, w.Code, w.Body.String())
			}

			// Verify that no file escaped uploadDir
			files, err := filepath.Glob(filepath.Join(tempDir, "*"))
			if err != nil {
				t.Fatalf("failed to glob tempDir: %v", err)
			}
			for _, file := range files {
				base := filepath.Base(file)
				if base != "test.db" && base != "uploads" {
					t.Errorf("file escaped upload directory! Found: %s", file)
				}
			}

			// Verify that the files are created strictly inside uploadDir
			uploadFiles, err := filepath.Glob(filepath.Join(uploadDir, "*"))
			if err != nil {
				t.Fatalf("failed to glob uploadDir: %v", err)
			}
			for _, file := range uploadFiles {
				absFile, err := filepath.Abs(file)
				if err != nil {
					t.Fatalf("failed to get absolute file path: %v", err)
				}
				absUploadDir, err := filepath.Abs(uploadDir)
				if err != nil {
					t.Fatalf("failed to get absolute upload directory path: %v", err)
				}
				if !strings.HasPrefix(absFile, absUploadDir) {
					t.Errorf("file %s is outside upload directory %s", absFile, absUploadDir)
				}
			}
		})
	}
}
