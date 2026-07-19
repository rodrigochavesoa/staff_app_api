package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"staff_app/internal/domain"
)

const (
	AITrainingModeOff       = "off"
	AITrainingModeAssistive = "assistive"
	AITrainingModeRequired  = "required"
)

type GenerationRequest struct {
	Aluno       *domain.Aluno
	Frequencia  int
	Objetivo    string
	Nivel       string
	Restricoes  string
	Observacoes string
	LocalFicha  map[string]any
	Contexto    *AthleteTrainingContext
}

type AthleteTrainingContext struct {
	Complexidade     string                `json:"complexidade"`
	DadosUsados      []string              `json:"dados_usados"`
	Anamnese         *AnamneseTrainingHint `json:"anamnese,omitempty"`
	Historico        []TrainingHistoryHint `json:"historico,omitempty"`
	SVED             *SVEDTrainingHint     `json:"sved,omitempty"`
	Evidencias       []KnowledgeEvidence   `json:"evidencias,omitempty"`
	ResumoEstrutural map[string]any        `json:"resumo_estrutural,omitempty"`
}

type AnamneseTrainingHint struct {
	StatusAprovacao string  `json:"status_aprovacao"`
	Patologias      string  `json:"patologias,omitempty"`
	LesoesAtuais    string  `json:"lesoes_atuais,omitempty"`
	DoresCronicas   string  `json:"dores_cronicas,omitempty"`
	Medicamentos    string  `json:"medicamentos,omitempty"`
	RiskScore       float64 `json:"risk_score"`
	Experiencia     string  `json:"experiencia,omitempty"`
	Objetivo        string  `json:"objetivo,omitempty"`
}

type TrainingHistoryHint struct {
	ID        int64   `json:"id"`
	TipoFicha string  `json:"tipo_ficha"`
	Status    string  `json:"status"`
	Objetivo  string  `json:"objetivo,omitempty"`
	Nivel     string  `json:"nivel,omitempty"`
	IesScore  float64 `json:"ies_score,omitempty"`
	Volume    int     `json:"volume_sved,omitempty"`
	Data      string  `json:"data,omitempty"`
}

type SVEDTrainingHint struct {
	IesMedio       float64 `json:"ies_medio"`
	DensidadeMedia float64 `json:"densidade_media"`
	VolumeMedio    float64 `json:"volume_medio"`
	Fichas         int     `json:"fichas"`
}

type KnowledgeEvidence struct {
	Fonte      string   `json:"fonte"`
	Conteudo   string   `json:"conteudo"`
	Tags       []string `json:"tags,omitempty"`
	Relevancia float64  `json:"relevancia"`
}

type AIMetadata struct {
	AIUsed            bool     `json:"ai_used"`
	Provider          string   `json:"provider"`
	Model             string   `json:"model"`
	FallbackUsed      bool     `json:"fallback_used"`
	FallbackReason    string   `json:"fallback_reason,omitempty"`
	SafetyValidated   bool     `json:"safety_validated"`
	QualityValidated  bool     `json:"quality_validated"`
	ContextUsed       bool     `json:"context_used"`
	EvidenceCount     int      `json:"evidence_count"`
	Complexity        string   `json:"complexity,omitempty"`
	Warnings          []string `json:"warnings"`
	Sources           []string `json:"sources,omitempty"`
	ConfidenceScore   float64  `json:"confidence_score,omitempty"`
	EvidenceFallback  bool     `json:"evidence_fallback_used"`
	Validations       []string `json:"validations,omitempty"`
	EvidenceReasons   []string `json:"evidence_reasons,omitempty"`
}

type GenerationResult struct {
	Ficha    map[string]any
	Metadata AIMetadata
}

type TrainingProvider interface {
	Name() string
	Model() string
	Generate(ctx context.Context, req *GenerationRequest) (string, error)
}

type SafetyValidationResult struct {
	Passed        bool
	BlockedReason string
}

type TrainingSafetyValidator interface {
	Validate(ctx context.Context, req *GenerationRequest, rawJSON string) (*SafetyValidationResult, error)
}

type QualityValidationResult struct {
	Passed   bool
	Warnings []string
}

type TrainingQualityValidator interface {
	Validate(ctx context.Context, req *GenerationRequest, rawJSON string) (*QualityValidationResult, error)
}

