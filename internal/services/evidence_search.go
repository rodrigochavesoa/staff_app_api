package services

import (
	"context"
	"fmt"

	"staff_app/internal/domain"
)

// KnowledgeEvidenceSearcher finds technical evidence for training generation.
type KnowledgeEvidenceSearcher interface {
	Search(ctx context.Context, req GenerationRequest, anam *AnamneseTrainingHint) ([]KnowledgeEvidence, error)
}

// LocalKnowledgeEvidenceSearcher ports the legacy loop over local document search.
type LocalKnowledgeEvidenceSearcher struct {
	Docs LocalDocumentSearcher
}

func NewLocalKnowledgeEvidenceSearcher(docs LocalDocumentSearcher) *LocalKnowledgeEvidenceSearcher {
	return &LocalKnowledgeEvidenceSearcher{Docs: docs}
}

func (s *LocalKnowledgeEvidenceSearcher) Search(ctx context.Context, req GenerationRequest, anam *AnamneseTrainingHint) ([]KnowledgeEvidence, error) {
	if s == nil || s.Docs == nil {
		return nil, fmt.Errorf("knowledge evidence searcher is not configured")
	}

	queryParts := []string{req.Restricoes, req.Objetivo, req.Nivel, "musculação"}
	if anam != nil {
		queryParts = append([]string{anam.Patologias, anam.LesoesAtuais, anam.DoresCronicas}, queryParts...)
	}
	candidates := nonEmptyStrings(queryParts)
	candidates = append(candidates, "força", "segurança", "progressão")

	var docs []domain.KnowledgeDocument
	for _, query := range candidates {
		found, err := s.Docs.SearchLocalDocuments(ctx, query, "musculacao", 3)
		if err != nil {
			return nil, fmt.Errorf("searching training evidence: %w", err)
		}
		if len(found) > 0 {
			docs = found
			break
		}
	}

	evidencias := make([]KnowledgeEvidence, 0, len(docs))
	for _, doc := range docs {
		evidencias = append(evidencias, KnowledgeEvidence{
			Fonte:      doc.Fonte,
			Conteudo:   truncateEvidenceText(doc.Conteudo, 900),
			Tags:       doc.Tags,
			Relevancia: doc.Relevancia,
		})
	}
	return evidencias, nil
}
