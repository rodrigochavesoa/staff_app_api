-- Migration Down: 0010_create_historico_fichas_table.down.sql
DROP INDEX IF EXISTS idx_historico_aluno;
DROP INDEX IF EXISTS idx_historico_tipo;
DROP TABLE IF EXISTS historico_fichas;
