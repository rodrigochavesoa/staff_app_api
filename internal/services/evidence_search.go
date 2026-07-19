package services

import (
	"context"
	"sort"
	"strings"

	"staff_app/internal/domain"
)

const hybridCandidateLimit = 20

// KnowledgeEvidenceSearcher finds knowledge snippets for gated evidence retrieval.
type KnowledgeEvidenceSearcher interface {
	Search(ctx context.Context, req EvidenceSearchRequest) ([]KnowledgeEvidence, error)
}

// LocalKnowledgeEvidenceSearcher ports the legacy single-loop local document search.
// Prefer HybridKnowledgeEvidenceSearcher for the periodized pipeline.
type LocalKnowledgeEvidenceSearcher struct {
	Docs LocalDocumentSearcher
}

func NewLocalKnowledgeEvidenceSearcher(docs LocalDocumentSearcher) *LocalKnowledgeEvidenceSearcher {
	return &LocalKnowledgeEvidenceSearcher{Docs: docs}
}

func (s *LocalKnowledgeEvidenceSearcher) Search(ctx context.Context, req EvidenceSearchRequest) ([]KnowledgeEvidence, error) {
	if s == nil || s.Docs == nil {
		return nil, nil
	}
	if strings.EqualFold(strings.TrimSpace(req.Complexity), "simples") {
		return nil, nil
	}

	queryParts := []string{req.Generation.Restricoes, req.Generation.Objetivo, req.Generation.Nivel, "musculação"}
	if req.Anamnese != nil {
		queryParts = append([]string{
			req.Anamnese.Patologias,
			req.Anamnese.LesoesAtuais,
			req.Anamnese.DoresCronicas,
		}, queryParts...)
	}
	candidates := filterNonEmpty(queryParts)
	candidates = append(candidates, "força", "segurança", "progressão")

	var docs []domain.KnowledgeDocument
	for _, query := range candidates {
		found, err := s.Docs.SearchLocalDocuments(ctx, query, "musculacao", 3)
		if err != nil {
			return nil, err
		}
		if len(found) > 0 {
			docs = found
			break
		}
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 3
	}
	evidencias := make([]KnowledgeEvidence, 0, len(docs))
	for _, doc := range docs {
		evidencias = append(evidencias, KnowledgeEvidence{
			Fonte:      doc.Fonte,
			Conteudo:   truncateEvidenceText(doc.Conteudo, 900),
			Tags:       append([]string(nil), doc.Tags...),
			Relevancia: doc.Relevancia,
		})
		if len(evidencias) >= topK {
			break
		}
	}
	return evidencias, nil
}

// HybridKnowledgeEvidenceSearcher combines lexical scoring over SQL candidates with
// optional vector search and deterministic rerank (spec §6.2–6.3).
type HybridKnowledgeEvidenceSearcher struct {
	Docs     LocalDocumentSearcher
	Embed    EmbeddingProvider // optional
	Store    VectorStore       // optional
	Reranker EvidenceReranker
}

func NewHybridKnowledgeEvidenceSearcher(docs LocalDocumentSearcher) *HybridKnowledgeEvidenceSearcher {
	return &HybridKnowledgeEvidenceSearcher{
		Docs:     docs,
		Reranker: DeterministicEvidenceReranker{},
	}
}

