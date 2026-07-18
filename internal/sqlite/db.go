package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"staff_app/internal/migrations"
	"staff_app/internal/platform/logger"

	_ "modernc.org/sqlite" // SQLite CGO-free driver
)

// DB represents the database connection pool
type DB struct {
	*sql.DB
}

// Connect opens a connection to the SQLite database and executes migrations safely
func Connect(dbPath string) (*DB, error) {
	// Check if database file existed and was not empty BEFORE opening it
	dbExisted := false
	if info, err := os.Stat(dbPath); err == nil && info.Size() > 0 {
		dbExisted = true
	}

	// 1. Open the connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Set connection pool limits
	db.SetMaxOpenConns(1) // SQLite supports only 1 writer at a time
	db.SetMaxIdleConns(1)

	// Ping database to verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		// Discard Close error as we are returning the original connection error (G104)
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		// Discard Close error as we are returning the original connection error (G104)
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	wrappedDB := &DB{db}

	// 2. Run migrations
	if err := wrappedDB.runMigrations(ctx, dbPath, dbExisted); err != nil {
		// Discard Close error as we are returning the original connection error (G104)
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	logger.Info("Database connected and schema initialized successfully", "path", dbPath)
	return wrappedDB, nil
}

// runMigrations applies database migrations safely using version control
func (db *DB) runMigrations(ctx context.Context, dbPath string, dbExisted bool) error {
	// Create schema_migrations table if not exists
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Get list of all available migrations
	allMigrations, err := migrations.GetMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations list: %w", err)
	}

	// Fetch already applied versions from schema_migrations
	appliedVersions := make(map[int]bool)
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var v int
			if err := rows.Scan(&v); err == nil {
				appliedVersions[v] = true
			}
		}
	}

	// Identify pending migrations
	var pending []migrations.Migration
	for _, m := range allMigrations {
		if !appliedVersions[m.Version] {
			pending = append(pending, m)
		}
	}

	// If no pending migrations, return immediately (fast startup, no backup)
	if len(pending) == 0 {
		logger.Debug("Database schema is already up to date")
		return nil
	}

	// Trigger backup before applying migrations (only if database existed with content previously)
	if dbExisted {
		logger.Info("New database migrations found. Initiating backup before migrating...", "pending_count", len(pending))
		if err := backupDatabase(dbPath); err != nil {
			logger.Warn("Failed to create database backup (continuing with migration anyway)", "error", err)
		}
	} else {
		logger.Info("Creating a fresh database schema with migrations", "pending_count", len(pending))
	}

	// Apply migrations sequentially inside a transaction
	for _, m := range pending {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to start transaction for migration %d: %w", m.Version, err)
		}

		logger.Info("Applying database migration...", "version", m.Version, "name", m.Name)

		skipMigration := false
		if m.Version == 2 {
			hasAtivo, err := hasColumn(ctx, tx, "alunos", "ativo")
			if err == nil && hasAtivo {
				logger.Info("Column 'ativo' already exists in table 'alunos'. Skipping ALTER TABLE for migration 2.", "version", 2)
				skipMigration = true
			}
		} else if m.Version == 5 {
			// Idempotently add nome_completo column if missing
			hasNomeCompleto, err := hasColumn(ctx, tx, "users", "nome_completo")
			if err == nil && !hasNomeCompleto {
				if _, err := tx.ExecContext(ctx, "ALTER TABLE users ADD COLUMN nome_completo TEXT;"); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to add column nome_completo to users: %w", err)
				}
			}
			// Idempotently add aprovado column if missing
			hasAprovado, err := hasColumn(ctx, tx, "users", "aprovado")
			if err == nil && !hasAprovado {
				if _, err := tx.ExecContext(ctx, "ALTER TABLE users ADD COLUMN aprovado INTEGER DEFAULT 0;"); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to add column aprovado to users: %w", err)
				}
			}
		} else if m.Version == 6 {
			// 1. Check if pre_registro_id in anamnese_tokens is NOT NULL
			isNotNull, err := isColumnNotNull(ctx, tx, "anamnese_tokens", "pre_registro_id")
			if err == nil && isNotNull {
				logger.Info("Reconstructing table 'anamnese_tokens' to make 'pre_registro_id' nullable.", "version", 6)

				// Rename old table
				if _, err := tx.ExecContext(ctx, "ALTER TABLE anamnese_tokens RENAME TO temp_anamnese_tokens;"); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to rename anamnese_tokens to temp_anamnese_tokens: %w", err)
				}

				// Create new table with updated nullable fields and extra columns
				newTableSQL := `
				CREATE TABLE IF NOT EXISTS anamnese_tokens (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					token TEXT UNIQUE NOT NULL,
					pre_registro_id INTEGER NULL,
					expira_em TIMESTAMP NOT NULL,
					usado INTEGER DEFAULT 0,
					aluno_id INTEGER NULL,
					aluno_nome TEXT,
					aluno_email TEXT,
					criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					criado_por TEXT,
					ip_origem TEXT,
					usado_em TIMESTAMP NULL,
					ip_submissao TEXT,
					anamnese_id INTEGER NULL,
					FOREIGN KEY (aluno_id) REFERENCES alunos(id) ON DELETE SET NULL,
					FOREIGN KEY (pre_registro_id) REFERENCES pre_registros(id) ON DELETE SET NULL
				);`
				if _, err := tx.ExecContext(ctx, newTableSQL); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to create new anamnese_tokens table: %w", err)
				}

				// Copy old records if any
				copySQL := `
				INSERT INTO anamnese_tokens (id, token, pre_registro_id, expira_em, usado)
				SELECT id, token, pre_registro_id, expira_em, usado FROM temp_anamnese_tokens;`
				if _, err := tx.ExecContext(ctx, copySQL); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to copy records to reconstructed anamnese_tokens table: %w", err)
				}

				// Drop old table
				if _, err := tx.ExecContext(ctx, "DROP TABLE temp_anamnese_tokens;"); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to drop temp_anamnese_tokens table: %w", err)
				}
			} else {
				// If not reconstructed, add extra columns to anamnese_tokens idempotently
				extraCols := map[string]string{
					"aluno_id":     "INTEGER NULL REFERENCES alunos(id) ON DELETE SET NULL",
					"aluno_nome":   "TEXT",
					"aluno_email":  "TEXT",
					"criado_em":    "TIMESTAMP DEFAULT CURRENT_TIMESTAMP",
					"criado_por":   "TEXT",
					"ip_origem":    "TEXT",
					"usado_em":     "TIMESTAMP NULL",
					"ip_submissao": "TEXT",
					"anamnese_id":  "INTEGER NULL",
				}
				for colName, colDef := range extraCols {
					hasCol, err := hasColumn(ctx, tx, "anamnese_tokens", colName)
					if err == nil && !hasCol {
						if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE anamnese_tokens ADD COLUMN %s %s;", colName, colDef)); err != nil {
							_ = tx.Rollback()
							return fmt.Errorf("failed to add column %s to anamnese_tokens: %w", colName, err)
						}
					}
				}
			}

			// 2. Idempotently add extra columns to anamneses
			anamnesesCols := map[string]string{
				"status_aprovacao": "TEXT DEFAULT 'pendente'",
				"aprovado_por":     "TEXT",
				"aprovado_em":      "TIMESTAMP",
				"motivo_rejeicao":  "TEXT",
				"token_origem":     "TEXT",
			}
			for colName, colDef := range anamnesesCols {
				hasCol, err := hasColumn(ctx, tx, "anamneses", colName)
				if err == nil && !hasCol {
					if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE anamneses ADD COLUMN %s %s;", colName, colDef)); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to add column %s to anamneses: %w", colName, err)
					}
				}
			}

			// 3. Idempotently add extra columns to pre_registros
			preRegistrosCols := map[string]string{
				"status":          "TEXT DEFAULT 'aguardando_aprovacao'",
				"aprovado_por":    "INTEGER NULL REFERENCES users(id)",
				"aprovado_em":     "TIMESTAMP NULL",
				"aluno_id_criado": "INTEGER NULL REFERENCES alunos(id)",
				"motivo_rejeicao": "TEXT",
			}
			for colName, colDef := range preRegistrosCols {
				hasCol, err := hasColumn(ctx, tx, "pre_registros", colName)
				if err == nil && !hasCol {
					if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE pre_registros ADD COLUMN %s %s;", colName, colDef)); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to add column %s to pre_registros: %w", colName, err)
					}
				}
			}
		} else if m.Version == 7 {
			// Check if exercicios_reabilitacao exists and has old schema (lacks 'categoria')
			hasCategoria, err := hasColumn(ctx, tx, "exercicios_reabilitacao", "categoria")
			if err == nil {
				var tableExists int
				_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='exercicios_reabilitacao'").Scan(&tableExists)

				if tableExists > 0 && !hasCategoria {
					logger.Info("Reconstructing table 'exercicios_reabilitacao' to match legacy monólito schema.", "version", 7)

					// Rename old table
					if _, err := tx.ExecContext(ctx, "ALTER TABLE exercicios_reabilitacao RENAME TO temp_exercicios_reabilitacao;"); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to rename exercicios_reabilitacao to temp_exercicios_reabilitacao: %w", err)
					}

					// Create new table
					newTableSQL := `
					CREATE TABLE IF NOT EXISTS exercicios_reabilitacao (
						codigo INTEGER PRIMARY KEY AUTOINCREMENT,
						nome TEXT UNIQUE NOT NULL,
						categoria TEXT DEFAULT 'normal',
						descricao_terapeutica TEXT,
						descricao TEXT,
						indicacoes TEXT,
						contraindicacoes TEXT,
						restricoes_sugeridas TEXT,
						grupo_muscular TEXT,
						musculo_foco TEXT,
						tipo_exercicio TEXT,
						intensidade TEXT,
						nivel_prioridade INTEGER DEFAULT 2,
						fonte_cientifica TEXT,
						url TEXT,
						url_secundaria TEXT,
						video_url TEXT,
						criado_por TEXT,
						criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
						status TEXT DEFAULT 'ativo',
						notas_profissional TEXT,
						atualizado_em TIMESTAMP,
						atualizado_por TEXT
					);`
					if _, err := tx.ExecContext(ctx, newTableSQL); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to create new exercicios_reabilitacao table: %w", err)
					}

					// Copy data
					copySQL := `
					INSERT INTO exercicios_reabilitacao (
						codigo, nome, descricao_terapeutica, descricao, grupo_muscular,
						contraindicacoes, restricoes_sugeridas, url_secundaria, video_url
					)
					SELECT 
						id, nome, descricao, descricao, grupo_muscular,
						restricoes_sugeridas, restricoes_sugeridas, video_url, video_url
					FROM temp_exercicios_reabilitacao;`
					if _, err := tx.ExecContext(ctx, copySQL); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to copy records to reconstructed exercicios_reabilitacao: %w", err)
					}

					// Drop temp table
					if _, err := tx.ExecContext(ctx, "DROP TABLE temp_exercicios_reabilitacao;"); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to drop temp_exercicios_reabilitacao table: %w", err)
					}
				}
			}
		} else if m.Version == 11 {
			// Idempotently add extra columns to fichas_treino_web
			extraColsFichas := map[string]string{
				"tipo_ficha":        "TEXT DEFAULT 'manual'",
				"num_treinos":       "INTEGER DEFAULT 1",
				"versao":            "INTEGER DEFAULT 1",
				"ficha_anterior_id": "INTEGER",
				"data_arquivamento": "TEXT",
			}
			for colName, colDef := range extraColsFichas {
				hasCol, err := hasColumn(ctx, tx, "fichas_treino_web", colName)
				if err == nil && !hasCol {
					if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE fichas_treino_web ADD COLUMN %s %s;", colName, colDef)); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to add column %s to fichas_treino_web: %w", colName, err)
					}
				}
			}
			skipMigration = true
		} else if m.Version == 12 {
			// Idempotently add extra SVED columns to fichas_treino_web
			extraColsSved := map[string]string{
				"ies_score":    "REAL DEFAULT 0.0",
				"volume_sved":  "INTEGER DEFAULT 0",
				"densidade":    "REAL DEFAULT 0.0",
				"tut_total":    "INTEGER DEFAULT 0",
				"series":       "TEXT",
				"rir":          "INTEGER DEFAULT 2",
				"cadencia":     "TEXT DEFAULT '4010'",
				"rest_seconds": "INTEGER DEFAULT 60",
			}
			for colName, colDef := range extraColsSved {
				hasCol, err := hasColumn(ctx, tx, "fichas_treino_web", colName)
				if err == nil && !hasCol {
					if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE fichas_treino_web ADD COLUMN %s %s;", colName, colDef)); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to add column %s to fichas_treino_web: %w", colName, err)
					}
				}
			}
			skipMigration = true
		}

		if !skipMigration {
			if _, err := tx.ExecContext(ctx, m.UpSQL); err != nil {
				// Rollback unhandled error is safe to ignore here as we are returning the original error (G104)
				_ = tx.Rollback()
				return fmt.Errorf("failed to execute migration %d (%s): %w", m.Version, m.Name, err)
			}
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", m.Version); err != nil {
			// Rollback unhandled error is safe to ignore here as we are returning the original error (G104)
			_ = tx.Rollback()
			return fmt.Errorf("failed to record migration status for version %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction for migration %d: %w", m.Version, err)
		}
		logger.Info("Migration applied successfully", "version", m.Version, "name", m.Name)
	}

	return nil
}

