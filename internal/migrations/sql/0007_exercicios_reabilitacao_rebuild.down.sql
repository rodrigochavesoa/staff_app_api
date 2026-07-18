-- Drop sugestoes_exercicios_rehab table
DROP TABLE IF EXISTS sugestoes_exercicios_rehab;

-- Recreate exercicios_reabilitacao with original simple schema
DROP TABLE IF EXISTS exercicios_reabilitacao;
CREATE TABLE IF NOT EXISTS exercicios_reabilitacao (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nome TEXT UNIQUE NOT NULL,
    descricao TEXT,
    grupo_muscular TEXT,
    restricoes_sugeridas TEXT,
    video_url TEXT
);