type TelemetryData struct {
	Provider       string
	Model          string
	DurationMs     int64
	Success        bool
	FallbackUsed   bool
	FallbackReason string
	SafetyPassed   bool
	QualityPassed  bool
	TokensUsed     int
}

type TrainingTelemetryRecorder interface {
	Record(ctx context.Context, data *TelemetryData) error
}

type TrainingProviderChain struct {
	mode      string
	timeout   time.Duration
	providers []TrainingProvider
	safety    TrainingSafetyValidator
	quality   TrainingQualityValidator
	telemetry TrainingTelemetryRecorder
}

func NewTrainingProviderChain(mode string, timeout time.Duration, providers []TrainingProvider, safety TrainingSafetyValidator, quality TrainingQualityValidator, telemetry TrainingTelemetryRecorder) *TrainingProviderChain {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		mode = AITrainingModeAssistive
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if safety == nil {
		safety = DefaultTrainingSafetyValidator{}
	}
	if quality == nil {
		quality = DefaultTrainingQualityValidator{}
	}
	if telemetry == nil {
		telemetry = NoopTrainingTelemetryRecorder{}
	}
	return &TrainingProviderChain{
		mode:      mode,
		timeout:   timeout,
		providers: providers,
		safety:    safety,
		quality:   quality,
		telemetry: telemetry,
	}
}

func (c *TrainingProviderChain) Resolve(ctx context.Context, req *GenerationRequest) (*GenerationResult, error) {
	if req == nil {
		return nil, errors.New("generation request is required")
	}
	if req.LocalFicha == nil {
		return nil, errors.New("local deterministic ficha is required")
	}

	if c == nil || c.mode == AITrainingModeOff {
		return localTrainingResult(req.LocalFicha, false, "", req.Contexto)
	}

	var reasons []string
	providers := c.providers
	if len(providers) == 0 {
		reason := "no AI training providers configured"
		if c.mode == AITrainingModeRequired {
			return nil, errors.New(reason)
		}
		return localTrainingResult(req.LocalFicha, false, reason, req.Contexto)
	}

	for _, provider := range providers {
		if provider == nil {
			continue
		}
		if provider.Name() == "local" {
			return localTrainingResult(req.LocalFicha, len(reasons) > 0, strings.Join(reasons, "; "), req.Contexto)
		}

		providerCtx, cancel := context.WithTimeout(ctx, c.timeout)
		start := time.Now()
		raw, err := provider.Generate(providerCtx, req)
		cancel()
		duration := time.Since(start).Milliseconds()
		if err != nil {
			reason := fmt.Sprintf("provider %s failed: %v", provider.Name(), err)
			reasons = append(reasons, reason)
			c.record(ctx, &TelemetryData{
				Provider:       provider.Name(),
				Model:          provider.Model(),
				DurationMs:     duration,
				Success:        false,
				FallbackReason: err.Error(),
			})
			continue
		}

		safetyResult, err := c.safety.Validate(ctx, req, raw)
		if err != nil || safetyResult == nil || !safetyResult.Passed {
			reason := "safety validation failed"
			if safetyResult != nil && safetyResult.BlockedReason != "" {
				reason = safetyResult.BlockedReason
			}
			if err != nil {
				reason = err.Error()
			}
			reasons = append(reasons, fmt.Sprintf("provider %s rejected by safety: %s", provider.Name(), reason))
			c.record(ctx, &TelemetryData{
				Provider:       provider.Name(),
				Model:          provider.Model(),
				DurationMs:     duration,
				Success:        false,
				FallbackReason: reason,
				SafetyPassed:   false,
			})
			continue
		}

		qualityResult, err := c.quality.Validate(ctx, req, raw)
		qualityPassed := err == nil && qualityResult != nil && qualityResult.Passed
		var warnings []string
		if qualityResult != nil {
			warnings = qualityResult.Warnings
		}
		if err != nil {
			warnings = append(warnings, err.Error())
		}

		ficha, err := parseTrainingJSON(raw)
		if err != nil {
			reason := fmt.Sprintf("invalid provider JSON: %v", err)
			reasons = append(reasons, fmt.Sprintf("provider %s failed: %s", provider.Name(), reason))
			c.record(ctx, &TelemetryData{
				Provider:       provider.Name(),
				Model:          provider.Model(),
				DurationMs:     duration,
				Success:        false,
				FallbackReason: reason,
				SafetyPassed:   true,
			})
			continue
		}

		metadata := AIMetadata{
			AIUsed:           true,
			Provider:         provider.Name(),
			Model:            provider.Model(),
			FallbackUsed:     false,
			SafetyValidated:  true,
			QualityValidated: qualityPassed,
			ContextUsed:      req.Contexto != nil,
			EvidenceCount:    evidenceCount(req.Contexto),
			Complexity:       contextComplexity(req.Contexto),
			Warnings:         warnings,
		}
		EnrichMetadata(ctx, &metadata, req.Contexto)
		ficha["ai_metadata"] = metadata
		c.record(ctx, &TelemetryData{
			Provider:      provider.Name(),
			Model:         provider.Model(),
			DurationMs:    duration,
			Success:       true,
			SafetyPassed:  true,
			QualityPassed: qualityPassed,
		})
		return &GenerationResult{Ficha: ficha, Metadata: metadata}, nil
	}

	if c.mode == AITrainingModeRequired {
		return nil, fmt.Errorf("all AI training providers failed: %s", strings.Join(reasons, "; "))
	}
	return localTrainingResult(req.LocalFicha, len(reasons) > 0, strings.Join(reasons, "; "), req.Contexto)
}

