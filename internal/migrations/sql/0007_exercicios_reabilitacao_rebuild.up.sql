-- Create sugestoes_exercicios_rehab table
CREATE TABLE IF NOT EXISTS sugestoes_exercicios_rehab (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nome_exercicio TEXT NOT NULL,
    tipo_exercicio TEXT,
    nivel_prioridade INTEGER DEFAULT 2,
    frequencia_sugestao INTEGER DEFAULT 1,
    exercicio_similar_nome TEXT,
    rag_fonte TEXT,
    justificativa_clinica TEXT,
    status TEXT DEFAULT 'pendente', -- 'pendente', 'aprovado', 'rejeitado'
    aprovado_em TIMESTAMP,
    aprovado_por TEXT,
    exercicio_reabilitacao_codigo INTEGER,
    notas_profissional TEXT,
    motivo_rejeicao TEXT,
    data_sugestao TIMESTAMP
);

-- Recreate exercicios_reabilitacao table with full monólito compatibility
-- (This serves as fallback if the database is initialized fresh)
CREATE TABLE IF NOT EXISTS exercicios_reabilitacao (
    codigo INTEGER PRIMARY KEY AUTOINCREMENT,
    nome TEXT UNIQUE NOT NULL,
    categoria TEXT DEFAULT 'normal', -- 'terapeutico' ou 'normal'
    descricao_terapeutica TEXT,
    descricao TEXT, -- Aliasing Go v1
    indicacoes TEXT,
    contraindicacoes TEXT,
    restricoes_sugeridas TEXT, -- Aliasing Go v1
    grupo_muscular TEXT,
    musculo_foco TEXT,
    tipo_exercicio TEXT,
    intensidade TEXT,
    nivel_prioridade INTEGER DEFAULT 2,
    fonte_cientifica TEXT,
    url TEXT,
    url_secundaria TEXT,
    video_url TEXT, -- Aliasing Go v1
    criado_por TEXT,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'ativo', -- 'ativo' ou 'inativo'
    notas_profissional TEXT,
    atualizado_em TIMESTAMP,
    atualizado_por TEXT
);
