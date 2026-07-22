package http

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/repositories"

	"github.com/go-chi/chi/v5"
)

// randomRead is crypto/rand.Read by default; tests may stub it to simulate entropy failure.
var randomRead = rand.Read

type PreCadastroHandler struct {
	preRepo    repositories.PreRegistroRepository
	alunoRepo  repositories.AlunoRepository
	planoRepo  repositories.PlanoRepository
	anamRepo   repositories.AnamneseRepository
	configRepo repositories.ConfiguracaoRepository
}

func NewPreCadastroHandler(
	pre repositories.PreRegistroRepository,
	aluno repositories.AlunoRepository,
	plano repositories.PlanoRepository,
	anam repositories.AnamneseRepository,
	config repositories.ConfiguracaoRepository,
) *PreCadastroHandler {
	return &PreCadastroHandler{
		preRepo:    pre,
		alunoRepo:  aluno,
		planoRepo:  plano,
		anamRepo:   anam,
		configRepo: config,
	}
}

type preCadastroRequest struct {
	Nome           string `json:"nome"`
	Email          string `json:"email"`
	Telefone       string `json:"telefone"`
	DataNascimento string `json:"data_nascimento"`
	Genero         string `json:"genero"`
	PlanoID        int64  `json:"plano_id"`
}

func generateSecureToken() (string, error) {
	b := make([]byte, 16)
	if _, err := randomRead(b); err != nil {
		return "", fmt.Errorf("generate secure token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (h *PreCadastroHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req preCadastroRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	req.Nome = strings.TrimSpace(req.Nome)
	req.Email = strings.TrimSpace(req.Email)
	req.Telefone = strings.TrimSpace(req.Telefone)
	req.DataNascimento = strings.TrimSpace(req.DataNascimento)
	req.Genero = strings.TrimSpace(req.Genero)

	if req.Nome == "" || req.Email == "" || req.DataNascimento == "" || req.PlanoID <= 0 {
		writeJSONError(w, "Nome, Email, Data de Nascimento e PlanoID são obrigatórios", http.StatusBadRequest)
		return
	}

	existingPre, err := h.preRepo.FindByEmail(r.Context(), req.Email)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if existingPre != nil {
		writeJSONError(w, "Já existe uma solicitação de pré-cadastro pendente ou ativa para este e-mail", http.StatusConflict)
		return
	}

	existingAluno, err := h.alunoRepo.GetByEmail(r.Context(), req.Email)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if existingAluno != nil {
		writeJSONError(w, "Este e-mail já pertence a um aluno cadastrado e ativo", http.StatusConflict)
		return
	}

	plano, err := h.planoRepo.GetByID(r.Context(), req.PlanoID)
	if err != nil || plano == nil || !plano.Ativo {
		writeJSONError(w, "Plano inválido ou inativo", http.StatusBadRequest)
		return
	}

	// Expiração padrão do pré-registro: 72h.
	expHrs := 72
	now := time.Now()
	expiraEm := now.Add(time.Duration(expHrs) * time.Hour)

	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.Split(fwd, ",")[0]
	}
	ua := r.UserAgent()

	p := &domain.PreRegistro{
		Nome:           req.Nome,
		Email:          req.Email,
		Telefone:       req.Telefone,
		DataNascimento: req.DataNascimento,
		Genero:         req.Genero,
		PlanoID:        &req.PlanoID,
		PlanoValor:     &plano.PrecoDefault,
		IpOrigem:       ip,
		UserAgent:      ua,
		ExpiraEm:       expiraEm,
		CriadoEm:       now,
		Usado:          false,
		Status:         "aguardando_aprovacao",
	}

	if err := h.preRepo.Create(r.Context(), p); err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	audit := &domain.PreRegistroAudit{
		PreRegistroID: p.ID,
		Evento:        "CRIADO",
		Detalhes:      "Pré-cadastro criado com sucesso. IP: " + ip,
		IpOrigem:      ip,
		UserAgent:     ua,
		CriadoEm:      now,
	}
	_ = h.preRepo.AddAudit(r.Context(), audit)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":         true,
		"message":         "Pré-cadastro recebido com sucesso! Aguarde a análise da nossa equipe técnica.",
		"pre_registro_id": p.ID,
	})
}

