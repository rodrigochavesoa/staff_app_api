package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"staff_app/internal/repositories"
)

type HealthHandler struct {
	db repositories.DatabaseHealth
}

func NewHealthHandler(db repositories.DatabaseHealth) *HealthHandler {
	return &HealthHandler{db: db}
}

// Health verifica o estado do serviço e da conexão com o banco.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	dbStatus := "connected"

	// Verifica a conexão com o banco.
	if h.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()
		if err := h.db.PingContext(ctx); err != nil {
			status = "error"
			dbStatus = "disconnected: " + err.Error()
		}
	} else {
		status = "error"
		dbStatus = "not_initialized"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]any{
		"status":    status,
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
		"database":  dbStatus,
	}

	_ = json.NewEncoder(w).Encode(response)
}

// Ping responde sem consultar o banco.
func (h *HealthHandler) Ping(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"pong"}`))
}

// Index retorna informações básicas da API.
func (h *HealthHandler) Index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]any{
		"service":     "STAFF Fichas Web API (Go Edition)",
		"version":     "1.0.0",
		"status":      "running",
		"description": "REST API in Go, frontend-agnostic and decoupled.",
		"endpoints": map[string]any{
			"health": []string{"GET /health", "GET /ping"},
		},
	}

	_ = json.NewEncoder(w).Encode(response)
}
