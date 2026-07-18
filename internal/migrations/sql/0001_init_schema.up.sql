-- Migration Up: Initial Schema Setup

-- 1. Users Table
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    is_admin INTEGER DEFAULT 0,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ultimo_login TIMESTAMP,
    ativo INTEGER DEFAULT 1,
    foto_perfil TEXT
);

-- 2. Planos Table
CREATE TABLE IF NOT EXISTS planos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nome TEXT UNIQUE NOT NULL,
    preco_default REAL NOT NULL,
    descricao TEXT,
    ativo INTEGER DEFAULT 1
);

-- 3. Alunos Table
CREATE TABLE IF NOT EXISTS alunos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nome TEXT NOT NULL,
    idade INTEGER NOT NULL,
    sexo TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    telefone TEXT,
    objetivo TEXT,
    exclusoes_permanentes TEXT,
    turma TEXT,
    usuario_id INTEGER,
    plano_id INTEGER,
    plano_valor REAL,
    plano_pago INTEGER DEFAULT 0,
    plano_ativo INTEGER DEFAULT 1,
    plano_inicio TEXT,
    plano_fim TEXT,
    cadastro_aprovado INTEGER DEFAULT 0,
    cadastro_aprovado_por INTEGER,
    cadastro_aprovado_em TIMESTAMP,
    pre_registro_id INTEGER,
    FOREIGN KEY (usuario_id) REFERENCES users(id),
    FOREIGN KEY (plano_id) REFERENCES planos(id),
    FOREIGN KEY (cadastro_aprovado_por) REFERENCES users(id)
);

-- 4. Fichas Treino Web Table
CREATE TABLE IF NOT EXISTS fichas_treino_web (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    aluno TEXT,
    idade INTEGER,
    sexo TEXT,
    objetivo TEXT,
    modalidade TEXT,
    nivel TEXT,
    frequencia_semanal INTEGER,
    duracao_treino INTEGER,
    restricoes TEXT,
    feedback TEXT,
    turma TEXT,
    lista_exercicios TEXT DEFAULT 'exercicios_com_grupos',
    data_criacao TEXT DEFAULT CURRENT_TIMESTAMP,
    ficha_json TEXT
);

