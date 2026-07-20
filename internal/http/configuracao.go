package http

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/platform/logger"
	"staff_app/internal/repositories"
)

// AdminConfigHandler handles HTTP requests for system configurations and administrative dashboard stats.
type AdminConfigHandler struct {
	configRepo    repositories.ConfiguracaoRepository
	dashboardRepo repositories.DashboardRepository
}

// NewAdminConfigHandler creates a new AdminConfigHandler instance.
func NewAdminConfigHandler(configRepo repositories.ConfiguracaoRepository, dashboardRepo repositories.DashboardRepository) *AdminConfigHandler {
	return &AdminConfigHandler{
		configRepo:    configRepo,
		dashboardRepo: dashboardRepo,
	}
}

// List handles GET /api/v1/admin/configuracoes
func (h *AdminConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	configs, err := h.configRepo.List(r.Context())
	if err != nil {
		logger.Error("Failed to list configurations", err)
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Mask sensitive configuration values (such as SMTP_PASSWORD)
	for _, c := range configs {
		if c.Sensivel {
			if c.Valor != "" {
				c.Valor = "********"
				c.ValorMascarado = true
			} else {
				c.ValorMascarado = false
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"configuracoes": configs,
	})
}

// updateConfigRequest defines the PUT request body for updating configurations
type updateConfigRequest struct {
	Configuracoes map[string]string `json:"configuracoes"`
}

// Update handles PUT /api/v1/admin/configuracoes
func (h *AdminConfigHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Fetch current configuration keys to check for existence and validate types
	dbConfigs, err := h.configRepo.List(r.Context())
	if err != nil {
		logger.Error("Failed to list database configurations during update", err)
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	dbConfigMap := make(map[string]*domain.Configuracao)
	for _, c := range dbConfigs {
		dbConfigMap[c.Chave] = c
	}

	user, userOk := UserFromContext(r.Context())
	var userID *int64
	if userOk {
		userID = &user.ID
	}

	var updates []*domain.Configuracao

	for key, val := range req.Configuracoes {
		dbCfg, ok := dbConfigMap[key]
		if !ok {
			writeJSONError(w, fmt.Sprintf("Chave de configuração não permitida: %s", key), http.StatusBadRequest)
			return
		}

		// Handle sensitive values
		if dbCfg.Sensivel {
			if val == "********" {
				// Mask sent back means no change, skip this key
				continue
			}
			if val == "" {
				// Empty string means clear/wipe the password
				dbCfg.Valor = ""
			} else {
				// Otherwise substitute the new password
				dbCfg.Valor = val
			}
		} else {
			dbCfg.Valor = strings.TrimSpace(val)
		}

		// Type validations
		switch dbCfg.Tipo {
		case "boolean":
			lowerVal := strings.ToLower(dbCfg.Valor)
			if lowerVal != "true" && lowerVal != "false" {
				writeJSONError(w, fmt.Sprintf("Valor inválido para chave %s. Esperado booleano (true/false).", key), http.StatusBadRequest)
				return
			}
			dbCfg.Valor = lowerVal // normalize to lowercase
		case "int":
			portVal, err := strconv.Atoi(dbCfg.Valor)
			if err != nil {
				writeJSONError(w, fmt.Sprintf("Valor inválido para chave %s. Esperado número inteiro.", key), http.StatusBadRequest)
				return
			}
			// Specific validation for port range
			if key == "SMTP_PORT" {
				if portVal < 1 || portVal > 65535 {
					writeJSONError(w, "SMTP_PORT deve ficar entre 1 e 65535.", http.StatusBadRequest)
					return
				}
			}
		case "float":
			if _, err := strconv.ParseFloat(dbCfg.Valor, 64); err != nil {
				writeJSONError(w, fmt.Sprintf("Valor inválido para chave %s. Esperado número decimal.", key), http.StatusBadRequest)
				return
			}
		case "json":
			if !json.Valid([]byte(dbCfg.Valor)) {
				writeJSONError(w, fmt.Sprintf("Valor inválido para chave %s. Esperado JSON válido.", key), http.StatusBadRequest)
				return
			}
		}

		// Specific validation for email formats
		if strings.HasSuffix(key, "_EMAIL") && dbCfg.Valor != "" {
			if !isBasicEmail(dbCfg.Valor) {
				writeJSONError(w, fmt.Sprintf("Valor inválido para chave %s. Esperado formato de e-mail válido.", key), http.StatusBadRequest)
				return
			}
		}

		dbCfg.AtualizadoPor = userID
		updates = append(updates, dbCfg)
	}

	if len(updates) > 0 {
		if err := h.configRepo.UpdateMultiple(r.Context(), updates); err != nil {
			logger.Error("Failed to update system configurations", err)
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Configurações atualizadas com sucesso.",
	})
}

// testSMTPRequest defines the POST request body for testing SMTP connection
type testSMTPRequest struct {
	ToEmail   string  `json:"to_email"`
	Host      *string `json:"host,omitempty"`
	Port      *string `json:"port,omitempty"`
	User      *string `json:"user,omitempty"`
	Password  *string `json:"password,omitempty"`
	FromEmail *string `json:"from_email,omitempty"`
	FromName  *string `json:"from_name,omitempty"`
}

// TestSMTP handles POST /api/v1/admin/configuracoes/testar-smtp
func (h *AdminConfigHandler) TestSMTP(w http.ResponseWriter, r *http.Request) {
	var req testSMTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	req.ToEmail = strings.TrimSpace(req.ToEmail)
	if req.ToEmail == "" || !isBasicEmail(req.ToEmail) {
		writeJSONError(w, "Destinatário de e-mail válido (to_email) é obrigatório.", http.StatusBadRequest)
		return
	}

	// Load existing configurations from DB to use as defaults
	dbConfigs, err := h.configRepo.List(r.Context())
	if err != nil {
		logger.Error("Failed to fetch configurations for SMTP test", err)
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	configMap := make(map[string]string)
	for _, c := range dbConfigs {
		configMap[c.Chave] = c.Valor
	}

	// Resolve actual SMTP parameters, checking overrides from the payload
	host := configMap["SMTP_HOST"]
	if req.Host != nil {
		host = strings.TrimSpace(*req.Host)
	}

	portStr := configMap["SMTP_PORT"]
	if req.Port != nil {
		portStr = strings.TrimSpace(*req.Port)
	}

	user := configMap["SMTP_USER"]
	if req.User != nil {
		user = strings.TrimSpace(*req.User)
	}

	password := configMap["SMTP_PASSWORD"]
	if req.Password != nil {
		overriddenPassword := *req.Password
		if overriddenPassword != "********" {
			password = overriddenPassword
		}
	}

	fromEmail := configMap["SMTP_FROM_EMAIL"]
	if req.FromEmail != nil {
		fromEmail = strings.TrimSpace(*req.FromEmail)
	}

	fromName := configMap["SMTP_FROM_NAME"]
	if req.FromName != nil {
		fromName = strings.TrimSpace(*req.FromName)
	}

	// Basic validation of inputs
	if host == "" {
		writeJSONError(w, "Host do servidor SMTP não configurado.", http.StatusBadRequest)
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		writeJSONError(w, "Porta SMTP inválida.", http.StatusBadRequest)
		return
	}
	if fromEmail == "" || !isBasicEmail(fromEmail) {
		writeJSONError(w, "E-mail de remetente (from_email) inválido.", http.StatusBadRequest)
		return
	}
	if hasHeaderInjection(fromName) {
		writeJSONError(w, "Nome de remetente inválido.", http.StatusBadRequest)
		return
	}

	// Log SMTP test request safely, without passwords
	logger.Info("Starting SMTP connection test",
		"to_email", req.ToEmail,
		"host", host,
		"port", port,
		"user", user,
		"from_email", fromEmail,
	)

	// Execute actual SMTP send
	subject := "Teste de Conexão SMTP - Sistema RC Staff"
	body := fmt.Sprintf("Olá,\n\nEste é um e-mail de teste enviado pelo Sistema RC Staff para validar as configurações de SMTP.\n\nSe você recebeu este e-mail, as configurações estão corretas!\n\nEnviado em: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	if err := sendEmailRaw(host, port, user, password, fromEmail, fromName, req.ToEmail, subject, body); err != nil {
		logger.Error("SMTP test failed internally", err)
		writeJSONError(w, "Falha ao enviar e-mail de teste. Verifique host, porta, autenticação e remetente.", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "E-mail de teste enviado com sucesso!",
	})
}

// DashboardStats handles GET /api/v1/admin/dashboard/stats
func (h *AdminConfigHandler) DashboardStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dashboardRepo.GetStats(r.Context())
	if err != nil {
		logger.Error("Failed to retrieve dashboard stats", err)
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(stats)
}

// sendEmailRaw connects to SMTP server and sends a simple text email with implicit SMTPS or STARTTLS
func sendEmailRaw(host string, port int, user, password, fromEmail, fromName, toEmail string, subject, body string) error {
	if !isBasicEmail(fromEmail) || !isBasicEmail(toEmail) || hasHeaderInjection(fromName) {
		return fmt.Errorf("invalid SMTP envelope or header value")
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	var conn net.Conn
	var err error

	// Connect using SMTPS (Implicit TLS) for port 465
	if port == 465 {
		tlsConfig := &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to build SMTP client: %w", err)
	}
	defer client.Close()

	// Use STARTTLS for standard ports if not already connected via SMTPS
	if port != 465 && (port == 587 || port == 25 || port == 2525) {
		tlsConfig := &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("STARTTLS handshake failed: %w", err)
		}
	}

	// Authenticate if a user is provided
	if user != "" || password != "" {
		auth := smtp.PlainAuth("", user, password, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	if err := client.Mail(fromEmail); err != nil {
		return fmt.Errorf("MAIL command rejected: %w", err)
	}
	if err := client.Rcpt(toEmail); err != nil {
		return fmt.Errorf("RCPT command rejected: %w", err)
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA command failed: %w", err)
	}
	defer wc.Close()

	fromHeader := fromEmail
	if fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", fromName, fromEmail)
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"\r\n"+
		"%s", fromHeader, toEmail, subject, body)

	if _, err = wc.Write([]byte(msg)); err != nil {
		return fmt.Errorf("failed to write email body: %w", err)
	}

	return nil
}

func isBasicEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || hasHeaderInjection(value) {
		return false
	}
	at := strings.Index(value, "@")
	return at > 0 && at < len(value)-1
}

func hasHeaderInjection(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}
