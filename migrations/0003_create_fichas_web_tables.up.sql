-- Migration Up: Create tables for fichas_web accesses and feedback
CREATE TABLE IF NOT EXISTS fichas_web_acessos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hash TEXT NOT NULL,
    data_acesso TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_agent TEXT,
    ip_address TEXT,
    FOREIGN KEY (hash) REFERENCES fichas_web(hash)
);

CREATE TABLE IF NOT EXISTS fichas_web_feedback (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ficha_id INTEGER NOT NULL,
    rating INTEGER NOT NULL,
    comentario TEXT,
    data_feedback TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (ficha_id) REFERENCES fichas_treino_web(id)
);