func (s *HybridKnowledgeEvidenceSearcher) Search(ctx context.Context, req EvidenceSearchRequest) ([]KnowledgeEvidence, error) {
	if s == nil || s.Docs == nil {
		return nil, nil
	}
	if strings.EqualFold(strings.TrimSpace(req.Complexity), "simples") {
		return nil, nil
	}

	queries := buildEvidenceQueries(req)
	if len(queries) == 0 {
		return nil, nil
	}
	modalidade := evidenceModalidade(req)

	restrictionToks := restrictionTokens(req)
	byKey := make(map[string]KnowledgeEvidence)

	for _, q := range queries {
		docs, err := s.listLexicalCandidates(ctx, q, modalidade)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			score := lexicalEvidenceScore(q, restrictionToks, modalidade, doc)
			key := evidenceDedupKey(doc.Fonte, doc.Conteudo)
			ev := KnowledgeEvidence{
				Fonte:      doc.Fonte,
				Conteudo:   truncateEvidenceText(doc.Conteudo, 900),
				Tags:       append([]string(nil), doc.Tags...),
				Relevancia: score,
			}
			if prev, ok := byKey[key]; !ok || score > prev.Relevancia {
				byKey[key] = ev
			}
		}
	}

	if s.Embed != nil && s.Store != nil {
		vec, err := s.Embed.GenerateEmbeddings(ctx, queries[0])
		if err == nil && len(vec) > 0 {
			hits, err := s.Store.SearchSimilar(ctx, vec, hybridCandidateLimit)
			if err == nil {
				for _, doc := range hits {
					score := clamp01(0.55 + 0.45*clamp01(doc.Relevancia))
					key := evidenceDedupKey(doc.Fonte, doc.Conteudo)
					ev := KnowledgeEvidence{
						Fonte:      doc.Fonte,
						Conteudo:   truncateEvidenceText(doc.Conteudo, 900),
						Tags:       append([]string(nil), doc.Tags...),
						Relevancia: score,
					}
					if prev, ok := byKey[key]; !ok || score > prev.Relevancia {
						byKey[key] = ev
					}
				}
			}
		}
		// Vector failures are non-fatal: lexical results still apply.
	}

	candidates := make([]KnowledgeEvidence, 0, len(byKey))
	for _, ev := range byKey {
		candidates = append(candidates, ev)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Relevancia == candidates[j].Relevancia {
			return candidates[i].Fonte < candidates[j].Fonte
		}
		return candidates[i].Relevancia > candidates[j].Relevancia
	})

	reranker := s.Reranker
	if reranker == nil {
		reranker = DeterministicEvidenceReranker{}
	}
	return reranker.Rerank(ctx, req, candidates), nil
}

func EvidenceTopK(complexity string) int {
	if strings.EqualFold(strings.TrimSpace(complexity), "complexo") {
		return 5
	}
	return 3
}

// listLexicalCandidates prefers token-OR SQL (LocalDocumentCandidateSource).
// Falls back to short-term SearchLocalDocuments when candidates are unavailable/empty.
func (s *HybridKnowledgeEvidenceSearcher) listLexicalCandidates(ctx context.Context, query, modalidade string) ([]domain.KnowledgeDocument, error) {
	if src, ok := s.Docs.(LocalDocumentCandidateSource); ok {
		docs, err := src.SearchLocalDocumentCandidates(ctx, query, modalidade, hybridCandidateLimit)
		if err != nil {
			return nil, err
		}
		if len(docs) > 0 {
			return docs, nil
		}
	}
	return s.listCandidatesExpanded(ctx, query, modalidade)
}

func (s *HybridKnowledgeEvidenceSearcher) listCandidatesExpanded(ctx context.Context, query, modalidade string) ([]domain.KnowledgeDocument, error) {
	byKey := make(map[string]domain.KnowledgeDocument)
	for _, term := range expandLexicalCandidateQueries(query) {
		docs, err := s.Docs.SearchLocalDocuments(ctx, term, modalidade, hybridCandidateLimit)
		if err != nil {
			return nil, err
		}
		for _, doc := range docs {
			key := evidenceDedupKey(doc.Fonte, doc.Conteudo)
			if _, ok := byKey[key]; !ok {
				byKey[key] = doc
			}
		}
		if len(byKey) >= hybridCandidateLimit {
			break
		}
	}
	out := make([]domain.KnowledgeDocument, 0, len(byKey))
	for _, doc := range byKey {
		out = append(out, doc)
	}
	return out, nil
}

func expandLexicalCandidateQueries(query string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 12)
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.Map(func(r rune) rune {
			if r == '%' || r == '_' {
				return -1
			}
			return r
		}, s)
		if len([]rune(s)) < 3 {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, tok := range tokenizeEvidence(query) {
		add(tok)
	}
	if len(out) == 0 {
		add(query)
	}
	for _, fb := range []string{"força", "segurança", "progressão"} {
		add(fb)
	}
	return out
}