-- 5. Fichas Web (Public Links) Table
CREATE TABLE IF NOT EXISTS fichas_web (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hash TEXT UNIQUE NOT NULL,
    ficha_id INTEGER NOT NULL,
    aluno_id INTEGER NOT NULL,
    user_id INTEGER,
    conteudo_json TEXT NOT NULL,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expira_em TIMESTAMP NOT NULL,
    acessos INTEGER DEFAULT 0,
    ultimo_acesso TIMESTAMP,
    ativo INTEGER DEFAULT 1,
    renovado_de INTEGER,
    FOREIGN KEY (ficha_id) REFERENCES fichas_treino_web(id),
    FOREIGN KEY (aluno_id) REFERENCES alunos(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- 6. Treinos Realizados Table
CREATE TABLE IF NOT EXISTS treinos_realizados (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ficha_id INTEGER NOT NULL,
    aluno_id INTEGER,
    hash_ficha TEXT,
    data_treino DATE NOT NULL,
    tipo_treino TEXT,
    tipo_ficha TEXT NOT NULL,
    observacao TEXT,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(ficha_id, data_treino),
    FOREIGN KEY (ficha_id) REFERENCES fichas_treino_web(id),
    FOREIGN KEY (aluno_id) REFERENCES alunos(id)
);

-- 7. Feedback Fichas Table
CREATE TABLE IF NOT EXISTS feedback_fichas (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hash_ficha TEXT UNIQUE NOT NULL,
    aluno_id INTEGER NOT NULL,
    rating INTEGER NOT NULL,
    comentario TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (aluno_id) REFERENCES alunos(id)
);

-- 8. Feedback Notificacoes Table
CREATE TABLE IF NOT EXISTS feedback_notificacoes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feedback_id INTEGER NOT NULL,
    user_id INTEGER,
    lido INTEGER DEFAULT 0,
    lido_em TIMESTAMP,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (feedback_id) REFERENCES feedback_fichas(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- 9. Atividades Garmin Table
CREATE TABLE IF NOT EXISTS atividades_garmin (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    aluno_id INTEGER NOT NULL,
    file_nome TEXT NOT NULL,
    activity_type TEXT,
    start_time TIMESTAMP,
    duration_seconds INTEGER,
    distance_meters REAL,
    elevation_gain_m REAL,
    elevation_loss_m REAL,
    calories INTEGER,
    avg_power_watts REAL,
    threshold_power REAL,
    avg_heart_rate REAL,
    max_heart_rate REAL,
    avg_cadence REAL,
    avg_speed_kmh REAL,
    max_speed_kmh REAL,
    aerobic_te REAL,
    anaerobic_te REAL,
    FOREIGN KEY (aluno_id) REFERENCES alunos(id)
);

-- 10. Atividades Records Table
CREATE TABLE IF NOT EXISTS atividades_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    atividade_id INTEGER NOT NULL,
    timestamp TIMESTAMP,
    latitude REAL,
    longitude REAL,
    altitude_m REAL,
    heart_rate INTEGER,
    cadence INTEGER,
    speed_kmh REAL,
    power_watts REAL,
    raw_data TEXT,
    FOREIGN KEY (atividade_id) REFERENCES atividades_garmin(id) ON DELETE CASCADE
);

-- 11. Atividades Analytics Table
CREATE TABLE IF NOT EXISTS atividades_analytics (
    atividade_id INTEGER PRIMARY KEY,
    tss_score REAL,
    heart_rate_variability REAL,
    FOREIGN KEY (atividade_id) REFERENCES atividades_garmin(id) ON DELETE CASCADE
);

-- 12. Teste 3km Table
CREATE TABLE IF NOT EXISTS teste_3km (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    aluno_id INTEGER NOT NULL,
    data_teste TEXT NOT NULL,
    tempo_segundos INTEGER NOT NULL,
    pse INTEGER,
    fonte TEXT NOT NULL,
    vdot REAL NOT NULL,
    ftp_pace_segundos INTEGER NOT NULL,
    pace_z1_min INTEGER, pace_z1_max INTEGER,
    pace_z2_min INTEGER, pace_z2_max INTEGER,
    pace_z3_min INTEGER, pace_z3_max INTEGER,
    pace_z4_min INTEGER, pace_z4_max INTEGER,
    pace_z5_min INTEGER, pace_z5_max INTEGER,
    indice_confianca INTEGER,
    observacoes TEXT,
    FOREIGN KEY (aluno_id) REFERENCES alunos(id)
);

-- 13. Periodizacao Corrida Table
CREATE TABLE IF NOT EXISTS periodizacao_corrida (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    aluno_id INTEGER NOT NULL,
    data_inicio TEXT NOT NULL,
    duracao_semanas INTEGER NOT NULL,
    modo TEXT NOT NULL,
    semana_atual INTEGER DEFAULT 1,
    status TEXT DEFAULT 'ativo',
    distancia_prova REAL NOT NULL,
    nivel TEXT NOT NULL,
    vdot REAL NOT NULL,
    pace_base INTEGER,
    volume_semanal REAL,
    dias_disponiveis INTEGER,
    plano_json TEXT,
    modo_geracao TEXT,
    data_ultima_geracao TEXT,
    dias_semana_selecionados TEXT,
    versao INTEGER DEFAULT 1,
    ficha_anterior_id INTEGER,
    data_arquivamento TEXT,
    FOREIGN KEY (aluno_id) REFERENCES alunos(id)
);

-- 14. Anamneses Table
CREATE TABLE IF NOT EXISTS anamneses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    aluno_id INTEGER,
    data_nascimento TEXT,
    idade INTEGER,
    sexo TEXT,
    altura REAL,
    peso REAL,
    telefone TEXT,
    email TEXT,
    patologias TEXT,
    medicamentos TEXT,
    lesoes_atuais TEXT,
    dores_cronicas TEXT,
    parq_doenca_cardiaca INTEGER,
    parq_dor_peito INTEGER,
    parq_tontura INTEGER,
    parq_problema_osseo INTEGER,
    parq_medicamento_pressao INTEGER,
    parq_impedimento_activity INTEGER,
    experiencia_treino TEXT,
    objetivo_principal TEXT,
    contato_emergencia_nome TEXT,
    contato_emergencia_telefone TEXT,
    risk_score_cached REAL,
    preenchido_por TEXT,
    ativa INTEGER DEFAULT 1,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    pre_registro_id INTEGER,
    FOREIGN KEY (aluno_id) REFERENCES alunos(id)
);

-- 15. Anamnese Tokens Table
CREATE TABLE IF NOT EXISTS anamnese_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token TEXT UNIQUE NOT NULL,
    pre_registro_id INTEGER NOT NULL,
    expira_em TIMESTAMP NOT NULL,
    usado INTEGER DEFAULT 0
);

-- 16. Pre Registros Table
CREATE TABLE IF NOT EXISTS pre_registros (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nome TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    telefone TEXT,
    data_nascimento TEXT,
    genero TEXT,
    payment_ref TEXT,
    plano_id INTEGER,
    plano_valor REAL,
    ip_origem TEXT,
    user_agent TEXT,
    expira_em TIMESTAMP NOT NULL,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usado INTEGER DEFAULT 0
);

-- 17. Exercicios Reabilitacao Table
CREATE TABLE IF NOT EXISTS exercicios_reabilitacao (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    nome TEXT UNIQUE NOT NULL,
    descricao TEXT,
    grupo_muscular TEXT,
    restricoes_sugeridas TEXT,
    video_url TEXT
);

-- Indices for performance
CREATE INDEX IF NOT EXISTS idx_hash ON fichas_web(hash);
CREATE INDEX IF NOT EXISTS idx_expira ON fichas_web(expira_em);
CREATE INDEX IF NOT EXISTS idx_aluno ON fichas_web(aluno_id);
CREATE INDEX IF NOT EXISTS idx_ativo ON fichas_web(ativo);

CREATE INDEX IF NOT EXISTS idx_treinos_ficha ON treinos_realizados(ficha_id);
CREATE INDEX IF NOT EXISTS idx_treinos_aluno ON treinos_realizados(aluno_id);
CREATE INDEX IF NOT EXISTS idx_treinos_data ON treinos_realizados(data_treino);