// hasColumn checks if a column exists in a given table using PRAGMA table_info
func hasColumn(ctx context.Context, tx *sql.Tx, tableName, columnName string) (bool, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typeStr string
		var notnull, pk int
		var dfltVal any
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltVal, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, columnName) {
			return true, nil
		}
	}
	return false, nil
}

// isColumnNotNull checks if a column is NOT NULL in a given table using PRAGMA table_info
func isColumnNotNull(ctx context.Context, tx *sql.Tx, tableName, columnName string) (bool, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typeStr string
		var notnull, pk int
		var dfltVal any
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltVal, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, columnName) {
			return notnull == 1, nil
		}
	}
	return false, nil
}

// backupDatabase copies the database file if it exists and has content
func backupDatabase(dbPath string) error {
	info, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		// No database file exists yet, no backup needed
		return nil
	}
	if err != nil {
		return err
	}

	// If the database is empty, no backup needed
	if info.Size() == 0 {
		return nil
	}

	// Generate backup filename
	dir := filepath.Dir(dbPath)
	ext := filepath.Ext(dbPath)
	name := info.Name()
	baseName := name[:len(name)-len(ext)]

	timestamp := time.Now().Format("20060102_150405")
	backupName := fmt.Sprintf("%s_backup_%s%s", baseName, timestamp, ext)
	backupPath := filepath.Join(dir, backupName)

	logger.Info("Creating database backup before running migrations...", "original", dbPath, "backup", backupPath)

	// Copy the file
	// #nosec G304 - dbPath is a trusted path defined in application configuration
	src, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open source database file for backup: %w", err)
	}
	defer func() {
		// Discard Close error on read-only file (G104)
		_ = src.Close()
	}()

	// #nosec G304 - backupPath is constructed internally in trusted db config directory
	// Use OpenFile with 0600 permissions to restrict read/write access (G302)
	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer func() {
		// Discard Close error on write-only backup file (G104)
		_ = dst.Close()
	}()

	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("failed to write backup data: %w", err)
	}

	logger.Info("Database backup created successfully", "path", backupPath)
	return nil
}

// parseDateTime parses date/time strings in multiple formats (RFC3339, 2006-01-02 15:04:05, etc.)
func parseDateTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty time string")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unsupported date/time format: %q", s)
}
