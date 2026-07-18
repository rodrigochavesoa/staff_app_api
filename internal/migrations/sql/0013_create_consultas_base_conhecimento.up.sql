CREATE TABLE IF NOT EXISTS consultas_base_conhecimento (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    query_original TEXT NOT NULL,
    query_normalizada TEXT NOT NULL,
    modalidade TEXT,
    objetivo TEXT,
    perfil TEXT,
    k INTEGER NOT NULL DEFAULT 3,
    total_resultados INTEGER DEFAULT 0,
    hits INTEGER DEFAULT 1,
    resultados_json TEXT NOT NULL,
    usuario_id INTEGER NULL,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ultima_utilizacao TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (usuario_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_consultas_base_cache_key
ON consultas_base_conhecimento(
    query_normalizada,
    COALESCE(modalidade, ''),
    COALESCE(objetivo, ''),
    COALESCE(perfil, ''),
    k
);

CREATE INDEX IF NOT EXISTS idx_consultas_base_ultima
ON consultas_base_conhecimento(ultima_utilizacao);

CREATE TABLE IF NOT EXISTS base_conhecimento_documentos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    fonte TEXT NOT NULL,
    titulo TEXT,
    conteudo TEXT NOT NULL,
    tags TEXT,
    modalidade TEXT,
    ativo INTEGER DEFAULT 1,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_base_conhecimento_docs_ativo
ON base_conhecimento_documentos(ativo, modalidade);
