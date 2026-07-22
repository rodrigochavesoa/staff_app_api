package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/repositories"

	"github.com/go-chi/chi/v5"
)

type AnamneseHandler struct {
	anamRepo   repositories.AnamneseRepository
	alunoRepo  repositories.AlunoRepository
	userRepo   repositories.UserRepository
	configRepo repositories.ConfiguracaoRepository
}

func NewAnamneseHandler(
	anam repositories.AnamneseRepository,
	aluno repositories.AlunoRepository,
	user repositories.UserRepository,
	config repositories.ConfiguracaoRepository,
) *AnamneseHandler {
	return &AnamneseHandler{
		anamRepo:   anam,
		alunoRepo:  aluno,
		userRepo:   user,
		configRepo: config,
	}
}

type anamneseSubmitRequest struct {
	Peso                      float64 `json:"peso"`
	Altura                    float64 `json:"altura"`
	Patologias                string  `json:"patologias"`
	Medicamentos              string  `json:"medicamentos"`
	LesoesAtuais              string  `json:"lesoes_atuais"`
	DoresCronicas             string  `json:"dores_cronicas"`
	ParqDoencaCardiaca        int     `json:"parq_doenca_cardiaca"`
	ParqDorPeito              int     `json:"parq_dor_peito"`
	ParqTontura               int     `json:"parq_tontura"`
	ParqProblemaOsseo         int     `json:"parq_problema_osseo"`
	ParqMedicamentoPressao    int     `json:"parq_medicamento_pressao"`
	ParqImpedimentoActivity   int     `json:"parq_impedimento_activity"`
	ExperienciaTreino         string  `json:"experiencia_treino"`
	ObjetivoPrincipal         string  `json:"objetivo_principal"`
	ContatoEmergenciaNome     string  `json:"contato_emergencia_nome"`
	ContatoEmergenciaTelefone string  `json:"contato_emergencia_telefone"`
}

func (h *AnamneseHandler) GenerateLink(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeJSONError(w, "Invalid Aluno ID", http.StatusBadRequest)
		return
	}

	aluno, err := h.alunoRepo.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if aluno == nil {
		writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
		return
	}

	var authUsername string = "admin"
	if u, ok := UserFromContext(r.Context()); ok && u != nil {
		authUsername = u.Username
	}

	now := time.Now()
	tokenStr, err := generateSecureToken()
	if err != nil {
		logger.Error("failed to generate anamnese token entropy", err)
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	expiraEm := now.Add(168 * time.Hour) // 7 dias

	t := &domain.AnamneseToken{
		Token:         tokenStr,
		PreRegistroID: nil,
		ExpiraEm:      expiraEm,
		Usado:         false,
		AlunoID:       &aluno.ID,
		AlunoNome:     aluno.Nome,
		AlunoEmail:    aluno.Email,
		CriadoEm:      now,
		CriadoPor:     authUsername,
		IpOrigem:      r.RemoteAddr,
	}

	if err := h.anamRepo.CreateToken(r.Context(), t); err != nil {
		writeJSONError(w, "Failed to generate token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	audit := &domain.AnamneseTokenAudit{
		Token:         tokenStr,
		AlunoID:       &aluno.ID,
		PreRegistroID: nil,
		Evento:        "GERADO",
		Ip:            r.RemoteAddr,
		UserAgent:     r.UserAgent(),
		Detalhes:      fmt.Sprintf("Token gerado manualmente pelo treinador %s para Aluno ID %d", authUsername, aluno.ID),
		DataEvento:    now,
	}
	_ = h.anamRepo.AddTokenAudit(r.Context(), audit)

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	hostLink := fmt.Sprintf("%s://%s/anamnese/submit/%s", scheme, r.Host, tokenStr)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":       true,
		"anamnese_link": hostLink,
	})
}

