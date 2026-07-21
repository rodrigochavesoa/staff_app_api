package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	garminpkg "staff_app/internal/garmin"
	"staff_app/internal/repositories"

	"github.com/go-chi/chi/v5"
)

type GarminHandler struct {
	cfg       *config.Config
	repo      repositories.GarminRepository
	alunoRepo repositories.AlunoRepository
}

func NewGarminHandler(cfg *config.Config, repo repositories.GarminRepository, aluno repositories.AlunoRepository) *GarminHandler {
	return &GarminHandler{
		cfg:       cfg,
		repo:      repo,
		alunoRepo: aluno,
	}
}

type garminUploadResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message"`
}

func (h *GarminHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxUploadBytes)
	// Limita a memória do multipart a 10 MB para evitar uso excessivo de RAM (G120).
	const maxMultipartMemory = 10 * 1024 * 1024
	// #nosec G120 - MaxBytesReader limita o corpo total da requisição e evita exaustão de memória.
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{
			Success: false,
			Message: "Arquivo excede o limite permitido ou formulário inválido",
		})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{
			Success: false,
			Message: "Nenhum arquivo foi enviado",
		})
		return
	}
	defer file.Close()

	alunoID, err := strconv.ParseInt(r.FormValue("aluno_id"), 10, 64)
	if err != nil || alunoID <= 0 {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{
			Success: false,
			Message: "aluno_id é obrigatório",
		})
		return
	}

	if _, err := h.alunoRepo.GetByID(r.Context(), alunoID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeGarminJSON(w, http.StatusNotFound, garminUploadResponse{
				Success: false,
				Message: "Aluno não encontrado",
			})
			return
		}
		writeJSONError(w, "Failed to verify student existence", http.StatusInternalServerError)
		return
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(header.Filename), "."))
	if ext != "csv" && ext != "fit" {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{
			Success: false,
			Message: "Apenas arquivos .fit ou .csv são permitidos",
		})
		return
	}

	savedPath, savedName, err := h.saveUpload(file, alunoID, header.Filename)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{
			Success: false,
			Message: fmt.Sprintf("Erro ao salvar arquivo: %v", err),
		})
		return
	}

	if ext == "csv" {
		h.processCSV(w, r, savedPath, savedName, alunoID)
		return
	}
	h.processFIT(w, r, savedPath, savedName, alunoID)
}

func (h *GarminHandler) processCSV(w http.ResponseWriter, r *http.Request, path, filename string, alunoID int64) {
	// #nosec G304 - o caminho é gerado nesta função e validado para permanecer em GarminUploadDir.
	f, err := os.Open(path)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{
			Success: false,
			Message: fmt.Sprintf("Erro ao abrir arquivo CSV: %v", err),
		})
		return
	}
	defer f.Close()

	activities, err := garminpkg.ParseCSV(f)
	if err != nil {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{
			Success: false,
			Message: fmt.Sprintf("Erro ao ler arquivo CSV: %v", err),
		})
		return
	}
	if len(activities) == 0 {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{
			Success: false,
			Message: "Arquivo CSV não contém atividades válidas",
		})
		return
	}

	var ids []int64
	var summaries []any
	for _, parsed := range activities {
		activity := parsed.Activity
		activity.AlunoID = alunoID
		activity.FileNome = filename
		id, err := h.repo.SaveActivity(r.Context(), &activity)
		if err != nil {
			if strings.Contains(err.Error(), "Atividade já registrada") || strings.Contains(err.Error(), "atividade já registrada") {
				continue
			}
			writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{
				Success: false,
				Message: fmt.Sprintf("Erro ao salvar atividade: %v", err),
			})
			return
		}
		ids = append(ids, id)
		if activity.AnalyticsSummary != nil {
			summaries = append(summaries, activity.AnalyticsSummary.ActivitySummary)
		}
	}

	writeGarminJSON(w, http.StatusCreated, garminUploadResponse{
		Success: true,
		Data: map[string]any{
			"atividade_ids":      ids,
			"activity_summaries": summaries,
			"total_activities":   len(ids),
			"file_type":          "csv",
		},
		Message: fmt.Sprintf("Arquivo CSV processado com sucesso! (%d atividades importadas)", len(ids)),
	})
}

func (h *GarminHandler) processFIT(w http.ResponseWriter, r *http.Request, path, filename string, alunoID int64) {
	activity, err := garminpkg.ParseFITFile(path)
	if err != nil {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{
			Success: false,
			Message: fmt.Sprintf("Erro ao processar arquivo FIT: %v", err),
		})
		return
	}
	activity.AlunoID = alunoID
	activity.FileNome = filename

	id, err := h.repo.SaveActivity(r.Context(), activity)
	if err != nil {
		if strings.Contains(err.Error(), "atividade já registrada") {
			writeGarminJSON(w, http.StatusConflict, garminUploadResponse{
				Success: false,
				Message: fmt.Sprintf("Atividade já registrada: %v", err),
			})
			return
		}
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{
			Success: false,
			Message: fmt.Sprintf("Erro ao salvar atividade: %v", err),
		})
		return
	}

	summary := any(nil)
	if activity.AnalyticsSummary != nil {
		summary = activity.AnalyticsSummary.ActivitySummary
	}
	writeGarminJSON(w, http.StatusCreated, garminUploadResponse{
		Success: true,
		Data: map[string]any{
			"atividade_id":       id,
			"activity_summary":   summary,
			"total_records":      len(activity.Records),
			"heart_rate_metrics": metricOrNil(activity.AnalyticsSummary, "hr"),
			"speed_metrics":      metricOrNil(activity.AnalyticsSummary, "speed"),
			"file_type":          "fit",
		},
		Message: fmt.Sprintf("Arquivo FIT processado com sucesso! (%d registros)", len(activity.Records)),
	})
}

