-- Migration Down: Clean Initial Schema Setup

DROP INDEX IF EXISTS idx_ativo;
DROP INDEX IF EXISTS idx_aluno;
DROP INDEX IF EXISTS idx_expira;
DROP INDEX IF EXISTS idx_hash;

DROP INDEX IF EXISTS idx_treinos_data;
DROP INDEX IF EXISTS idx_treinos_aluno;
DROP INDEX IF EXISTS idx_treinos_ficha;

DROP TABLE IF EXISTS exercicios_reabilitacao;
DROP TABLE IF EXISTS pre_registros;
DROP TABLE IF EXISTS anamnese_tokens;
DROP TABLE IF EXISTS anamneses;
DROP TABLE IF EXISTS periodizacao_corrida;
DROP TABLE IF EXISTS teste_3km;
DROP TABLE IF EXISTS atividades_analytics;
DROP TABLE IF EXISTS atividades_records;
DROP TABLE IF EXISTS atividades_garmin;
DROP TABLE IF EXISTS feedback_notificacoes;
DROP TABLE IF EXISTS feedback_fichas;
DROP TABLE IF EXISTS treinos_realizados;
DROP TABLE IF EXISTS fichas_web;
DROP TABLE IF EXISTS fichas_treino_web;
DROP TABLE IF EXISTS alunos;
DROP TABLE IF EXISTS planos;
DROP TABLE IF EXISTS users;
