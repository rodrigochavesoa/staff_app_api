package migrations

import (
	"embed"
	"fmt"
)

//go:embed sql/*.sql
var migrationFiles embed.FS

// Migration represents a versioned SQL migration.
type Migration struct {
	Version int
	Name    string
	UpSQL   string
}

// GetMigrations returns all defined migrations in order of version.
func GetMigrations() ([]Migration, error) {
	files := []struct {
		Version int
		Name    string
		Path    string
	}{
		{Version: 1, Name: "init_schema", Path: "sql/0001_init_schema.up.sql"},
		{Version: 2, Name: "add_alunos_ativo", Path: "sql/0002_add_alunos_ativo.up.sql"},
		{Version: 3, Name: "create_fichas_web_tables", Path: "sql/0003_create_fichas_web_tables.up.sql"},
		{Version: 4, Name: "create_legacy_fichas_table", Path: "sql/0004_create_legacy_fichas_table.up.sql"},
		{Version: 5, Name: "auth_users_plans_update", Path: "sql/0005_auth_users_plans_update.up.sql"},
		{Version: 6, Name: "anamnese_pre_cadastro_updates", Path: "sql/0006_anamnese_pre_cadastro_updates.up.sql"},
		{Version: 7, Name: "exercicios_reabilitacao_rebuild", Path: "sql/0007_exercicios_reabilitacao_rebuild.up.sql"},
		{Version: 8, Name: "create_periodizacao_corrida_web", Path: "sql/0008_create_periodizacao_corrida_web.up.sql"},
		{Version: 9, Name: "create_configuracoes_sistema", Path: "sql/0009_create_configuracoes_sistema.up.sql"},
		{Version: 10, Name: "create_historico_fichas_table", Path: "sql/0010_create_historico_fichas_table.up.sql"},
		{Version: 11, Name: "alter_fichas_treino_web", Path: "sql/0011_alter_fichas_treino_web.up.sql"},
		{Version: 12, Name: "alter_fichas_treino_web_sved", Path: "sql/0012_alter_fichas_treino_web_sved.up.sql"},
		{Version: 13, Name: "create_consultas_base_conhecimento", Path: "sql/0013_create_consultas_base_conhecimento.up.sql"},
		{Version: 14, Name: "seed_base_conhecimento", Path: "sql/0014_seed_base_conhecimento.up.sql"},
		{Version: 15, Name: "create_training_pipeline_events", Path: "sql/0015_create_training_pipeline_events.up.sql"},
	}

	migrations := make([]Migration, len(files))
	for i, f := range files {
		data, err := migrationFiles.ReadFile(f.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %d (%s): %w", f.Version, f.Name, err)
		}
		migrations[i] = Migration{
			Version: f.Version,
			Name:    f.Name,
			UpSQL:   string(data),
		}
	}

	return migrations, nil
}
