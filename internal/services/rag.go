package services

import (
	"context"
	"errors"
	"staff_app/internal/domain"
)

// EmbeddingProvider gera embeddings vetoriais para um texto.
type EmbeddingProvider interface {
	GenerateEmbeddings(ctx context.Context, text string) ([]float32, error)
}

// VectorStore abstrai busca por similaridade numa base vetorial.
type VectorStore interface {
	SearchSimilar(ctx context.Context, vector []float32, k int) ([]domain.KnowledgeDocument, error)
}

var (
	ErrNoServiceAvailable = errors.New("RAG service and local knowledge base are currently unavailable")
)
