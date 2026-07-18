-- Migration Down: Drop periodizacao_corrida_web table and indices
DROP INDEX IF EXISTS idx_periodizacao_corrida_web_periodizacao;
DROP INDEX IF EXISTS idx_periodizacao_corrida_web_hash;
DROP TABLE IF EXISTS periodizacao_corrida_web;