func (h *PreCadastroHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	nome := r.URL.Query().Get("nome")

	list, err := h.preRepo.List(r.Context(), status, nome)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if list == nil {
		list = []domain.PreRegistro{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total":         len(list),
		"pre_registros": list,
	})
}

func (h *PreCadastroHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeJSONError(w, "Invalid ID parameter", http.StatusBadRequest)
		return
	}

	p, err := h.preRepo.FindByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		writeJSONError(w, "Pré-cadastro não encontrado", http.StatusNotFound)
		return
	}

	trail, err := h.preRepo.GetAuditTrail(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	type auditResponse struct {
		Evento   string    `json:"evento"`
		Usuario  string    `json:"usuario"`
		Detalhes string    `json:"detalhes"`
		Data     time.Time `json:"data"`
	}

	var auditTrail []auditResponse
	for _, a := range trail {
		userLabel := "público"
		if a.UsuarioID != nil {
			userLabel = fmt.Sprintf("User ID: %d", *a.UsuarioID)
		}
		auditTrail = append(auditTrail, auditResponse{
			Evento:   a.Evento,
			Usuario:  userLabel,
			Detalhes: a.Detalhes,
			Data:     a.CriadoEm,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":              p.ID,
		"nome":            p.Nome,
		"email":           p.Email,
		"telefone":        p.Telefone,
		"data_nascimento": p.DataNascimento,
		"genero":          p.Genero,
		"plano_id":        p.PlanoID,
		"plano_valor":     p.PlanoValor,
		"status":          p.Status,
		"ip_origem":       p.IpOrigem,
		"user_agent":      p.UserAgent,
		"criado_em":       p.CriadoEm,
		"audit_trail":     auditTrail,
	})
}

func (h *PreCadastroHandler) Approve(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeJSONError(w, "Invalid ID parameter", http.StatusBadRequest)
		return
	}

	p, err := h.preRepo.FindByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		writeJSONError(w, "Pré-cadastro não encontrado", http.StatusNotFound)
		return
	}

	if p.Status != "aguardando_aprovacao" {
		writeJSONError(w, "Este pré-cadastro já possui status '"+p.Status+"'", http.StatusBadRequest)
		return
	}

	// Idade a partir de data_nascimento (YYYY-MM-DD).
	idade := 0
	if birthTime, err := time.Parse("2006-01-02", p.DataNascimento); err == nil {
		idade = time.Now().Year() - birthTime.Year()
		if time.Now().YearDay() < birthTime.YearDay() {
			idade--
		}
	}

	var authUserID int64
	var authUsername string = "admin"
	if u, ok := r.Context().Value(userContextKey{}).(*domain.User); ok && u != nil {
		authUserID = u.ID
		authUsername = u.Username
	}

	now := time.Now()
	aluno := &domain.Aluno{
		Nome:                p.Nome,
		Idade:               idade,
		Sexo:                p.Genero,
		Email:               p.Email,
		Telefone:            p.Telefone,
		Objetivo:            "",
		Turma:               "Turma Geral",
		PlanoID:             p.PlanoID,
		PlanoValor:          p.PlanoValor,
		PlanoPago:           false,
		PlanoAtivo:          true,
		CadastroAprovado:    true,
		CadastroAprovadoPor: &authUserID,
		CadastroAprovadoEm:  &now,
		PreRegistroID:       &p.ID,
		Ativo:               true,
	}

	if err := h.alunoRepo.Create(r.Context(), aluno); err != nil {
		writeJSONError(w, "Failed to create aluno record: "+err.Error(), http.StatusInternalServerError)
		return
	}

	p.Status = "aprovado"
	p.AprovadoPor = &authUserID
	p.AprovadoEm = &now
	p.AlunoIDCriado = &aluno.ID
	p.Usado = true

	if err := h.preRepo.Update(r.Context(), p); err != nil {
		writeJSONError(w, "Failed to update pre-registration status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	audit := &domain.PreRegistroAudit{
		PreRegistroID: p.ID,
		Evento:        "APROVADO",
		UsuarioID:     &authUserID,
		Detalhes:      fmt.Sprintf("Aprovado pelo treinador. Criado Aluno ID: %d", aluno.ID),
		IpOrigem:      r.RemoteAddr,
		UserAgent:     r.UserAgent(),
		CriadoEm:      now,
	}
	_ = h.preRepo.AddAudit(r.Context(), audit)

	configs, _ := h.configRepo.List(r.Context())
	autoGenerate := true
	autoSend := false
	for _, c := range configs {
		if c.Chave == "AUTO_GENERATE_ANAMNESE_ON_APPROVE" {
			autoGenerate = c.Valor == "true"
		}
		if c.Chave == "AUTO_SEND_ANAMNESE_EMAIL" {
			autoSend = c.Valor == "true"
		}
	}

	var hostLink string
	var emailEnviado *bool
	var emailErro string
	if autoGenerate {
		tokenStr, err := generateSecureToken()
		if err != nil {
			logger.Error("failed to generate anamnese token entropy", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		anamToken := &domain.AnamneseToken{
			Token:         tokenStr,
			PreRegistroID: &p.ID,
			ExpiraEm:      now.Add(168 * time.Hour), // 7 dias
			Usado:         false,
			AlunoID:       &aluno.ID,
			AlunoNome:     aluno.Nome,
			AlunoEmail:    aluno.Email,
			CriadoEm:      now,
			CriadoPor:     authUsername,
			IpOrigem:      r.RemoteAddr,
		}

		if err := h.anamRepo.CreateToken(r.Context(), anamToken); err != nil {
			writeJSONError(w, "Failed to generate anamnese token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		anamAudit := &domain.AnamneseTokenAudit{
			Token:         tokenStr,
			AlunoID:       &aluno.ID,
			PreRegistroID: &p.ID,
			Evento:        "GERADO",
			Ip:            r.RemoteAddr,
			UserAgent:     r.UserAgent(),
			Detalhes:      "Token gerado automaticamente após aprovação de pré-cadastro. Expira em 7 dias.",
			DataEvento:    now,
		}
		_ = h.anamRepo.AddTokenAudit(r.Context(), anamAudit)

		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		hostLink = fmt.Sprintf("%s://%s/anamnese/submit/%s", scheme, r.Host, tokenStr)

		if autoSend {
			errSend := sendAnamneseEmail(r.Context(), h.configRepo, h.anamRepo, anamToken, r.Host, scheme)
			sentVal := errSend == nil
			emailEnviado = &sentVal
			if errSend != nil {
				emailErro = errSend.Error()
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp := map[string]any{
		"success":  true,
		"message":  "Cadastro de aluno criado com sucesso!",
		"aluno_id": aluno.ID,
	}
	if hostLink != "" {
		resp["anamnese_link"] = hostLink
	}
	if emailEnviado != nil {
		resp["email_enviado"] = *emailEnviado
		if !*emailEnviado && emailErro != "" {
			resp["email_erro"] = emailErro
		}
	}
	_ = json.NewEncoder(w).Encode(resp)
}

type rejectRequest struct {
	Motivo string `json:"motivo"`
}

func (h *PreCadastroHandler) Reject(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscan(idStr, &id); err != nil {
		writeJSONError(w, "Invalid ID parameter", http.StatusBadRequest)
		return
	}

	p, err := h.preRepo.FindByID(r.Context(), id)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		writeJSONError(w, "Pré-cadastro não encontrado", http.StatusNotFound)
		return
	}

	if p.Status != "aguardando_aprovacao" {
		writeJSONError(w, "Este pré-cadastro já possui status '"+p.Status+"'", http.StatusBadRequest)
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

	var authUserID int64
	if u, ok := r.Context().Value(userContextKey{}).(*domain.User); ok && u != nil {
		authUserID = u.ID
	}

	now := time.Now()
	p.Status = "rejeitado"
	p.AprovadoPor = &authUserID
	p.AprovadoEm = &now
	p.MotivoRejeicao = &req.Motivo

	if err := h.preRepo.Update(r.Context(), p); err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	audit := &domain.PreRegistroAudit{
		PreRegistroID: p.ID,
		Evento:        "REJEITADO",
		UsuarioID:     &authUserID,
		Detalhes:      "Rejeitado: " + req.Motivo,
		IpOrigem:      r.RemoteAddr,
		UserAgent:     r.UserAgent(),
		CriadoEm:      now,
	}
	_ = h.preRepo.AddAudit(r.Context(), audit)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Pré-cadastro rejeitado.",
	})
}
