-- Migration Up: 0010_create_historico_fichas_table.up.sql
CREATE TABLE IF NOT EXISTS historico_fichas (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    aluno_id INTEGER NOT NULL,
    tipo_ficha TEXT NOT NULL, -- 'musculacao' ou 'corrida'
    versao INTEGER DEFAULT 1,
    status TEXT DEFAULT 'arquivada',
    data_criacao TEXT,
    data_arquivamento TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    data_inicio_uso TEXT, -- Exclusivo de corrida
    ficha_origem_id INTEGER,
    ficha_origem_tabela TEXT, -- 'fichas_treino_web' ou 'periodizacao_corrida'
    ficha_json TEXT, -- Snapshots em formato JSON estruturado
    plano_json TEXT, -- Exclusivo de corrida
    vdot REAL, -- Exclusivo de corrida
    pace_base TEXT, -- Exclusivo de corrida
    distancia_prova REAL, -- Exclusivo de corrida
    nivel TEXT, -- Exclusivo de corrida
    duracao_semanas INTEGER, -- Exclusivo de corrida
    modo TEXT, -- Exclusivo de corrida
    semanas_completadas INTEGER DEFAULT 0, -- Exclusivo de corrida
    taxa_completude REAL DEFAULT 0.0,
    feedback_dificuldade_medio REAL DEFAULT 0.0,
    dores_reportadas TEXT, -- Exclusivo de corrida
    total_treinos_planejados INTEGER DEFAULT 0,
    total_treinos_realizados INTEGER DEFAULT 0,
    dias_uso INTEGER DEFAULT 0,
    objetivo TEXT, -- Exclusivo de musculação
    modalidade TEXT, -- Exclusivo de musculação
    frequencia_semanal INTEGER, -- Exclusivo de musculação
    observacoes_gerais TEXT,
    coach_notes TEXT,
    FOREIGN KEY (aluno_id) REFERENCES alunos(id)
);

CREATE INDEX IF NOT EXISTS idx_historico_aluno ON historico_fichas(aluno_id);
CREATE INDEX IF NOT EXISTS idx_historico_tipo ON historico_fichas(tipo_ficha);
