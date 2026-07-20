package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"staff_app/internal/domain"
	"staff_app/internal/repositories"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	tokenTTL         = 8 * time.Hour
	defaultSecretKey = "dev-secret-key-change-me-in-production" // #nosec G101 - development fallback only; production config validation rejects this value.
)

type userContextKey struct{}

type AuthHandler struct {
	users     repositories.UserRepository
	secretKey string
}

func NewAuthHandler(users repositories.UserRepository, secretKey string) *AuthHandler {
	if secretKey == "" {
		secretKey = defaultSecretKey
	}
	return &AuthHandler{
		users:     users,
		secretKey: secretKey,
	}
}

type authClaims struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	NomeCompleto string `json:"nome_completo"`
}

type changePasswordRequest struct {
	PasswordAtual string `json:"password_atual"`
	PasswordNova  string `json:"password_nova"`
}

type userResponse struct {
	ID           int64   `json:"id"`
	Username     string  `json:"username"`
	Email        string  `json:"email"`
	NomeCompleto string  `json:"nome_completo,omitempty"`
	IsAdmin      bool    `json:"is_admin"`
	CriadoEm     string  `json:"criado_em,omitempty"`
	UltimoLogin  *string `json:"ultimo_login,omitempty"`
	Ativo        bool    `json:"ativo,omitempty"`
	Aprovado     bool    `json:"aprovado,omitempty"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeJSONError(w, "Usuário ou senha incorretos.", http.StatusUnauthorized)
		return
	}

	user, err := h.users.GetByLogin(r.Context(), req.Username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Usuário ou senha incorretos.", http.StatusUnauthorized)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if !user.Aprovado {
		writeJSONError(w, "Cadastro pendente de aprovação do administrador.", http.StatusUnauthorized)
		return
	}
	if !user.Ativo {
		writeJSONError(w, "Conta inativa. Entre em contato com o suporte.", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeJSONError(w, "Usuário ou senha incorretos.", http.StatusUnauthorized)
		return
	}

	now := time.Now().UTC()
	if err := h.users.UpdateLastLogin(r.Context(), user.ID, now); err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	user.UltimoLogin = &now

	token, err := h.signToken(user, now)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":              token,
		"token_type":         "Bearer",
		"expires_in_seconds": int(tokenTTL.Seconds()),
		"user":               toUserResponse(user),
	})
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	req.NomeCompleto = strings.TrimSpace(req.NomeCompleto)

	if req.Username == "" || req.Email == "" || req.NomeCompleto == "" || req.Password == "" {
		writeJSONError(w, "username, email, nome_completo and password are required", http.StatusBadRequest)
		return
	}
	if !strings.Contains(req.Email, "@") {
		writeJSONError(w, "valid email is required", http.StatusBadRequest)
		return
	}
	if len(req.Password) < 6 {
		writeJSONError(w, "A senha deve ter pelo menos 6 caracteres.", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	user := &domain.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		NomeCompleto: req.NomeCompleto,
		IsAdmin:      false,
		Ativo:        false,
		Aprovado:     false,
	}
	if err := h.users.Create(r.Context(), user); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeJSONError(w, "Username or email is already registered", http.StatusConflict)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Cadastro realizado com sucesso. Aguarde aprovação do administrador para efetuar login.",
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toUserResponse(user))
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	if req.PasswordAtual == "" || req.PasswordNova == "" {
		writeJSONError(w, "password_atual and password_nova are required", http.StatusBadRequest)
		return
	}
	if len(req.PasswordNova) < 8 {
		writeJSONError(w, "A nova senha deve ter pelo menos 8 caracteres.", http.StatusBadRequest)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.PasswordAtual)); err != nil {
		writeJSONError(w, "Senha atual incorreta.", http.StatusUnauthorized)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.PasswordNova), bcrypt.DefaultCost)
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := h.users.UpdatePassword(r.Context(), user.ID, string(hash)); err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Senha alterada com sucesso.",
	})
}

func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []*domain.User{}
	}
	resp := make([]userResponse, 0, len(users))
	for _, user := range users {
		resp = append(resp, toUserResponse(user))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"total":    len(resp),
		"usuarios": resp,
	})
}

func (h *AuthHandler) ApproveUser(w http.ResponseWriter, r *http.Request) {
	h.writeUserAction(w, r, h.users.Approve)
}

func (h *AuthHandler) RejectUser(w http.ResponseWriter, r *http.Request) {
	h.writeUserAction(w, r, h.users.RejectPending)
}

func (h *AuthHandler) ToggleUser(w http.ResponseWriter, r *http.Request) {
	h.writeUserAction(w, r, h.users.ToggleActive)
}

func (h *AuthHandler) writeUserAction(w http.ResponseWriter, r *http.Request, action func(context.Context, int64) error) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeJSONError(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	if err := action(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "User not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := bearerToken(r)
		if tokenString == "" {
			writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		claims := &authClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(h.secretKey), nil
		})
		if err != nil || !token.Valid {
			writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		id, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil || id <= 0 {
			writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		user, err := h.users.GetByID(r.Context(), id)
		if err != nil || !user.Ativo || !user.Aprovado {
			writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *AuthHandler) OptionalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := bearerToken(r)
		if tokenString == "" {
			next.ServeHTTP(w, r)
			return
		}

		claims := &authClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(h.secretKey), nil
		})
		if err != nil || !token.Valid {
			writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		id, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil || id <= 0 {
			writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		user, err := h.users.GetByID(r.Context(), id)
		if err != nil || !user.Ativo || !user.Aprovado {
			writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok {
			writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !user.IsAdmin {
			writeJSONError(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserFromContext(ctx context.Context) (*domain.User, bool) {
	user, ok := ctx.Value(userContextKey{}).(*domain.User)
	return user, ok && user != nil
}

func (h *AuthHandler) signToken(user *domain.User, now time.Time) (string, error) {
	claims := authClaims{
		Username: user.Username,
		Email:    user.Email,
		IsAdmin:  user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(user.ID, 10),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(h.secretKey))
	if err != nil {
		return "", err
	}
	return signed, nil
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func toUserResponse(user *domain.User) userResponse {
	resp := userResponse{
		ID:           user.ID,
		Username:     user.Username,
		Email:        user.Email,
		NomeCompleto: user.NomeCompleto,
		IsAdmin:      user.IsAdmin,
		Ativo:        user.Ativo,
		Aprovado:     user.Aprovado,
	}
	if !user.CriadoEm.IsZero() {
		resp.CriadoEm = user.CriadoEm.UTC().Format(time.RFC3339)
	}
	if user.UltimoLogin != nil {
		value := user.UltimoLogin.UTC().Format(time.RFC3339)
		resp.UltimoLogin = &value
	}
	return resp
}