func (h *GarminHandler) Activity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "atividade_id"), 10, 64)
	if err != nil || id <= 0 {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{Success: false, Message: "ID da atividade inválido"})
		return
	}

	activity, err := h.repo.Activity(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeGarminJSON(w, http.StatusNotFound, garminUploadResponse{Success: false, Message: "Atividade não encontrada"})
			return
		}
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao buscar atividade: %v", err)})
		return
	}
	records, err := h.repo.ActivityRecords(r.Context(), id, 10000)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao buscar registros: %v", err)})
		return
	}
	analytics, err := h.repo.ActivityAnalytics(r.Context(), id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao buscar analytics: %v", err)})
		return
	}

	writeGarminJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{
			"atividade": activity,
			"records":   records,
			"analytics": analytics,
		},
	})
}

func (h *GarminHandler) ListAlunoActivities(w http.ResponseWriter, r *http.Request) {
	alunoID, err := strconv.ParseInt(chi.URLParam(r, "aluno_id"), 10, 64)
	if err != nil || alunoID <= 0 {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{Success: false, Message: "ID do aluno inválido"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	activityType := r.URL.Query().Get("activity_type")

	activities, total, err := h.repo.ListAlunoActivities(r.Context(), alunoID, activityType, limit, offset)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao listar atividades: %v", err)})
		return
	}

	if activities == nil {
		activities = []*domain.GarminActivity{}
	}
	writeGarminJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{
			"activities": activities,
			"total":      total,
			"filtered_by": map[string]any{
				"activity_type": activityType,
				"limit":         normalizedLimit(limit),
				"offset":        maxInt(offset, 0),
			},
		},
	})
}

func (h *GarminHandler) AlunoStats(w http.ResponseWriter, r *http.Request) {
	alunoID, err := strconv.ParseInt(chi.URLParam(r, "aluno_id"), 10, 64)
	if err != nil || alunoID <= 0 {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{Success: false, Message: "ID do aluno inválido"})
		return
	}
	stats, err := h.repo.AlunoStats(r.Context(), alunoID)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao calcular estatísticas: %v", err)})
		return
	}
	writeGarminJSON(w, http.StatusOK, map[string]any{"success": true, "data": stats})
}

func (h *GarminHandler) DeleteActivity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "atividade_id"), 10, 64)
	if err != nil || id <= 0 {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{Success: false, Message: "ID da atividade inválido"})
		return
	}
	if err := h.repo.DeleteActivity(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeGarminJSON(w, http.StatusNotFound, garminUploadResponse{Success: false, Message: "Atividade não encontrada"})
			return
		}
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao deletar atividade: %v", err)})
		return
	}
	writeGarminJSON(w, http.StatusOK, garminUploadResponse{Success: true, Message: "Atividade deletada com sucesso!"})
}

func (h *GarminHandler) ChartDistance(w http.ResponseWriter, r *http.Request) {
	h.chartFromActivities(w, r, "distance_timeline", garminpkg.ChartDistanceTimeline, "Nenhuma atividade encontrada")
}

func (h *GarminHandler) ChartActivityTypes(w http.ResponseWriter, r *http.Request) {
	h.chartFromActivities(w, r, "activity_types", garminpkg.ChartActivityTypes, "Nenhuma atividade encontrada")
}

func (h *GarminHandler) ChartVelocityScatter(w http.ResponseWriter, r *http.Request) {
	h.chartFromActivities(w, r, "velocity_scatter", garminpkg.ChartVelocityScatter, "Nenhuma atividade encontrada")
}

func (h *GarminHandler) ChartHRZones(w http.ResponseWriter, r *http.Request) {
	alunoID, ok := h.alunoIDParam(w, r)
	if !ok {
		return
	}
	samples, _, err := h.repo.HeartRateSamples(r.Context(), alunoID, 5000)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao gerar gráfico: %v", err)})
		return
	}
	fcmax, err := h.repo.MaxHeartRate(r.Context(), alunoID)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao gerar gráfico: %v", err)})
		return
	}
	chart, ok := garminpkg.ChartHeartRateZones(samples, fcmax)
	if !ok {
		writeGarminJSON(w, http.StatusNotFound, garminUploadResponse{Success: false, Message: "Nenhum dado de frequência cardíaca"})
		return
	}
	writeChartResponse(w, "hr_zones", chart)
}

