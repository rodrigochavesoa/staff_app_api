package services

import (
	"context"
	"errors"
	"staff_app/internal/domain"
)

// EmbeddingProvider generates vector embeddings for a given text.
type EmbeddingProvider interface {
	GenerateEmbeddings(ctx context.Context, text string) ([]float32, error)
}

// VectorStore abstracts similarity search inside a vector database.
type VectorStore interface {
	SearchSimilar(ctx context.Context, vector []float32, k int) ([]domain.KnowledgeDocument, error)
}

var (
	ErrNoServiceAvailable = errors.New("RAG service and local knowledge base are currently unavailable")
)