func (c *TrainingProviderChain) record(ctx context.Context, data *TelemetryData) {
	if c == nil || c.telemetry == nil || data == nil {
		return
	}
	_ = c.telemetry.Record(ctx, data)
}

func localTrainingResult(local map[string]any, fallbackUsed bool, fallbackReason string, athleteCtx *AthleteTrainingContext) (*GenerationResult, error) {
	ficha := cloneMap(local)
	metadata := AIMetadata{
		AIUsed:           false,
		Provider:         "local",
		Model:            "local-deterministic",
		FallbackUsed:     fallbackUsed,
		FallbackReason:   fallbackReason,
		SafetyValidated:  true,
		QualityValidated: false,
		ContextUsed:      athleteCtx != nil,
		EvidenceCount:    evidenceCount(athleteCtx),
		Complexity:       contextComplexity(athleteCtx),
		Warnings:         []string{},
	}
	EnrichMetadata(context.Background(), &metadata, athleteCtx)
	ficha["ai_metadata"] = metadata
	return &GenerationResult{Ficha: ficha, Metadata: metadata}, nil
}

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src)+1)
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

type DefaultTrainingSafetyValidator struct{}

func (DefaultTrainingSafetyValidator) Validate(_ context.Context, req *GenerationRequest, rawJSON string) (*SafetyValidationResult, error) {
	if req == nil {
		return nil, errors.New("generation request is required")
	}
	ficha, err := parseTrainingJSON(rawJSON)
	if err != nil {
		return nil, err
	}

	clinicalText := requestClinicalText(req)
	for _, name := range collectExerciseNames(ficha) {
		exerciseName := strings.ToLower(name)
		for _, rule := range activeClinicalSafetyRules(req, clinicalText) {
			if containsAny(exerciseName, rule.BlockedExercises) {
				return &SafetyValidationResult{
					Passed:        false,
					BlockedReason: fmt.Sprintf("exercício %q bloqueado por %s", name, rule.Reason),
				}, nil
			}
		}
	}

	return &SafetyValidationResult{Passed: true}, nil
}

type clinicalSafetyRule struct {
	Reason           string
	Triggers         []string
	BlockedExercises []string
}

