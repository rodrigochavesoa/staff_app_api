package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Backup original env vars
	origPort, hadPort := os.LookupEnv("PORT")
	origDb, hadDb := os.LookupEnv("DB_PATH")
	origEnv, hadEnv := os.LookupEnv("ENV")
	origSecret, hadSecret := os.LookupEnv("SECRET_KEY")
	origAdminPassword, hadAdminPassword := os.LookupEnv("ADMIN_DEFAULT_PASSWORD")

	defer func() {
		if hadPort {
			os.Setenv("PORT", origPort)
		} else {
			os.Unsetenv("PORT")
		}
		if hadDb {
			os.Setenv("DB_PATH", origDb)
		} else {
			os.Unsetenv("DB_PATH")
		}
		if hadEnv {
			os.Setenv("ENV", origEnv)
		} else {
			os.Unsetenv("ENV")
		}
		if hadSecret {
			os.Setenv("SECRET_KEY", origSecret)
		} else {
			os.Unsetenv("SECRET_KEY")
		}
		if hadAdminPassword {
			os.Setenv("ADMIN_DEFAULT_PASSWORD", origAdminPassword)
		} else {
			os.Unsetenv("ADMIN_DEFAULT_PASSWORD")
		}
	}()

	// Clear environment variables that might be set
	os.Unsetenv("PORT")
	os.Unsetenv("DB_PATH")
	os.Unsetenv("ENV")
	os.Unsetenv("SECRET_KEY")
	os.Unsetenv("ADMIN_DEFAULT_PASSWORD")

	// Change working directory to a clean temp directory to ignore any root .env file
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	cfg := Load()

	if cfg.Port != "5000" {
		t.Errorf("Expected default Port 5000, got %s", cfg.Port)
	}

	if cfg.DatabasePath != "fichas_treino.db" {
		t.Errorf("Expected default DatabasePath 'fichas_treino.db', got %s", cfg.DatabasePath)
	}

	if cfg.Environment != "development" {
		t.Errorf("Expected default Environment 'development', got %s", cfg.Environment)
	}
}

func TestLoadCustom(t *testing.T) {
	// Backup original env vars
	origPort, hadPort := os.LookupEnv("PORT")
	origDb, hadDb := os.LookupEnv("DB_PATH")
	origEnv, hadEnv := os.LookupEnv("ENV")
	origSecret, hadSecret := os.LookupEnv("SECRET_KEY")
	origAdminPassword, hadAdminPassword := os.LookupEnv("ADMIN_DEFAULT_PASSWORD")

	defer func() {
		if hadPort {
			os.Setenv("PORT", origPort)
		} else {
			os.Unsetenv("PORT")
		}
		if hadDb {
			os.Setenv("DB_PATH", origDb)
		} else {
			os.Unsetenv("DB_PATH")
		}
		if hadEnv {
			os.Setenv("ENV", origEnv)
		} else {
			os.Unsetenv("ENV")
		}
		if hadSecret {
			os.Setenv("SECRET_KEY", origSecret)
		} else {
			os.Unsetenv("SECRET_KEY")
		}
		if hadAdminPassword {
			os.Setenv("ADMIN_DEFAULT_PASSWORD", origAdminPassword)
		} else {
			os.Unsetenv("ADMIN_DEFAULT_PASSWORD")
		}
	}()

	// Change working directory to a clean temp directory to ignore any root .env file
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Set custom environment variables
	os.Setenv("PORT", "8080")
	os.Setenv("DB_PATH", "test.db")
	os.Setenv("ENV", "production")
	os.Setenv("SECRET_KEY", "super-secret")
	os.Setenv("ADMIN_DEFAULT_PASSWORD", "super-admin-secret")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Expected Port 8080, got %s", cfg.Port)
	}

	if cfg.DatabasePath != "test.db" {
		t.Errorf("Expected DatabasePath 'test.db', got %s", cfg.DatabasePath)
	}

	if cfg.Environment != "production" {
		t.Errorf("Expected Environment 'production', got %s", cfg.Environment)
	}

	if cfg.SecretKey != "super-secret" {
		t.Errorf("Expected SecretKey 'super-secret', got %s", cfg.SecretKey)
	}
	if cfg.AdminDefaultPassword != "super-admin-secret" {
		t.Errorf("Expected AdminDefaultPassword 'super-admin-secret', got %s", cfg.AdminDefaultPassword)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		secretKey   string
		adminPass   string
		expectError bool
	}{
		{
			name:        "Dev env with default secret",
			environment: "development",
			secretKey:   "dev-secret-key-change-me-in-production",
			adminPass:   "admin-change-me-immediately",
			expectError: false,
		},
		{
			name:        "Prod env with secure secret",
			environment: "production",
			secretKey:   "some-secure-custom-key-123",
			adminPass:   "some-secure-admin-password",
			expectError: false,
		},
		{
			name:        "Prod env with default secret",
			environment: "production",
			secretKey:   "dev-secret-key-change-me-in-production",
			adminPass:   "some-secure-admin-password",
			expectError: true,
		},
		{
			name:        "Prod env with empty secret",
			environment: "production",
			secretKey:   "",
			adminPass:   "some-secure-admin-password",
			expectError: true,
		},
		{
			name:        "Prod env with default admin password",
			environment: "production",
			secretKey:   "some-secure-custom-key-123",
			adminPass:   "admin-change-me-immediately",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Environment:          tc.environment,
				SecretKey:            tc.secretKey,
				AdminDefaultPassword: tc.adminPass,
			}
			err := cfg.Validate()
			if tc.expectError && err == nil {
				t.Errorf("Expected validation error, but got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no validation error, but got: %v", err)
			}
		})
	}
}