func (h *GarminHandler) ChartHRSeries(w http.ResponseWriter, r *http.Request) {
	alunoID, ok := h.alunoIDParam(w, r)
	if !ok {
		return
	}
	samples, _, err := h.repo.HeartRateSamples(r.Context(), alunoID, 500)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao gerar gráfico: %v", err)})
		return
	}
	chart, ok := garminpkg.ChartHRSeries(samples)
	if !ok {
		writeGarminJSON(w, http.StatusNotFound, garminUploadResponse{Success: false, Message: "Nenhum dado de frequência cardíaca"})
		return
	}
	writeChartResponse(w, "hr_series", chart)
}

func (h *GarminHandler) ChartCalories(w http.ResponseWriter, r *http.Request) {
	alunoID, ok := h.alunoIDParam(w, r)
	if !ok {
		return
	}
	samples, err := h.repo.CaloriesSamples(r.Context(), alunoID, 20)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao gerar gráfico: %v", err)})
		return
	}
	chart, ok := garminpkg.ChartCalories(samples)
	if !ok {
		writeGarminJSON(w, http.StatusNotFound, garminUploadResponse{Success: false, Message: "Nenhum dado de calorias encontrado"})
		return
	}
	writeChartResponse(w, "calories", chart)
}

func (h *GarminHandler) ChartDashboard(w http.ResponseWriter, r *http.Request) {
	alunoID, ok := h.alunoIDParam(w, r)
	if !ok {
		return
	}
	activities, _, err := h.repo.ListAlunoActivities(r.Context(), alunoID, "", 100, 0)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao gerar dashboard: %v", err)})
		return
	}
	samples, _, err := h.repo.HeartRateSamples(r.Context(), alunoID, 5000)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao gerar dashboard: %v", err)})
		return
	}
	fcmax, err := h.repo.MaxHeartRate(r.Context(), alunoID)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao gerar dashboard: %v", err)})
		return
	}
	chart, ok := garminpkg.ChartDashboard(activities, samples, fcmax)
	if !ok {
		writeGarminJSON(w, http.StatusNotFound, garminUploadResponse{Success: false, Message: "Nenhum dado disponível"})
		return
	}
	writeChartResponse(w, "dashboard", chart)
}

func (h *GarminHandler) chartFromActivities(w http.ResponseWriter, r *http.Request, chartType string, build func([]*domain.GarminActivity) (string, bool), notFoundMessage string) {
	alunoID, ok := h.alunoIDParam(w, r)
	if !ok {
		return
	}
	activities, _, err := h.repo.ListAlunoActivities(r.Context(), alunoID, "", 100, 0)
	if err != nil {
		writeGarminJSON(w, http.StatusInternalServerError, garminUploadResponse{Success: false, Message: fmt.Sprintf("Erro ao gerar gráfico: %v", err)})
		return
	}
	chart, ok := build(activities)
	if !ok {
		writeGarminJSON(w, http.StatusNotFound, garminUploadResponse{Success: false, Message: notFoundMessage})
		return
	}
	writeChartResponse(w, chartType, chart)
}

func (h *GarminHandler) alunoIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	alunoID, err := strconv.ParseInt(chi.URLParam(r, "aluno_id"), 10, 64)
	if err != nil || alunoID <= 0 {
		writeGarminJSON(w, http.StatusBadRequest, garminUploadResponse{Success: false, Message: "ID do aluno inválido"})
		return 0, false
	}
	return alunoID, true
}

func (h *GarminHandler) saveUpload(src io.Reader, alunoID int64, originalName string) (string, string, error) {
	// Restringe a permissão do diretório a 0750 (G301).
	if err := os.MkdirAll(h.cfg.GarminUploadDir, 0o750); err != nil {
		return "", "", err
	}
	filename := fmt.Sprintf("%d_%s_%s", alunoID, time.Now().Format("20060102_150405"), safeFilename(originalName))
	dst := filepath.Join(h.cfg.GarminUploadDir, filename)
	cleanRoot, err := filepath.Abs(h.cfg.GarminUploadDir)
	if err != nil {
		return "", "", err
	}
	cleanDst, err := filepath.Abs(dst)
	if err != nil {
		return "", "", err
	}
	if rel, err := filepath.Rel(cleanRoot, cleanDst); err != nil || strings.HasPrefix(rel, "..") {
		return "", "", fmt.Errorf("invalid upload destination")
	}

	// #nosec G304 - cleanDst é validado para permanecer em cleanRoot, sem travessia de diretório.
	// Restringe a permissão do arquivo a 0600 (G302).
	out, err := os.OpenFile(cleanDst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return "", "", err
	}
	return cleanDst, filename, nil
}

func safeFilename(name string) string {
	base := filepath.Base(name)
	var b strings.Builder
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "activity"
	}
	return b.String()
}

func writeGarminJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeChartResponse(w http.ResponseWriter, chartType, chart string) {
	writeGarminJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data": map[string]any{
			"chart_json": chart,
			"chart_type": chartType,
		},
	})
}

func metricOrNil(summary *domain.AnalyticsSummary, name string) any {
	if summary == nil {
		return nil
	}
	switch name {
	case "hr":
		return summary.HeartRate
	case "speed":
		return summary.Speed
	default:
		return nil
	}
}

func normalizedLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
