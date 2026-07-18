package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/services"
	"staff_app/internal/sqlite"
)

type RAGHandler struct {
	repo              *sqlite.RAGRepository
	embeddingProvider services.EmbeddingProvider
	vectorStore       services.VectorStore
}

func NewRAGHandler(db *sqlite.DB, embed services.EmbeddingProvider, store services.VectorStore) *RAGHandler {
	return &RAGHandler{
		repo:              sqlite.NewRAGRepository(db),
		embeddingProvider: embed,
		vectorStore:       store,
	}
}

func normalizeQuery(q string) string {
	return strings.ToLower(strings.Join(strings.Fields(q), " "))
}

func (h *RAGHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query      string `json:"query"`
		K          int    `json:"k"`
		Modalidade string `json:"modalidade"`
		Objetivo   string `json:"objetivo"`
		Perfil     string `json:"perfil"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// 1. Validation
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" || len(req.Query) < 3 || len(req.Query) > 500 {
		writeJSONError(w, "Query must be between 3 and 500 characters", http.StatusBadRequest)
		return
	}

	if req.K <= 0 {
		req.K = 3
	} else if req.K > 10 {
		req.K = 10
	}

	req.Modalidade = strings.TrimSpace(req.Modalidade)
	req.Objetivo = strings.TrimSpace(req.Objetivo)
	req.Perfil = strings.TrimSpace(req.Perfil)

	if len(req.Modalidade) > 100 || len(req.Objetivo) > 100 || len(req.Perfil) > 100 {
		writeJSONError(w, "Optional fields (modalidade, objetivo, perfil) must not exceed 100 characters", http.StatusBadRequest)
		return
	}

	queryNorm := normalizeQuery(req.Query)

	// Fetch current user ID from request context (set by AuthMiddleware)
	var userID *int64
	if userVal := r.Context().Value(userContextKey{}); userVal != nil {
		if u, ok := userVal.(*domain.User); ok {
			userID = &u.ID
		}
	}

	// 2. Check Cache
	cached, err := h.repo.GetCachedQuery(r.Context(), queryNorm, req.Modalidade, req.Objetivo, req.Perfil, req.K)
	if err != nil {
		writeJSONError(w, "Database error checking cache: "+err.Error(), http.StatusInternalServerError)
		return
	}

	queryUsada := req.Query
	if req.Modalidade != "" {
		queryUsada += " para " + req.Modalidade
	}
	if req.Objetivo != "" {
		queryUsada += " com objetivo de " + req.Objetivo
	}
	if req.Perfil != "" {
		queryUsada += " nível " + req.Perfil
	}

	if cached != nil {
		// Increment Cache hits
		if err := h.repo.IncrementCacheHits(r.Context(), cached.ID); err != nil {
			logger.Error("Failed to increment cache hits", err)
		}

		var docs []domain.KnowledgeDocument
		if err := json.Unmarshal([]byte(cached.ResultadosJSON), &docs); err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success":          true,
				"query_original":   req.Query,
				"query_usada":      queryUsada,
				"total_resultados": len(docs),
				"from_cache":       true,
				"cache_hits":       cached.Hits + 1,
				"resultados":       docs,
			})
			return
		}
	}

	// 3. Query External Vector Database (RAG)
	var docs []domain.KnowledgeDocument
	var queryErr error
	if h.embeddingProvider != nil && h.vectorStore != nil {
		var embeddings []float32
		embeddings, queryErr = h.embeddingProvider.GenerateEmbeddings(r.Context(), queryNorm)
		if queryErr == nil {
			docs, queryErr = h.vectorStore.SearchSimilar(r.Context(), embeddings, req.K)
		}
	} else {
		queryErr = errors.New("external RAG provider is not configured")
	}

	// 4. Fallback to Local SQLite Document Search
	if queryErr != nil {
		logger.Info("External RAG query failed or not configured, falling back to local document search", "error", queryErr.Error())
		docs, err = h.repo.SearchLocalDocuments(r.Context(), req.Query, req.Modalidade, req.K)
		if err != nil {
			writeJSONError(w, "Local search database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// 5. If both external and local search yield no active documents, return HTTP 503
	if len(docs) == 0 {
		writeJSONError(w, services.ErrNoServiceAvailable.Error(), http.StatusServiceUnavailable)
		return
	}

	// 6. Cache new result
	resultadosBytes, err := json.Marshal(docs)
	if err == nil {
		if err := h.repo.SaveCachedQuery(r.Context(), req.Query, queryNorm, req.Modalidade, req.Objetivo, req.Perfil, req.K, len(docs), string(resultadosBytes), userID); err != nil {
			logger.Error("Failed to save query results to cache", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":          true,
		"query_original":   req.Query,
		"query_usada":      queryUsada,
		"total_resultados": len(docs),
		"from_cache":       false,
		"cache_hits":       1,
		"resultados":       docs,
	})
}

func (h *RAGHandler) GetHistorico(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}
	}

	historico, err := h.repo.GetHistorico(r.Context(), limit)
	if err != nil {
		writeJSONError(w, "Failed to retrieve history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"historico": historico,
	})
}

func (h *RAGHandler) GetEstatisticas(w http.ResponseWriter, r *http.Request) {
	estatisticas, err := h.repo.GetEstatisticas(r.Context())
	if err != nil {
		writeJSONError(w, "Failed to retrieve statistics: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":      true,
		"estatisticas": estatisticas,
	})
}

func (h *RAGHandler) GetPopulares(w http.ResponseWriter, r *http.Request) {
	populares, err := h.repo.GetPopulares(r.Context())
	if err != nil {
		writeJSONError(w, "Failed to retrieve popular queries: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"populares": populares,
	})
}