func evidenceModalidade(req EvidenceSearchRequest) string {
	if m := strings.TrimSpace(req.Modalidade); m != "" {
		return m
	}
	return "musculacao"
}

func buildEvidenceQueries(req EvidenceSearchRequest) []string {
	complexity := strings.ToLower(strings.TrimSpace(req.Complexity))
	objetivo := strings.TrimSpace(req.Generation.Objetivo)
	nivel := strings.TrimSpace(req.Generation.Nivel)
	modalidade := evidenceModalidade(req)

	restricaoParts := []string{req.Generation.Restricoes}
	if req.Anamnese != nil {
		restricaoParts = append(restricaoParts,
			req.Anamnese.Patologias,
			req.Anamnese.LesoesAtuais,
			req.Anamnese.DoresCronicas,
		)
	}
	restricaoToks := tokenizeEvidence(strings.Join(filterNonEmpty(restricaoParts), " "))
	topRestricoes := restricaoToks
	if len(topRestricoes) > 2 {
		topRestricoes = topRestricoes[:2]
	}

	moderadoQuery := strings.TrimSpace(strings.Join(filterNonEmpty([]string{
		objetivo,
		nivel,
		modalidade,
		strings.Join(topRestricoes, " "),
	}), " "))

	if complexity != "complexo" {
		if moderadoQuery == "" {
			return []string{fallbackEvidenceQuery(objetivo)}
		}
		return []string{moderadoQuery}
	}

	// complexo: até 3 sub-queries (spec §6.4).
	clinical := strings.TrimSpace(strings.Join(filterNonEmpty(restricaoParts), " "))
	progressao := strings.TrimSpace(strings.Join(filterNonEmpty([]string{objetivo, nivel, "progressão", "segurança"}), " "))
	cruzada := strings.TrimSpace(strings.Join(filterNonEmpty([]string{modalidade, "reabilitação", "SVED"}), " "))

	queries := make([]string, 0, 3)
	for _, q := range []string{clinical, progressao, cruzada} {
		if q == "" {
			continue
		}
		dup := false
		for _, existing := range queries {
			if strings.EqualFold(existing, q) {
				dup = true
				break
			}
		}
		if !dup {
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 {
		return []string{fallbackEvidenceQuery(objetivo)}
	}
	return queries
}

func fallbackEvidenceQuery(objetivo string) string {
	obj := strings.TrimSpace(objetivo)
	if obj == "" {
		obj = "treino"
	}
	return "prescrição segura musculação " + obj
}

func lexicalEvidenceScore(query string, restrictionToks []string, modalidade string, doc domain.KnowledgeDocument) float64 {
	titulo := strings.ToLower(doc.Titulo)
	conteudo := strings.ToLower(doc.Conteudo)
	tagSet := make(map[string]struct{}, len(doc.Tags))
	for _, t := range doc.Tags {
		tagSet[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}

	var score float64
	for _, tok := range restrictionToks {
		if _, ok := tagSet[tok]; ok {
			score += 3
			continue
		}
		for tag := range tagSet {
			if strings.Contains(tag, tok) || strings.Contains(tok, tag) {
				score += 3
				break
			}
		}
	}

	for _, tok := range tokenizeEvidence(query) {
		if strings.Contains(titulo, tok) {
			score += 2
		}
		if strings.Contains(conteudo, tok) {
			score += 1
		}
	}

	if modalidade != "" && strings.EqualFold(strings.TrimSpace(doc.Modalidade), modalidade) {
		score += 2
	}

	// Soft saturation so ranking stays in a stable 0–1 band for rerank boosts.
	return clamp01(score / (score + 5.0))
}

func evidenceDedupKey(fonte, conteudo string) string {
	c := strings.TrimSpace(conteudo)
	if len(c) > 80 {
		c = c[:80]
	}
	return strings.ToLower(strings.TrimSpace(fonte)) + "|" + c
}

func filterNonEmpty(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
