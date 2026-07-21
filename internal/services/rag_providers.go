package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"staff_app/internal/domain"
)

type OpenAIEmbeddingProvider struct {
	apiKey string
	client *http.Client
}

func NewOpenAIEmbeddingProvider(apiKey string) *OpenAIEmbeddingProvider {
	return &OpenAIEmbeddingProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *OpenAIEmbeddingProvider) GenerateEmbeddings(ctx context.Context, text string) ([]float32, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("openai API key is empty")
	}

	reqBody, err := json.Marshal(map[string]any{
		"input": text,
		"model": "text-embedding-3-small",
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("openai embeddings API error status %d: %v", resp.StatusCode, errResp)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("empty embeddings response from openai")
	}

	return result.Data[0].Embedding, nil
}

type ChromaVectorStore struct {
	chromaURL      string
	collectionName string
	client         *http.Client
}

func NewChromaVectorStore(chromaURL, collectionName string) *ChromaVectorStore {
	if collectionName == "" {
		collectionName = "knowledge_base"
	}
	return &ChromaVectorStore{
		chromaURL:      strings.TrimSuffix(chromaURL, "/"),
		collectionName: collectionName,
		client:         &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *ChromaVectorStore) getCollectionID(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/api/v1/collections/%s", s.chromaURL, s.collectionName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chromadb error fetching collection status %d", resp.StatusCode)
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	return res.ID, nil
}

func (s *ChromaVectorStore) SearchSimilar(ctx context.Context, vector []float32, k int) ([]domain.KnowledgeDocument, error) {
	if s.chromaURL == "" {
		return nil, fmt.Errorf("chroma URL is empty")
	}

	collectionID, err := s.getCollectionID(ctx)
	if err != nil {
		return nil, err
	}

	reqBody, err := json.Marshal(map[string]any{
		"query_embeddings": []any{vector},
		"n_results":        k,
	})
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/collections/%s/query", s.chromaURL, collectionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chromadb query error status %d", resp.StatusCode)
	}

	var res struct {
		Documents [][]string          `json:"documents"`
		Metadatas [][]map[string]any `json:"metadatas"`
		Distances [][]float64        `json:"distances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if len(res.Documents) == 0 || len(res.Documents[0]) == 0 {
		return nil, nil
	}

	var docs []domain.KnowledgeDocument
	for idx, docStr := range res.Documents[0] {
		var metadata map[string]any
		if len(res.Metadatas) > 0 && len(res.Metadatas[0]) > idx {
			metadata = res.Metadatas[0][idx]
		}

		var distance float64
		if len(res.Distances) > 0 && len(res.Distances[0]) > idx {
			distance = res.Distances[0][idx]
		}

		fonte := "Desconhecida"
		var tags []string
		if metadata != nil {
			if f, ok := metadata["fonte"].(string); ok {
				fonte = f
			}
			if tStr, ok := metadata["tags"].(string); ok {
				parts := strings.Split(tStr, ",")
				for _, p := range parts {
					t := strings.TrimSpace(p)
					if t != "" {
						tags = append(tags, t)
					}
				}
			}
		}

		// Converte distância L2 do Chroma em relevância (menor distância = maior relevância).
		relevance := 1.0
		if distance > 0 {
			relevance = 1.0 / (1.0 + distance)
		}

		docs = append(docs, domain.KnowledgeDocument{
			Rank:       idx + 1,
			Fonte:      fonte,
			Conteudo:   docStr,
			Tags:       tags,
			Relevancia: math.Round(relevance*100) / 100,
		})
	}

	return docs, nil
}
