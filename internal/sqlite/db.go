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

type DB struct {
	*sql.DB
}

func Connect(dbPath string) (*DB, error) {
	dbExisted := false
	if info, err := os.Stat(dbPath); err == nil && info.Size() > 0 {
		dbExisted = true
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite supports only 1 writer at a time
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	wrappedDB := &DB{db}

	if err := wrappedDB.runMigrations(ctx, dbPath, dbExisted); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	logger.Info("Database connected and schema initialized successfully", "path", dbPath)
	return wrappedDB, nil
}

func (db *DB) runMigrations(ctx context.Context, dbPath string, dbExisted bool) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	allMigrations, err := migrations.GetMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations list: %w", err)
	}

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

	var pending []migrations.Migration
	for _, m := range allMigrations {
		if !appliedVersions[m.Version] {
			pending = append(pending, m)
		}
	}

	// Sem migrações pendentes: retorna já (startup rápido, sem backup).
	if len(pending) == 0 {
		logger.Debug("Database schema is already up to date")
		return nil
	}

	// Backup antes de migrar, só se o arquivo já existia com conteúdo.
	if dbExisted {
		logger.Info("New database migrations found. Initiating backup before migrating...", "pending_count", len(pending))
		if err := backupDatabase(dbPath); err != nil {
			logger.Warn("Failed to create database backup (continuing with migration anyway)", "error", err)
		}
	} else {
		logger.Info("Creating a fresh database schema with migrations", "pending_count", len(pending))
	}

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
			// Adiciona nome_completo se a coluna ainda não existir.
			hasNomeCompleto, err := hasColumn(ctx, tx, "users", "nome_completo")
			if err == nil && !hasNomeCompleto {
				if _, err := tx.ExecContext(ctx, "ALTER TABLE users ADD COLUMN nome_completo TEXT;"); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to add column nome_completo to users: %w", err)
				}
			}
			// Adiciona aprovado se a coluna ainda não existir.
			hasAprovado, err := hasColumn(ctx, tx, "users", "aprovado")
			if err == nil && !hasAprovado {
				if _, err := tx.ExecContext(ctx, "ALTER TABLE users ADD COLUMN aprovado INTEGER DEFAULT 0;"); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to add column aprovado to users: %w", err)
				}
			}
		} else if m.Version == 6 {
			// Se pre_registro_id ainda for NOT NULL, reconstrói anamnese_tokens (nullable + colunas extras).
			isNotNull, err := isColumnNotNull(ctx, tx, "anamnese_tokens", "pre_registro_id")
			if err == nil && isNotNull {
				logger.Info("Reconstructing table 'anamnese_tokens' to make 'pre_registro_id' nullable.", "version", 6)

				if _, err := tx.ExecContext(ctx, "ALTER TABLE anamnese_tokens RENAME TO temp_anamnese_tokens;"); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to rename anamnese_tokens to temp_anamnese_tokens: %w", err)
				}

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

				copySQL := `
				INSERT INTO anamnese_tokens (id, token, pre_registro_id, expira_em, usado)
				SELECT id, token, pre_registro_id, expira_em, usado FROM temp_anamnese_tokens;`
				if _, err := tx.ExecContext(ctx, copySQL); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to copy records to reconstructed anamnese_tokens table: %w", err)
				}

				if _, err := tx.ExecContext(ctx, "DROP TABLE temp_anamnese_tokens;"); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("failed to drop temp_anamnese_tokens table: %w", err)
				}
			} else {
				// Sem reconstrução: só adiciona colunas extras em anamnese_tokens se faltarem.
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

			// Colunas extras em anamneses (idempotente).
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

			// Colunas extras em pre_registros (idempotente).
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
			// Schema antigo de exercicios_reabilitacao (sem categoria): reconstrói a tabela.
			hasCategoria, err := hasColumn(ctx, tx, "exercicios_reabilitacao", "categoria")
			if err == nil {
				var tableExists int
				_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='exercicios_reabilitacao'").Scan(&tableExists)

				if tableExists > 0 && !hasCategoria {
					logger.Info("Reconstructing table 'exercicios_reabilitacao' to match legacy monólito schema.", "version", 7)

					if _, err := tx.ExecContext(ctx, "ALTER TABLE exercicios_reabilitacao RENAME TO temp_exercicios_reabilitacao;"); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to rename exercicios_reabilitacao to temp_exercicios_reabilitacao: %w", err)
					}

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

					if _, err := tx.ExecContext(ctx, "DROP TABLE temp_exercicios_reabilitacao;"); err != nil {
						_ = tx.Rollback()
						return fmt.Errorf("failed to drop temp_exercicios_reabilitacao table: %w", err)
					}
				}
			}
		} else if m.Version == 11 {
			// Colunas extras em fichas_treino_web (idempotente).
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
			// Colunas SVED em fichas_treino_web (idempotente).
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
				_ = tx.Rollback()
				return fmt.Errorf("failed to execute migration %d (%s): %w", m.Version, m.Name, err)
			}
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", m.Version); err != nil {
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

func backupDatabase(dbPath string) error {
	info, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if info.Size() == 0 {
		return nil
	}

	dir := filepath.Dir(dbPath)
	ext := filepath.Ext(dbPath)
	name := info.Name()
	baseName := name[:len(name)-len(ext)]

	timestamp := time.Now().Format("20060102_150405")
	backupName := fmt.Sprintf("%s_backup_%s%s", baseName, timestamp, ext)
	backupPath := filepath.Join(dir, backupName)

	logger.Info("Creating database backup before running migrations...", "original", dbPath, "backup", backupPath)

	// #nosec G304 - dbPath é caminho confiável definido na configuração da aplicação
	src, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open source database file for backup: %w", err)
	}
	defer func() {
		_ = src.Close()
	}()

	// #nosec G304 - backupPath é montado internamente no diretório confiável do banco
	// OpenFile com 0600 restringe leitura/escrita do backup (G302).
	dst, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer func() {
		_ = dst.Close()
	}()

	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("failed to write backup data: %w", err)
	}

	logger.Info("Database backup created successfully", "path", backupPath)
	return nil
}

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