func (h *AnamneseHandler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	tokenStr := chi.URLParam(r, "token")
	if tokenStr == "" {
		writeJSONError(w, "Token parameter is required", http.StatusBadRequest)
		return
	}

	t, err := h.anamRepo.FindToken(r.Context(), tokenStr)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if t == nil {
		writeJSONError(w, "Token não encontrado", http.StatusNotFound)
		return
	}

	now := time.Now()
	if t.ExpiraEm.Before(now) {
		audit := &domain.AnamneseTokenAudit{
			Token:         t.Token,
			AlunoID:       t.AlunoID,
			PreRegistroID: t.PreRegistroID,
			Evento:        "EXPIRADO",
			Ip:            r.RemoteAddr,
			UserAgent:     r.UserAgent(),
			Detalhes:      "Tentativa de obter metadados de token expirado",
			DataEvento:    now,
		}
		_ = h.anamRepo.AddTokenAudit(r.Context(), audit)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "Token expirado",
			"message": "Este link de anamnese expirou.",
		})
		return
	}

	if t.Usado {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "Token já utilizado",
			"message": "Este link de anamnese já foi respondido e enviado.",
		})
		return
	}

	audit := &domain.AnamneseTokenAudit{
		Token:         t.Token,
		AlunoID:       t.AlunoID,
		PreRegistroID: t.PreRegistroID,
		Evento:        "VISUALIZADO",
		Ip:            r.RemoteAddr,
		UserAgent:     r.UserAgent(),
		Detalhes:      "Metadados de anamnese visualizados pelo aluno",
		DataEvento:    now,
	}
	_ = h.anamRepo.AddTokenAudit(r.Context(), audit)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":           t.Token,
		"pre_registro_id": t.PreRegistroID,
		"aluno_id":        t.AlunoID,
		"aluno_nome":      t.AlunoNome,
		"aluno_email":     t.AlunoEmail,
		"expira_em":       t.ExpiraEm,
	})
}

func (h *AnamneseHandler) Submit(w http.ResponseWriter, r *http.Request) {
	tokenStr := chi.URLParam(r, "token")
	if tokenStr == "" {
		writeJSONError(w, "Token parameter is required", http.StatusBadRequest)
		return
	}

	t, err := h.anamRepo.FindToken(r.Context(), tokenStr)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if t == nil {
		writeJSONError(w, "Token não encontrado", http.StatusNotFound)
		return
	}

	now := time.Now()
	if t.ExpiraEm.Before(now) {
		writeJSONError(w, "Este link de anamnese expirou", http.StatusGone)
		return
	}
	if t.Usado {
		writeJSONError(w, "Este link de anamnese já foi respondido", http.StatusConflict)
		return
	}

	var req anamneseSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.Peso <= 0 || req.Altura <= 0 {
		writeJSONError(w, "Altura e Peso válidos são obrigatórios", http.StatusBadRequest)
		return
	}

	// RiskScoreCached: soma simples do PAR-Q (0–6).
	parqScore := req.ParqDoencaCardiaca + req.ParqDorPeito + req.ParqTontura + req.ParqProblemaOsseo + req.ParqMedicamentoPressao + req.ParqImpedimentoActivity

	a := &domain.Anamnese{
		AlunoID:                   t.AlunoID,
		DataNascimento:            "",
		Idade:                     0,
		Sexo:                      "",
		Altura:                    req.Altura,
		Peso:                      req.Peso,
		Telefone:                  t.AlunoEmail, // placeholder até o aluno informar telefone
		Email:                     t.AlunoEmail,
		Patologias:                req.Patologias,
		Medicamentos:              req.Medicamentos,
		LesoesAtuais:              req.LesoesAtuais,
		DoresCronicas:             req.DoresCronicas,
		ParqDoencaCardiaca:        req.ParqDoencaCardiaca,
		ParqDorPeito:              req.ParqDorPeito,
		ParqTontura:               req.ParqTontura,
		ParqProblemaOsseo:         req.ParqProblemaOsseo,
		ParqMedicamentoPressao:    req.ParqMedicamentoPressao,
		ParqImpedimentoActivity:   req.ParqImpedimentoActivity,
		ExperienciaTreino:         req.ExperienciaTreino,
		ObjetivoPrincipal:         req.ObjetivoPrincipal,
		ContatoEmergenciaNome:     req.ContatoEmergenciaNome,
		ContatoEmergenciaTelefone: req.ContatoEmergenciaTelefone,
		RiskScoreCached:           float64(parqScore),
		PreenchidoPor:             "aluno",
		Ativa:                     false, // inativa até aprovação admin
		CriadoEm:                  now,
		PreRegistroID:             t.PreRegistroID,
		StatusAprovacao:           "pendente",
		TokenOrigem:               &t.Token,
	}

	if t.AlunoID != nil {
		aluno, err := h.alunoRepo.GetByID(r.Context(), *t.AlunoID)
		if err == nil && aluno != nil {
			a.Sexo = aluno.Sexo
			a.Idade = aluno.Idade
			a.Telefone = aluno.Telefone
		}
	}

	if err := h.anamRepo.Create(r.Context(), a); err != nil {
		writeJSONError(w, "Failed to create anamnese: "+err.Error(), http.StatusInternalServerError)
		return
	}

	t.Usado = true
	t.UsadoEm = &now
	t.IpSubmissao = r.RemoteAddr
	t.AnamneseID = &a.ID

	if err := h.anamRepo.UpdateToken(r.Context(), t); err != nil {
		writeJSONError(w, "Failed to update token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	audit := &domain.AnamneseTokenAudit{
		Token:         t.Token,
		AlunoID:       t.AlunoID,
		PreRegistroID: t.PreRegistroID,
		Evento:        "SUBMETIDO",
		Ip:            r.RemoteAddr,
		UserAgent:     r.UserAgent(),
		Detalhes:      fmt.Sprintf("Anamnese submetida e associada ao ID %d", a.ID),
		DataEvento:    now,
	}
	_ = h.anamRepo.AddTokenAudit(r.Context(), audit)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"message":     "Anamnese submetida com sucesso! Suas respostas foram encaminhadas para análise clínica da comissão técnica.",
		"anamnese_id": a.ID,
	})
}