var clinicalSafetyRules = []clinicalSafetyRule{
	{
		Reason:   "restrição lombar/coluna",
		Triggers: []string{"lombar", "hérnia", "hernia", "coluna", "ciático", "ciatico", "compressão", "compressao"},
		BlockedExercises: []string{
			"agachamento livre", "levantamento terra", "stiff", "good morning", "terra sumô", "terra sumo",
			"desenvolvimento militar", "clean", "arranco",
		},
	},
	{
		Reason:   "restrição de joelho",
		Triggers: []string{"joelho", "patelar", "patela", "menisco", "condromalácia", "condromalacia", "patelofemoral", "ligamento cruzado"},
		BlockedExercises: []string{
			"agachamento livre", "agachamento profundo", "leg press", "afundo", "avanço", "avanco",
			"passada", "salto", "pliometria", "cadeira extensora",
		},
	},
	{
		Reason:   "restrição de ombro",
		Triggers: []string{"ombro", "manguito", "supraespinhal", "impacto subacromial", "bursite", "tendinite"},
		BlockedExercises: []string{
			"desenvolvimento militar", "desenvolvimento com barra", "arnold press", "remada alta",
			"elevação lateral", "elevacao lateral", "elevação frontal", "elevacao frontal", "supino inclinado",
		},
	},
	{
		Reason:   "restrição cervical/pescoço",
		Triggers: []string{"cervical", "pescoço", "pescoco", "hérnia cervical", "hernia cervical", "whiplash"},
		BlockedExercises: []string{
			"desenvolvimento militar", "shrugs", "encolhimento", "remada alta", "rollout",
			"abdominal com carga na nuca", "sit-up com peso",
		},
	},
	{
		Reason:   "restrição de punho/cotovelo",
		Triggers: []string{"punho", "cotovelo", "epicondilite", "túnel do carpo", "tunel do carpo", "tendinite de punho"},
		BlockedExercises: []string{
			"curl com barra", "rosca direta", "skull crusher", "fundo no banco", "dip",
			"flexão diamante", "flexao diamante", "clean",
		},
	},
	{
		Reason:   "restrição de tornozelo",
		Triggers: []string{"tornozelo", "entorse", "tendão de aquiles", "tendao de aquiles", "fascite"},
		BlockedExercises: []string{
			"salto", "box jump", "pliometria", "corrida em escada", "panturrilha em pé unilateral",
			"afundo", "avanço", "avanco",
		},
	},
	{
		Reason:   "restrição gestacional",
		Triggers: []string{"gestante", "gravidez", "grávida", "gravida", "prenatal", "pré-natal", "pre-natal"},
		BlockedExercises: []string{
			"hiit", "tabata", "burpee", "pliometria", "agachamento livre", "levantamento terra",
			"abdominal supra", "crunch", "prancha lateral dinâmica", "valsalva",
		},
	},
	{
		Reason:   "restrição cardiorrespiratória de alto risco",
		Triggers: []string{"cardiopatia", "cardíaco", "cardiaco", "hipertensão", "hipertensao", "arritmia", "dor no peito", "dispneia"},
		BlockedExercises: []string{
			"hiit", "tabata", "sprint", "burpee", "pliometria", "crossfit", "emom", "amrap",
		},
	},
}

