package services

import (
	"context"
	"sort"
	"strings"
)

type EvidenceSearchRequest struct {
	Generation GenerationRequest
	Anamnese   *AnamneseTrainingHint
	Complexity string
	Modalidade string // defaults to musculacao when empty
	TopK       int
}

// EvidenceReranker reordena e corta candidatos de evidência.
type EvidenceReranker interface {
	Rerank(ctx context.Context, req EvidenceSearchRequest, candidates []KnowledgeEvidence) []KnowledgeEvidence
}

// DeterministicEvidenceReranker aplica bônus/penalidades da spec §6.3 sem aprendizado de máquina.
type DeterministicEvidenceReranker struct{}

func (DeterministicEvidenceReranker) Rerank(_ context.Context, req EvidenceSearchRequest, candidates []KnowledgeEvidence) []KnowledgeEvidence {
	if len(candidates) == 0 {
		return nil
	}
	topK := req.TopK
	if topK <= 0 {
		topK = EvidenceTopK(req.Complexity)
	}

	restrictionToks := restrictionTokens(req)
	objetivoTokens := tokenizeEvidence(req.Generation.Objetivo)

	scored := make([]KnowledgeEvidence, len(candidates))
	copy(scored, candidates)
	for i := range scored {
		rel := scored[i].Relevancia
		for _, tag := range scored[i].Tags {
			tagLower := strings.ToLower(strings.TrimSpace(tag))
			for _, tok := range restrictionToks {
				if tagLower == tok || strings.Contains(tagLower, tok) || strings.Contains(tok, tagLower) {
					rel += 0.15
					break
				}
			}
		}
		fonteLower := strings.ToLower(scored[i].Fonte)
		for _, tok := range objetivoTokens {
			if tok != "" && strings.Contains(fonteLower, tok) {
				rel += 0.10
				break
			}
		}
		scored[i].Relevancia = rel
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Relevancia == scored[j].Relevancia {
			return scored[i].Fonte < scored[j].Fonte
		}
		return scored[i].Relevancia > scored[j].Relevancia
	})

	deduped := make([]KnowledgeEvidence, 0, len(scored))
	for _, ev := range scored {
		if isDuplicateEvidencePrefix(deduped, ev.Conteudo) {
			continue
		}
		deduped = append(deduped, ev)
	}

	if len(deduped) > topK {
		deduped = deduped[:topK]
	}
	return deduped
}

func restrictionTokens(req EvidenceSearchRequest) []string {
	parts := []string{req.Generation.Restricoes}
	if req.Anamnese != nil {
		parts = append(parts, req.Anamnese.Patologias, req.Anamnese.LesoesAtuais, req.Anamnese.DoresCronicas)
	}
	return tokenizeEvidence(strings.Join(parts, " "))
}

func tokenizeEvidence(text string) []string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, ".,;:!?()[]{}\"'")
		if len(f) < 3 {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return out
}

func isDuplicateEvidencePrefix(existing []KnowledgeEvidence, content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return true
	}
	prefixLen := int(float64(len(content)) * 0.8)
	if prefixLen < 40 {
		prefixLen = len(content)
	}
	if prefixLen > len(content) {
		prefixLen = len(content)
	}
	prefix := content[:prefixLen]
	for _, ev := range existing {
		other := strings.TrimSpace(ev.Conteudo)
		if other == "" {
			continue
		}
		otherPrefixLen := prefixLen
		if otherPrefixLen > len(other) {
			otherPrefixLen = len(other)
		}
		if strings.HasPrefix(other, prefix) || strings.HasPrefix(content, other[:otherPrefixLen]) {
			return true
		}
	}
	return false
}