func (h *AnamneseHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.anamRepo.List(r.Context())
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if list == nil {
		list = []domain.Anamnese{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total":     len(list),
		"anamneses": list,
	})
}

func (h *AnamneseHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	list, err := h.anamRepo.ListPending(r.Context())
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if list == nil {
		list = []domain.Anamnese{}
	}

	type pendingItem struct {
		ID        int64     `json:"id"`
		AlunoID   *int64    `json:"aluno_id"`
		AlunoNome string    `json:"aluno_nome"`
		RiskScore float64   `json:"risk_score"`
		ParqScore int       `json:"parq_score"`
		CriadoEm  time.Time `json:"criado_em"`
	}

	var items []pendingItem
	for _, a := range list {
		alNome := "Desconhecido"
		if a.AlunoID != nil {
			if al, err := h.alunoRepo.GetByID(r.Context(), *a.AlunoID); err == nil && al != nil {
				alNome = al.Nome
			}
		}
		items = append(items, pendingItem{
			ID:        a.ID,
			AlunoID:   a.AlunoID,
			AlunoNome: alNome,
			RiskScore: a.RiskScoreCached,
			ParqScore: a.ParqDoencaCardiaca + a.ParqDorPeito + a.ParqTontura + a.ParqProblemaOsseo + a.ParqMedicamentoPressao + a.ParqImpedimentoActivity,
			CriadoEm:  a.CriadoEm,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total":     len(items),
		"anamneses": items,
	})
}

func (h *AnamneseHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeJSONError(w, "Invalid ID parameter", http.StatusBadRequest)
		return
	}

	a, err := h.anamRepo.FindByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if a == nil {
		writeJSONError(w, "Anamnese não encontrada", http.StatusNotFound)
		return
	}

	alNome := "Desconhecido"
	if a.AlunoID != nil {
		if al, err := h.alunoRepo.GetByID(r.Context(), *a.AlunoID); err == nil && al != nil {
			alNome = al.Nome
		}
	}

	// IMC = peso / (altura²).
	imc := 0.0
	if a.Altura > 0 {
		imc = a.Peso / (a.Altura * a.Altura)
	}

	parqScore := a.ParqDoencaCardiaca + a.ParqDorPeito + a.ParqTontura + a.ParqProblemaOsseo + a.ParqMedicamentoPressao + a.ParqImpedimentoActivity

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":             a.ID,
		"aluno_id":       a.AlunoID,
		"aluno_nome":     alNome,
		"altura":         a.Altura,
		"peso":           a.Peso,
		"imc":            imc,
		"patologias":     a.Patologias,
		"medicamentos":   a.Medicamentos,
		"lesoes_atuais":  a.LesoesAtuais,
		"dores_cronicas": a.DoresCronicas,
		"parq": map[string]any{
			"doenca_cardiaca":      a.ParqDoencaCardiaca,
			"dor_peito":            a.ParqDorPeito,
			"tontura":              a.ParqTontura,
			"problema_osseo":       a.ParqProblemaOsseo,
			"medicamento_pressao":  a.ParqMedicamentoPressao,
			"impedimento_activity": a.ParqImpedimentoActivity,
			"score":                parqScore,
		},
		"experiencia_treino": a.ExperienciaTreino,
		"objetivo_principal": a.ObjetivoPrincipal,
		"contato_emergencia": map[string]any{
			"nome":     a.ContatoEmergenciaNome,
			"telefone": a.ContatoEmergenciaTelefone,
		},
		"risk_score_cached": a.RiskScoreCached,
		"status_aprovacao":  a.StatusAprovacao,
		"criado_em":         a.CriadoEm,
	})
}

