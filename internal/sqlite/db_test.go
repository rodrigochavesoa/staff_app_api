package sqlite

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"staff_app/internal/platform/logger"
)

func TestConnectAndMigrations(t *testing.T) {
	// Initialize logger in debug mode for test context
	logger.Setup("development", false)

	// Create a temp database path
	tempDir, err := os.MkdirTemp("", "sqlite-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test_fichas_treino.db")

	// 1. Establish connection and run migrations
	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("failed to connect and run migrations: %v", err)
	}
	defer db.Close()

	// 2. Verify that tables are created by querying sqlite_master
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatalf("failed to query tables list: %v", err)
	}
	defer rows.Close()

	tables := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("failed to scan table name: %v", err)
		}
		tables[name] = true
	}

	requiredTables := []string{"users", "planos", "alunos", "fichas_treino_web", "fichas_web", "feedback_fichas", "feedback_notificacoes"}
	for _, table := range requiredTables {
		if !tables[table] {
			t.Errorf("Expected table '%s' to be created, but it was not found", table)
		}
	}
}

func TestBackupDatabaseSafety(t *testing.T) {
	logger.Setup("development", false)

	// Create temp dir
	tempDir, err := os.MkdirTemp("", "sqlite-backup-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "fichas_treino.db")

	// Create a mock original database file with some content
	err = os.WriteFile(dbPath, []byte("mock-sqlite-database-data-content-here"), 0644)
	if err != nil {
		t.Fatalf("failed to write mock original database: %v", err)
	}

	// 1. Run backupDatabase helper
	err = backupDatabase(dbPath)
	if err != nil {
		t.Fatalf("backupDatabase failed: %v", err)
	}

	// 2. Find backup files created in the temp directory
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	var foundBackup bool
	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, "fichas_treino_backup_") && strings.HasSuffix(name, ".db") {
			foundBackup = true

			// Verify content is preserved
			backupPath := filepath.Join(tempDir, name)
			content, err := os.ReadFile(backupPath)
			if err != nil {
				t.Fatalf("failed to read backup file: %v", err)
			}
			if string(content) != "mock-sqlite-database-data-content-here" {
				t.Errorf("Backup file content is corrupted. Expected original content, got: %s", string(content))
			}
		}
	}

	if !foundBackup {
		t.Error("No backup file was created when database was not empty")
	}
}

func TestBackupDatabaseEmptySafety(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-empty-backup-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "empty_fichas_treino.db")

	// Create an empty file
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}
	f.Close()

	// 1. Run backupDatabase helper on empty database file
	err = backupDatabase(dbPath)
	if err != nil {
		t.Fatalf("backupDatabase on empty file failed: %v", err)
	}

	// 2. Verify no backup file is created for an empty DB file
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	for _, file := range files {
		if file.Name() != "empty_fichas_treino.db" {
			t.Errorf("Unexpected file created: %s. Empty databases should not trigger backups.", file.Name())
		}
	}
}

func TestNoBackupIfUpToDate(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-no-backup-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "fichas_treino.db")

	// 1. First connection (creates schema)
	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("first connection failed: %v", err)
	}

	// Insert some mock data so the file size > 0
	_, err = db.Exec("INSERT INTO users (username, email, password_hash) VALUES ('testuser', 'test@test.com', 'hash')")
	if err != nil {
		db.Close()
		t.Fatalf("failed to insert mock data: %v", err)
	}
	db.Close()

	// 2. Second connection (schema should already be up to date)
	db2, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("second connection failed: %v", err)
	}
	db2.Close()

	// 3. Verify no backup file was created in the directory
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, "fichas_treino_backup_") {
			t.Errorf("Unexpected backup file found: %s. No backups should be created when schema is already up to date.", name)
		}
	}
}

func TestIdempotentMigrationAlunosAtivo(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-idempotent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "legacy_fichas_treino.db")

	// 1. Manually open connection and create a legacy Alunos table already containing column `ativo`
	// but without the `schema_migrations` table.
	dbRaw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open raw sqlite db: %v", err)
	}

	// Create legacy table representation with 'ativo' already present
	_, err = dbRaw.Exec(`
		CREATE TABLE alunos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			nome TEXT NOT NULL,
			idade INTEGER NOT NULL,
			sexo TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL,
			ativo INTEGER DEFAULT 1
		)
	`)
	if err != nil {
		dbRaw.Close()
		t.Fatalf("failed to setup legacy table: %v", err)
	}
	dbRaw.Close()

	// 2. Connect through our database manager Connect (which runs migrations 1 and 2)
	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("Connect failed on legacy database with pre-existing 'ativo' column: %v", err)
	}
	defer db.Close()

	// 3. Verify that both versions 1 and 2 are recorded in schema_migrations
	ctx := t.Context()
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version IN (1, 2)").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query schema_migrations: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 migrations applied, got %d", count)
	}
}

