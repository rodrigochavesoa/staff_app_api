package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                 string
	DatabasePath         string
	Environment          string
	SecretKey            string
	FichaExpirationDays  int
	HashLength           int
	CorsOrigins          []string
	GarminUploadDir      string
	MaxUploadBytes       int64
	AdminDefaultUsername string
	AdminDefaultEmail    string
	AdminDefaultPassword string
	AITrainingMode       string
	AITrainingProviders  []string
	AITrainingTimeoutSec int
	GeminiAPIKey         string
	GeminiModel          string
	OpenAITrainingModel  string
	ClaudeAPIKey         string
	ClaudeModel          string
}

// Load loads the configuration from environment variables and an optional .env file
func Load() *Config {
	// Try loading .env file
	loadEnvFile(".env")

	port := getEnv("PORT", "5000")
	dbPath := getEnv("DB_PATH", "fichas_treino.db")
	env := getEnv("ENV", "development")
	if env == "" {
		env = getEnv("FLASK_ENV", "development") // Fallback
	}
	secretKey := getEnv("SECRET_KEY", "dev-secret-key-change-me-in-production")

	expirationDays, err := strconv.Atoi(getEnv("FICHA_EXPIRATION_DAYS", "30"))
	if err != nil {
		expirationDays = 30
	}

	hashLength, err := strconv.Atoi(getEnv("HASH_LENGTH", "12"))
	if err != nil {
		hashLength = 12
	}

	maxUploadMB, err := strconv.Atoi(getEnv("GARMIN_MAX_UPLOAD_MB", "50"))
	if err != nil || maxUploadMB <= 0 {
		maxUploadMB = 50
	}

	aiTimeoutSec, err := strconv.Atoi(getEnv("AI_TRAINING_PROVIDER_TIMEOUT_SECONDS", "20"))
	if err != nil || aiTimeoutSec <= 0 {
		aiTimeoutSec = 20
	}

	// Parse CORS origins
	corsOriginsStr := getEnv("CORS_ORIGINS", "https://rcstorestaff.com.br,https://www.rcstorestaff.com.br,http://localhost:*,http://127.0.0.1:*")
	corsOrigins := strings.Split(corsOriginsStr, ",")
	for i, origin := range corsOrigins {
		corsOrigins[i] = strings.TrimSpace(origin)
	}

	aiProvidersStr := getEnv("AI_TRAINING_PROVIDERS", "gemini,openai,claude,local")
	aiProviders := strings.Split(aiProvidersStr, ",")
	for i, provider := range aiProviders {
		aiProviders[i] = strings.TrimSpace(strings.ToLower(provider))
	}

	return &Config{
		Port:                 port,
		DatabasePath:         dbPath,
		Environment:          env,
		SecretKey:            secretKey,
		FichaExpirationDays:  expirationDays,
		HashLength:           hashLength,
		CorsOrigins:          corsOrigins,
		GarminUploadDir:      getEnv("GARMIN_UPLOAD_DIR", "uploads_atividades"),
		MaxUploadBytes:       int64(maxUploadMB) * 1024 * 1024,
		AdminDefaultUsername: getEnv("ADMIN_DEFAULT_USERNAME", "admin"),
		AdminDefaultEmail:    getEnv("ADMIN_DEFAULT_EMAIL", "admin@example.com"),
		AdminDefaultPassword: getEnv("ADMIN_DEFAULT_PASSWORD", "admin-change-me-immediately"),
		AITrainingMode:       getEnv("AI_TRAINING_MODE", "assistive"),
		AITrainingProviders:  aiProviders,
		AITrainingTimeoutSec: aiTimeoutSec,
		GeminiAPIKey:         getEnv("GEMINI_API_KEY", getEnv("GOOGLE_API_KEY", "")),
		GeminiModel:          getEnv("GEMINI_MODEL", "gemini-2.5-flash-lite"),
		OpenAITrainingModel:  getEnv("OPENAI_TRAINING_MODEL", getEnv("OPENAI_MODEL", "gpt-4o-mini")),
		ClaudeAPIKey:         getEnv("CLAUDE_API_KEY", getEnv("ANTHROPIC_API_KEY", "")),
		ClaudeModel:          getEnv("CLAUDE_TRAINING_MODEL", getEnv("CLAUDE_MODEL", "claude-3-5-haiku-latest")),
	}
}

// getEnv gets an env var or returns a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// loadEnvFile reads a simple .env file and sets environment variables
func loadEnvFile(filename string) {
	// #nosec G304 - filename is a trusted static config path (.env) loaded at startup
	file, err := os.Open(filename)
	if err != nil {
		return // Ignore error, file might not exist
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on the first '='
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Parse inline comments in value
		if strings.HasPrefix(value, "\"") {
			endIdx := strings.Index(value[1:], "\"")
			if endIdx >= 0 {
				value = value[1 : endIdx+1]
			} else {
				value = value[1:]
			}
		} else if strings.HasPrefix(value, "'") {
			endIdx := strings.Index(value[1:], "'")
			if endIdx >= 0 {
				value = value[1 : endIdx+1]
			} else {
				value = value[1:]
			}
		} else {
			// No quotes: just strip anything from the first '#'
			if hashIdx := strings.Index(value, "#"); hashIdx >= 0 {
				value = strings.TrimSpace(value[:hashIdx])
			}
		}

		// Only set if not already set by system environment
		if _, exists := os.LookupEnv(key); !exists {
			// Ignored Setenv error is safe to discard as it only populates defaults on local run (G104)
			_ = os.Setenv(key, value)
		}
	}
}

// Validate checks the configuration for consistency and security requirements
func (c *Config) Validate() error {
	if c.Environment == "production" {
		if c.SecretKey == "" || c.SecretKey == "dev-secret-key-change-me-in-production" {
			return fmt.Errorf("SECRET_KEY must be set to a secure, custom value in production environment")
		}
		if c.AdminDefaultPassword == "" || c.AdminDefaultPassword == "admin-change-me-immediately" {
			return fmt.Errorf("ADMIN_DEFAULT_PASSWORD must be set to a secure, custom value in production environment")
		}
	}
	return nil
}
