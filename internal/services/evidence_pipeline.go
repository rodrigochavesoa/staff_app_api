package services

import (
	"context"
	"fmt"
	"strings"
)

// EvidencePipeline orquestra contexto SQL, classificação de complexidade
// e busca de evidências condicionada na geração de treino.
type EvidencePipeline struct {
	Structured *SQLStructuredContextLoader
	Classifier CaseComplexityClassifier
	Searcher   KnowledgeEvidenceSearcher
}

func NewEvidencePipeline(db ContextQueryDB, anam ActiveAnamneseFinder, docs LocalDocumentSearcher) *EvidencePipeline {
	return &EvidencePipeline{
		Structured: NewSQLStructuredContextLoader(db, anam),
		Classifier: DeterministicCaseComplexityClassifier{},
		Searcher:   NewHybridKnowledgeEvidenceSearcher(docs),
	}
}

func (p *EvidencePipeline) Build(ctx context.Context, alunoID int64, req GenerationRequest) (*AthleteTrainingContext, error) {
	if p == nil || p.Structured == nil {
		return nil, fmt.Errorf("evidence pipeline is not configured")
	}
	clf := p.Classifier
	if clf == nil {
		clf = DeterministicCaseComplexityClassifier{}
	}

	result, err := p.Structured.Load(ctx, alunoID, req)
	if err != nil {
		return nil, err
	}

	classified := clf.Classify(ctx, ClassificationInput{
		Context:     result,
		Frequencia:  req.Frequencia,
		Restricoes:  req.Restricoes,
		Observacoes: req.Observacoes,
	})
	result.Complexidade = classified.Complexity

	if classified.Complexity != "simples" && p.Searcher != nil {
		evidencias, err := p.Searcher.Search(ctx, EvidenceSearchRequest{
			Generation: req,
			Anamnese:   result.Anamnese,
			Complexity: classified.Complexity,
			Modalidade: "musculacao",
			TopK:       EvidenceTopK(classified.Complexity),
		})
		if err != nil {
			return nil, err
		}
		if len(evidencias) > 0 {
			result.Evidencias = evidencias
			result.DadosUsados = append(result.DadosUsados, "base_conhecimento")
		}
	}

	result.ResumoEstrutural = map[string]any{
		"tem_anamnese":      result.Anamnese != nil,
		"historico_count":   len(result.Historico),
		"evidencias_count":  len(result.Evidencias),
		"frequencia":        req.Frequencia,
		"restricoes_input":  strings.TrimSpace(req.Restricoes) != "",
		"observacoes_input": strings.TrimSpace(req.Observacoes) != "",
	}
	return result, nil
}
