-- Migration Up: Create configuracoes_sistema table

CREATE TABLE IF NOT EXISTS configuracoes_sistema (
    chave TEXT PRIMARY KEY,
    valor TEXT NOT NULL,
    tipo TEXT NOT NULL DEFAULT 'string', -- 'string', 'boolean', 'int', 'float', 'json'
    sensivel INTEGER DEFAULT 0,
    descricao TEXT,
    atualizado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    atualizado_por INTEGER,
    FOREIGN KEY (atualizado_por) REFERENCES users(id)
);

-- Seed basic default configuration values
INSERT OR IGNORE INTO configuracoes_sistema (chave, valor, tipo, sensivel, descricao) VALUES
('SMTP_ENABLED', 'false', 'boolean', 0, 'Habilita envio de e-mails via SMTP'),
('SMTP_HOST', '', 'string', 0, 'Endereço do servidor SMTP'),
('SMTP_PORT', '587', 'int', 0, 'Porta do servidor SMTP'),
('SMTP_USER', '', 'string', 0, 'Usuário de autenticação SMTP'),
('SMTP_PASSWORD', '', 'string', 1, 'Senha de autenticação SMTP'),
('SMTP_FROM_EMAIL', '', 'string', 0, 'E-mail do remetente padrão'),
('SMTP_FROM_NAME', 'Sistema RC Staff', 'string', 0, 'Nome do remetente padrão'),
('PRE_REGISTRO_EXPIRACAO_HORAS', '72', 'int', 0, 'Prazo de expiração de um pré-cadastro em horas'),
('AUTO_GENERATE_ANAMNESE_ON_APPROVE', 'true', 'boolean', 0, 'Gera link de anamnese automaticamente ao aprovar aluno'),
('AUTO_SEND_ANAMNESE_EMAIL', 'false', 'boolean', 0, 'Envia o e-mail de anamnese imediatamente após aprovar o aluno'),
('BLOCOS_DINAMICOS_PADRAO', 'false', 'boolean', 0, 'Habilita o uso de blocos de periodização dinâmicos para novos planos');