func requestClinicalText(req *GenerationRequest) string {
	if req == nil {
		return ""
	}
	parts := []string{req.Restricoes, req.Observacoes}
	if req.Aluno != nil {
		parts = append(parts, req.Aluno.ExclusoesPermanentes)
	}
	if req.Contexto != nil && req.Contexto.Anamnese != nil {
		parts = append(parts,
			req.Contexto.Anamnese.Patologias,
			req.Contexto.Anamnese.LesoesAtuais,
			req.Contexto.Anamnese.DoresCronicas,
			req.Contexto.Anamnese.Medicamentos,
		)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func activeClinicalSafetyRules(req *GenerationRequest, clinicalText string) []clinicalSafetyRule {
	active := make([]clinicalSafetyRule, 0, len(clinicalSafetyRules))
	for _, rule := range clinicalSafetyRules {
		if containsAny(clinicalText, rule.Triggers) {
			active = append(active, rule)
		}
	}
	if req != nil && req.Contexto != nil && req.Contexto.Anamnese != nil && req.Contexto.Anamnese.RiskScore >= 3 {
		for _, rule := range clinicalSafetyRules {
			if rule.Reason == "restrição cardiorrespiratória de alto risco" && !hasClinicalRule(active, rule.Reason) {
				active = append(active, rule)
			}
		}
	}
	return active
}

func hasClinicalRule(rules []clinicalSafetyRule, reason string) bool {
	for _, rule := range rules {
		if rule.Reason == reason {
			return true
		}
	}
	return false
}

type DefaultTrainingQualityValidator struct{}

func (DefaultTrainingQualityValidator) Validate(_ context.Context, _ *GenerationRequest, rawJSON string) (*QualityValidationResult, error) {
	ficha, err := parseTrainingJSON(rawJSON)
	if err != nil {
		return nil, err
	}
	warnings := make([]string, 0)
	exercises := collectExerciseObjects(ficha)
	if len(exercises) == 0 {
		warnings = append(warnings, "nenhum exercício identificado no JSON gerado")
	}
	if _, ok := ficha["treinos"]; !ok {
		warnings = append(warnings, "JSON gerado não possui campo treinos")
	}
	requiredExerciseFields := []string{"grupo_muscular", "series", "repeticoes", "descanso", "cadencia"}
	for _, exercise := range exercises {
		name, _ := exercise["nome"].(string)
		for _, field := range requiredExerciseFields {
			value, ok := exercise[field]
			if !ok || isEmptyTrainingValue(value) {
				warnings = append(warnings, fmt.Sprintf("exercício %q sem campo %s", name, field))
			}
		}
	}
	return &QualityValidationResult{Passed: len(warnings) == 0, Warnings: warnings}, nil
}

type NoopTrainingTelemetryRecorder struct{}

func (NoopTrainingTelemetryRecorder) Record(context.Context, *TelemetryData) error {
	return nil
}

type LocalTrainingProvider struct{}

func (LocalTrainingProvider) Name() string  { return "local" }
func (LocalTrainingProvider) Model() string { return "local-deterministic" }
func (LocalTrainingProvider) Generate(context.Context, *GenerationRequest) (string, error) {
	return "", errors.New("local provider is resolved from deterministic ficha")
}

type GeminiTrainingProvider struct {
	APIKey  string
	ModelID string
	Client  *http.Client
}

func NewGeminiTrainingProvider(apiKey, model string, timeout time.Duration) *GeminiTrainingProvider {
	if model == "" {
		model = "gemini-2.5-flash-lite"
	}
	return &GeminiTrainingProvider{APIKey: apiKey, ModelID: model, Client: &http.Client{Timeout: timeout}}
}

func (p *GeminiTrainingProvider) Name() string  { return "gemini" }
func (p *GeminiTrainingProvider) Model() string { return p.ModelID }

func (p *GeminiTrainingProvider) Generate(ctx context.Context, req *GenerationRequest) (string, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return "", errors.New("GEMINI_API_KEY/GOOGLE_API_KEY is not configured")
	}
	body := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{{"text": buildTrainingPrompt(req)}},
			},
		},
		"generationConfig": map[string]any{"response_mime_type": "application/json"},
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", p.ModelID, p.APIKey)
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := postJSON(ctx, p.Client, url, nil, body, &resp); err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("gemini returned no content")
	}
	return resp.Candidates[0].Content.Parts[0].Text, nil
}

type OpenAITrainingProvider struct {
	APIKey  string
	ModelID string
	Client  *http.Client
}

func NewOpenAITrainingProvider(apiKey, model string, timeout time.Duration) *OpenAITrainingProvider {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAITrainingProvider{APIKey: apiKey, ModelID: model, Client: &http.Client{Timeout: timeout}}
}

func (p *OpenAITrainingProvider) Name() string  { return "openai" }
func (p *OpenAITrainingProvider) Model() string { return p.ModelID }

func (p *OpenAITrainingProvider) Generate(ctx context.Context, req *GenerationRequest) (string, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return "", errors.New("OPENAI_API_KEY is not configured")
	}
	body := map[string]any{
		"model": p.ModelID,
		"messages": []map[string]string{
			{"role": "system", "content": "Você gera apenas JSON válido para fichas de treino. Nunca inclua markdown."},
			{"role": "user", "content": buildTrainingPrompt(req)},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	headers := map[string]string{"Authorization": "Bearer " + p.APIKey}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := postJSON(ctx, p.Client, "https://api.openai.com/v1/chat/completions", headers, body, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("openai returned no choices")
	}
	return resp.Choices[0].Message.Content, nil
}

type ClaudeTrainingProvider struct {
	APIKey  string
	ModelID string
	Client  *http.Client
}

func NewClaudeTrainingProvider(apiKey, model string, timeout time.Duration) *ClaudeTrainingProvider {
	if model == "" {
		model = "claude-3-5-haiku-latest"
	}
	return &ClaudeTrainingProvider{APIKey: apiKey, ModelID: model, Client: &http.Client{Timeout: timeout}}
}

func (p *ClaudeTrainingProvider) Name() string  { return "claude" }
func (p *ClaudeTrainingProvider) Model() string { return p.ModelID }

func (p *ClaudeTrainingProvider) Generate(ctx context.Context, req *GenerationRequest) (string, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return "", errors.New("CLAUDE_API_KEY/ANTHROPIC_API_KEY is not configured")
	}
	body := map[string]any{
		"model":      p.ModelID,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": buildTrainingPrompt(req)},
		},
	}
	headers := map[string]string{
		"x-api-key":         p.APIKey,
		"anthropic-version": "2023-06-01",
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := postJSON(ctx, p.Client, "https://api.anthropic.com/v1/messages", headers, body, &resp); err != nil {
		return "", err
	}
	for _, part := range resp.Content {
		if part.Text != "" {
			return part.Text, nil
		}
	}
	return "", errors.New("claude returned no text content")
}

