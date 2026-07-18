-- Migration Down: Anamnese and Pre-Cadastro updates rollback

DROP INDEX IF EXISTS idx_anamnese_tokens_audit_token;
DROP INDEX IF EXISTS idx_pre_registros_audit_id;
DROP TABLE IF EXISTS anamnese_tokens_audit;
DROP TABLE IF EXISTS pre_registros_audit;
