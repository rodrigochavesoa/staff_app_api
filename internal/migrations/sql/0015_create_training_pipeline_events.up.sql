CREATE TABLE IF NOT EXISTS training_pipeline_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    aluno_id INTEGER NOT NULL,
    endpoint TEXT NOT NULL,
    complexity TEXT NOT NULL,
    evidence_requested INTEGER NOT NULL DEFAULT 0,
    evidence_count INTEGER NOT NULL DEFAULT 0,
    evidence_fallback_used INTEGER NOT NULL DEFAULT 0,
    safety_rejected INTEGER NOT NULL DEFAULT 0,
    quality_warnings INTEGER NOT NULL DEFAULT 0,
    ai_fallback_used INTEGER NOT NULL DEFAULT 0,
    provider TEXT,
    duration_ms INTEGER,
    criado_em TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_training_pipeline_events_criado
ON training_pipeline_events(criado_em);

CREATE INDEX IF NOT EXISTS idx_training_pipeline_events_complexity
ON training_pipeline_events(complexity, evidence_count);
