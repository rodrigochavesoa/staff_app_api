-- Migration Up: Anamnese and Pre-Cadastro updates

-- 1. Create pre_registros_audit table
CREATE TABLE IF NOT EXISTS pre_registros_audit (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pre_registro_id INTEGER NOT NULL,
    evento TEXT NOT NULL,         -- 'CRIADO', 'APROVADO', 'REJEITADO', 'EXPIRADO'
    usuario_id INTEGER,           -- ID do Admin/Trainer que realizou a ação (NULL se sistêmico/público)
    detalhes TEXT,
    ip_origem TEXT,
    user_agent TEXT,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (pre_registro_id) REFERENCES pre_registros(id) ON DELETE CASCADE,
    FOREIGN KEY (usuario_id) REFERENCES users(id)
);

-- 2. Create anamnese_tokens_audit table
CREATE TABLE IF NOT EXISTS anamnese_tokens_audit (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token TEXT NOT NULL,
    aluno_id INTEGER,             -- NULL para pré-cadastros pendentes de aprovação
    pre_registro_id INTEGER,      -- NULL para geração via perfil de aluno existente
    evento TEXT NOT NULL,         -- 'GERADO', 'ENVIADO_EMAIL', 'VISUALIZADO', 'SUBMETIDO', 'EXPIRADO'
    ip TEXT,
    user_agent TEXT,
    detalhes TEXT,
    data_evento TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (aluno_id) REFERENCES alunos(id) ON DELETE SET NULL,
    FOREIGN KEY (pre_registro_id) REFERENCES pre_registros(id) ON DELETE SET NULL
);

-- 3. Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_pre_registros_audit_id ON pre_registros_audit(pre_registro_id);
CREATE INDEX IF NOT EXISTS idx_anamnese_tokens_audit_token ON anamnese_tokens_audit(token);
