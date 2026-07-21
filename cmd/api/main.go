package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"staff_app/internal/config"
	"staff_app/internal/domain"
	"staff_app/internal/exercicios/csvsync"
	"staff_app/internal/http"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg := config.Load()

	// Em produção, o logger também grava em arquivo.
	writeToFile := cfg.Environment == "production"
	cleanupLogger := logger.Setup(cfg.Environment, writeToFile)
	defer cleanupLogger()

	if err := cfg.Validate(); err != nil {
		logger.Error("Configuration validation failed", err)
		os.Exit(1)
	}

	logger.Info("Starting STAFF API (Go Edition)...",
		"env", cfg.Environment,
		"db_path", cfg.DatabasePath,
	)

	db, err := sqlite.Connect(cfg.DatabasePath)
	if err != nil {
		logger.Error("Failed to initialize database", err)
		os.Exit(1)
	}
	deps := http.NewSQLiteDeps(db)

	// Garante um admin inicial quando o banco está vazio.
	if err := bootstrapAdmin(context.Background(), db, cfg); err != nil {
		logger.Error("Failed to bootstrap admin user", err)
		os.Exit(1)
	}

	// Catálogo CSV → SQLite; falha não impede o startup.
	syncExerciseCatalog(db)

	server := http.NewServer(cfg, deps)

	if err := server.Start(); err != nil {
		logger.Error("HTTP Server runtime error", err)
		os.Exit(1)
	}

	logger.Info("STAFF API shutdown complete.")
	fmt.Println("Goodbye!")
}

func syncExerciseCatalog(db *sqlite.DB) {
	csvPath := csvsync.DefaultCSVPath()
	records, err := csvsync.ParseFile(csvPath)
	if err != nil {
		logger.Error("Failed to parse exercise catalog CSV", err, "path", csvPath)
		return
	}
	if records == nil {
		logger.Info("Exercise catalog CSV not found; skipping sync", "path", csvPath)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := sqlite.NewExercicioRepository(db)
	res, err := csvsync.Sync(ctx, repo, records)
	if err != nil {
		logger.Error("Failed to sync exercise catalog", err, "path", csvPath)
		return
	}
	skipped := res.SkippedEmptyName + res.SkippedInvalidCode + res.SkippedReservedRange +
		res.SkippedDBOwnerConflict + res.SkippedNameConflict
	logger.Info("Exercise catalog synced",
		"path", csvPath,
		"inserted", res.Inserted,
		"updated", res.Updated,
		"skipped", skipped,
	)
}

func bootstrapAdmin(ctx context.Context, db *sqlite.DB, cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	users := sqlite.NewUserRepository(db)
	count, err := users.Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminDefaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash bootstrap admin password: %w", err)
	}

	admin := &domain.User{
		Username:     cfg.AdminDefaultUsername,
		Email:        cfg.AdminDefaultEmail,
		PasswordHash: string(hash),
		NomeCompleto: "Administrador",
		IsAdmin:      true,
		Ativo:        true,
		Aprovado:     true,
	}
	if err := users.Create(ctx, admin); err != nil {
		return err
	}
	logger.Warn("Bootstrap admin user created. Change ADMIN_DEFAULT_PASSWORD before production use.", "username", admin.Username)
	return nil
}
