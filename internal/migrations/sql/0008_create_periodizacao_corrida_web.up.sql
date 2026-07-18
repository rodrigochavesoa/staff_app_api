-- Migration Up: Create table periodizacao_corrida_web and indices
CREATE TABLE IF NOT EXISTS periodizacao_corrida_web (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hash TEXT UNIQUE NOT NULL,
    periodizacao_id INTEGER NOT NULL,
    aluno_id INTEGER NOT NULL,
    user_id INTEGER,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expira_em TIMESTAMP NOT NULL,
    acessos INTEGER DEFAULT 0,
    ultimo_acesso TIMESTAMP,
    ativo INTEGER DEFAULT 1,
    FOREIGN KEY (periodizacao_id) REFERENCES periodizacao_corrida(id) ON DELETE CASCADE,
    FOREIGN KEY (aluno_id) REFERENCES alunos(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_periodizacao_corrida_web_hash ON periodizacao_corrida_web(hash);
CREATE INDEX IF NOT EXISTS idx_periodizacao_corrida_web_periodizacao ON periodizacao_corrida_web(periodizacao_id);