func (h *AnamneseHandler) Approve(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeJSONError(w, "Invalid ID parameter", http.StatusBadRequest)
		return
	}

	a, err := h.anamRepo.FindByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if a == nil {
		writeJSONError(w, "Anamnese não encontrada", http.StatusNotFound)
		return
	}

	var authUsername string = "admin"
	if u, ok := UserFromContext(r.Context()); ok && u != nil {
		authUsername = u.Username
	}

	now := time.Now()

	// Aprova e desativa anamneses ativas anteriores do mesmo aluno.
	if a.AlunoID != nil {
		if err := h.anamRepo.DeactivateAllPreviousForAluno(r.Context(), *a.AlunoID, a.ID); err != nil {
			writeJSONError(w, "Failed to deactivate old records: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	a.StatusAprovacao = "aprovada"
	a.Ativa = true
	a.AprovadoPor = &authUsername
	a.AprovadoEm = &now

	if err := h.anamRepo.Update(r.Context(), a); err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if a.TokenOrigem != nil {
		audit := &domain.AnamneseTokenAudit{
			Token:         *a.TokenOrigem,
			AlunoID:       a.AlunoID,
			PreRegistroID: a.PreRegistroID,
			Evento:        "ANAMNESE_APROVADA",
			Ip:            r.RemoteAddr,
			UserAgent:     r.UserAgent(),
			Detalhes:      fmt.Sprintf("Anamnese ID %d aprovada por %s", a.ID, authUsername),
			DataEvento:    now,
		}
		_ = h.anamRepo.AddTokenAudit(r.Context(), audit)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AnamneseHandler) Reject(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeJSONError(w, "Invalid ID parameter", http.StatusBadRequest)
		return
	}

	a, err := h.anamRepo.FindByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if a == nil {
		writeJSONError(w, "Anamnese não encontrada", http.StatusNotFound)
		return
	}

	var req rejectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	req.Motivo = strings.TrimSpace(req.Motivo)
	if req.Motivo == "" {
		writeJSONError(w, "Motivo da rejeição é obrigatório", http.StatusBadRequest)
		return
	}

	var authUsername string = "admin"
	if u, ok := UserFromContext(r.Context()); ok && u != nil {
		authUsername = u.Username
	}

	now := time.Now()
	a.StatusAprovacao = "rejeitada"
	a.Ativa = false
	a.AprovadoPor = &authUsername
	a.AprovadoEm = &now
	a.MotivoRejeicao = &req.Motivo

	if err := h.anamRepo.Update(r.Context(), a); err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if a.TokenOrigem != nil {
		audit := &domain.AnamneseTokenAudit{
			Token:         *a.TokenOrigem,
			AlunoID:       a.AlunoID,
			PreRegistroID: a.PreRegistroID,
			Evento:        "ANAMNESE_REJEITADA",
			Ip:            r.RemoteAddr,
			UserAgent:     r.UserAgent(),
			Detalhes:      fmt.Sprintf("Anamnese ID %d rejeitada por %s. Motivo: %s", a.ID, authUsername, req.Motivo),
			DataEvento:    now,
		}
		_ = h.anamRepo.AddTokenAudit(r.Context(), audit)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Anamnese rejeitada e devolvida para revisão.",
	})
}

func (h *AnamneseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeJSONError(w, "Invalid ID parameter", http.StatusBadRequest)
		return
	}

	a, err := h.anamRepo.FindByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if a == nil {
		writeJSONError(w, "Anamnese não encontrada", http.StatusNotFound)
		return
	}

	if err := h.anamRepo.Delete(r.Context(), id); err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AnamneseHandler) ReenviarEmail(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeJSONError(w, "Invalid Aluno ID", http.StatusBadRequest)
		return
	}

	aluno, err := h.alunoRepo.GetByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if aluno == nil {
		writeJSONError(w, "Aluno não encontrado", http.StatusNotFound)
		return
	}

	var authUsername string = "admin"
	if u, ok := UserFromContext(r.Context()); ok && u != nil {
		authUsername = u.Username
	}

	now := time.Now()
	tokenStr, err := generateSecureToken()
	if err != nil {
		logger.Error("failed to generate anamnese token entropy", err)
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	expiraEm := now.Add(168 * time.Hour) // 7 dias

	t := &domain.AnamneseToken{
		Token:         tokenStr,
		PreRegistroID: nil,
		ExpiraEm:      expiraEm,
		Usado:         false,
		AlunoID:       &aluno.ID,
		AlunoNome:     aluno.Nome,
		AlunoEmail:    aluno.Email,
		CriadoEm:      now,
		CriadoPor:     authUsername,
		IpOrigem:      r.RemoteAddr,
	}

	if err := h.anamRepo.CreateToken(r.Context(), t); err != nil {
		writeJSONError(w, "Failed to generate token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	auditGerado := &domain.AnamneseTokenAudit{
		Token:         tokenStr,
		AlunoID:       &aluno.ID,
		PreRegistroID: nil,
		Evento:        "GERADO",
		Ip:            r.RemoteAddr,
		UserAgent:     r.UserAgent(),
		Detalhes:      fmt.Sprintf("Token gerado manualmente via reenvio de e-mail pelo treinador %s", authUsername),
		DataEvento:    now,
	}
	_ = h.anamRepo.AddTokenAudit(r.Context(), auditGerado)

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	err = sendAnamneseEmail(r.Context(), h.configRepo, h.anamRepo, t, r.Host, scheme)
	if err != nil {
		writeJSONError(w, "Falha ao enviar e-mail via SMTP: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "E-mail de anamnese reenviado com sucesso!",
	})
}

func sendAnamneseEmail(ctx context.Context, configRepo repositories.ConfiguracaoRepository, anamRepo repositories.AnamneseRepository, token *domain.AnamneseToken, host string, scheme string) error {
	configs, err := configRepo.List(ctx)
	if err != nil {
		errFailed := fmt.Errorf("failed to list configurations: %w", err)
		logEmailFalhou(ctx, anamRepo, token, errFailed)
		return errFailed
	}

	configMap := make(map[string]string)
	for _, c := range configs {
		configMap[c.Chave] = c.Valor
	}

	if configMap["SMTP_ENABLED"] != "true" {
		errFailed := fmt.Errorf("serviço de e-mail (SMTP_ENABLED) desabilitado")
		logEmailFalhou(ctx, anamRepo, token, errFailed)
		return errFailed
	}

	hostSMTP := configMap["SMTP_HOST"]
	portStr := configMap["SMTP_PORT"]
	user := configMap["SMTP_USER"]
	password := configMap["SMTP_PASSWORD"]
	fromEmail := configMap["SMTP_FROM_EMAIL"]
	fromName := configMap["SMTP_FROM_NAME"]

	if hostSMTP == "" || portStr == "" || fromEmail == "" {
		errFailed := fmt.Errorf("configurações SMTP incompletas no sistema")
		logEmailFalhou(ctx, anamRepo, token, errFailed)
		return errFailed
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		errFailed := fmt.Errorf("porta SMTP inválida: %s", portStr)
		logEmailFalhou(ctx, anamRepo, token, errFailed)
		return errFailed
	}

	hostLink := fmt.Sprintf("%s://%s/anamnese/submit/%s", scheme, host, token.Token)

	subject := "Preenchimento de Anamnese - Sistema RC Staff"
	body := fmt.Sprintf(
		"Olá, %s,\n\n"+
			"Seu treinador solicitou o preenchimento/atualização do seu questionário de saúde (Anamnese).\n\n"+
			"Por favor, acesse o link abaixo para responder às perguntas:\n"+
			"%s\n\n"+
			"Este link é válido por 7 dias.\n\n"+
			"Atenciosamente,\n"+
			"Equipe RC Staff\n",
		token.AlunoNome, hostLink,
	)

	if err := sendEmailRaw(hostSMTP, port, user, password, fromEmail, fromName, token.AlunoEmail, subject, body); err != nil {
		errFailed := fmt.Errorf("erro no envio SMTP: %w", err)
		logEmailFalhou(ctx, anamRepo, token, errFailed)
		return errFailed
	}

	audit := &domain.AnamneseTokenAudit{
		Token:         token.Token,
		AlunoID:       token.AlunoID,
		PreRegistroID: token.PreRegistroID,
		Evento:        "ENVIADO_EMAIL",
		Ip:            "system",
		UserAgent:     "Go SMTP Service",
		Detalhes:      fmt.Sprintf("E-mail de anamnese enviado com sucesso para %s", token.AlunoEmail),
		DataEvento:    time.Now(),
	}
	_ = anamRepo.AddTokenAudit(ctx, audit)

	return nil
}

func logEmailFalhou(ctx context.Context, anamRepo repositories.AnamneseRepository, token *domain.AnamneseToken, err error) {
	audit := &domain.AnamneseTokenAudit{
		Token:         token.Token,
		AlunoID:       token.AlunoID,
		PreRegistroID: token.PreRegistroID,
		Evento:        "EMAIL_FALHOU",
		Ip:            "system",
		UserAgent:     "Go SMTP Service",
		Detalhes:      fmt.Sprintf("Falha no envio de e-mail: %v", err),
		DataEvento:    time.Now(),
	}
	_ = anamRepo.AddTokenAudit(ctx, audit)
}