func buildTrainingPrompt(req *GenerationRequest) string {
	localBytes, _ := json.Marshal(req.LocalFicha)
	contextBytes, _ := json.Marshal(req.Contexto)
	alunoNome := ""
	if req.Aluno != nil {
		alunoNome = req.Aluno.Nome
	}
	return fmt.Sprintf(`Gere ou enriqueça uma ficha de musculação periodizada em JSON válido, mantendo exatamente a estrutura de segurança abaixo.

Regras obrigatórias:
- Retorne apenas JSON, sem markdown.
- Preserve os campos principais: tipo, frequencia, objetivo, nivel, observacoes, treinos.
- Cada treino deve conter letra, nome e exercicios.
- Cada exercício deve conter ao menos nome, grupo_muscular, series, repeticoes, descanso e cadencia quando esses dados existirem na base local.
- Nunca prescreva exercícios contraindicados pelas restrições clínicas.
- A IA pode enriquecer organização, nomes, observações e progressões, mas não pode remover restrições clínicas nem criar exercícios de risco.
- Se estiver em dúvida, mantenha a versão local.
- Não inclua explicações, raciocínio, Chain-of-Thought, comentários ou texto fora do JSON.

Formato mínimo esperado:
{"tipo":"periodizada","frequencia":3,"objetivo":"Hipertrofia","nivel":"intermediário","observacoes":"texto curto","treinos":[{"letra":"A","nome":"A - Exemplo","exercicios":[{"nome":"Exercício seguro","grupo_muscular":"Grupo","series":3,"repeticoes":"10-12","descanso":60,"cadencia":"4010"}]}]}

Aluno: %s
Frequência semanal: %d
Objetivo: %s
Nível: %s
Restrições: %s
Observações: %s

Contexto técnico estruturado do aluno e evidências:
%s

Ficha local segura para usar como base:
%s`, alunoNome, req.Frequencia, req.Objetivo, req.Nivel, req.Restricoes, req.Observacoes, string(contextBytes), string(localBytes))
}

func postJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, body any, dest any) error {
	if client == nil {
		client = http.DefaultClient
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return err
	}
	return nil
}

func parseTrainingJSON(raw string) (map[string]any, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)
	var ficha map[string]any
	if err := json.Unmarshal([]byte(clean), &ficha); err != nil {
		return nil, err
	}
	if len(ficha) == 0 {
		return nil, errors.New("empty training JSON")
	}
	return ficha, nil
}

func collectExerciseNames(v any) []string {
	var names []string
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			if name, ok := typed["nome"].(string); ok && name != "" {
				names = append(names, name)
			}
			for _, value := range typed {
				walk(value)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(v)
	return names
}

func collectExerciseObjects(v any) []map[string]any {
	var exercises []map[string]any
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			if name, ok := typed["nome"].(string); ok && name != "" {
				if _, isWorkout := typed["exercicios"]; !isWorkout {
					exercises = append(exercises, typed)
				}
			}
			for _, value := range typed {
				walk(value)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(v)
	return exercises
}

func isEmptyTrainingValue(v any) bool {
	if v == nil {
		return true
	}
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func evidenceCount(ctx *AthleteTrainingContext) int {
	if ctx == nil {
		return 0
	}
	return len(ctx.Evidencias)
}

func contextComplexity(ctx *AthleteTrainingContext) string {
	if ctx == nil {
		return ""
	}
	return ctx.Complexidade
}