// CopyFile helper for setting up manual backups in tests
func CopyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func TestIdempotentMigrationExercicioReabilitacao(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-exercicios-idempotent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "legacy_exercicios.db")

	// 1. Manually open connection and create the old simple table representation
	dbRaw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open raw sqlite db: %v", err)
	}

	_, err = dbRaw.Exec(`
		CREATE TABLE exercicios_reabilitacao (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			nome TEXT UNIQUE NOT NULL,
			descricao TEXT,
			grupo_muscular TEXT,
			restricoes_sugeridas TEXT,
			video_url TEXT
		)
	`)
	if err != nil {
		dbRaw.Close()
		t.Fatalf("failed to setup old table: %v", err)
	}

	// Insert a mock exercise record
	_, err = dbRaw.Exec(`
		INSERT INTO exercicios_reabilitacao (id, nome, descricao, grupo_muscular, restricoes_sugeridas, video_url)
		VALUES (123, 'Agachamento Terapêutico', 'Descrição do agachamento', 'Pernas', 'Evitar dor no joelho', 'https://youtube.com/watch?v=123')
	`)
	if err != nil {
		dbRaw.Close()
		t.Fatalf("failed to insert legacy data: %v", err)
	}
	dbRaw.Close()

	// 2. Connect through our database manager Connect (which runs migrations up to 7)
	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("Connect failed on database with pre-existing old table: %v", err)
	}
	defer db.Close()

	// 3. Verify migration status
	ctx := t.Context()
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = 7").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("expected migration 7 applied, got %d", count)
	}

	// 4. Verify data migration
	var codigo int
	var nome, descTerap, desc, grupo, contra, restricoes, videoUrl, urlSecundaria string
	err = db.QueryRowContext(ctx, `
		SELECT codigo, nome, descricao_terapeutica, descricao, grupo_muscular, contraindicacoes, restricoes_sugeridas, video_url, url_secundaria
		FROM exercicios_reabilitacao
		WHERE codigo = 123
	`).Scan(&codigo, &nome, &descTerap, &desc, &grupo, &contra, &restricoes, &videoUrl, &urlSecundaria)
	if err != nil {
		t.Fatalf("failed to query migrated exercise: %v", err)
	}

	if codigo != 123 ||
		nome != "Agachamento Terapêutico" ||
		descTerap != "Descrição do agachamento" ||
		desc != "Descrição do agachamento" ||
		grupo != "Pernas" ||
		contra != "Evitar dor no joelho" ||
		restricoes != "Evitar dor no joelho" ||
		videoUrl != "https://youtube.com/watch?v=123" ||
		urlSecundaria != "https://youtube.com/watch?v=123" {
		t.Errorf("Migrated fields mismatch. Got values: "+
			"codigo=%d, nome=%s, descTerap=%s, desc=%s, grupo=%s, contra=%s, restricoes=%s, videoUrl=%s, urlSecundaria=%s",
			codigo, nome, descTerap, desc, grupo, contra, restricoes, videoUrl, urlSecundaria)
	}

	// 5. Verify sugestoes_exercicios_rehab exists
	var tableExists int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sugestoes_exercicios_rehab'").Scan(&tableExists)
	if err != nil {
		t.Fatalf("failed to check sugestoes_exercicios_rehab table existence: %v", err)
	}
	if tableExists != 1 {
		t.Error("expected table sugestoes_exercicios_rehab to be created")
	}
}

func TestMigrationsIdempotent_LegacyFichasTreinoWeb(t *testing.T) {
	logger.Setup("development", false)

	tempDir, err := os.MkdirTemp("", "sqlite-idempotent-fichas-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "legacy_fichas.db")

	dbRaw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open raw sqlite db: %v", err)
	}

	_, err = dbRaw.Exec(`
		CREATE TABLE fichas_treino_web (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			aluno TEXT,
			idade INTEGER,
			sexo TEXT,
			objetivo TEXT,
			modalidade TEXT,
			nivel TEXT,
			frequencia_semanal INTEGER,
			duracao_treino INTEGER,
			restricoes TEXT,
			feedback TEXT,
			turma TEXT,
			lista_exercicios TEXT DEFAULT 'exercicios_com_grupos',
			data_criacao TEXT DEFAULT CURRENT_TIMESTAMP,
			ficha_json TEXT,
			tipo_ficha TEXT DEFAULT 'manual',
			num_treinos INTEGER DEFAULT 1,
			versao INTEGER DEFAULT 1,
			ficha_anterior_id INTEGER,
			data_arquivamento TEXT
		)
	`)
	if err != nil {
		dbRaw.Close()
		t.Fatalf("failed to setup legacy table: %v", err)
	}
	dbRaw.Close()

	db, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer db.Close()

	ctx := t.Context()
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = 11").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("expected version 11 to be recorded in schema_migrations")
	}
}
