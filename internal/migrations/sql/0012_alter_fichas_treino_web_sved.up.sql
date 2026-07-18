ALTER TABLE fichas_treino_web ADD COLUMN ies_score REAL DEFAULT 0.0;
ALTER TABLE fichas_treino_web ADD COLUMN volume_sved INTEGER DEFAULT 0;
ALTER TABLE fichas_treino_web ADD COLUMN densidade REAL DEFAULT 0.0;
ALTER TABLE fichas_treino_web ADD COLUMN tut_total INTEGER DEFAULT 0;
ALTER TABLE fichas_treino_web ADD COLUMN series TEXT;
ALTER TABLE fichas_treino_web ADD COLUMN rir INTEGER DEFAULT 2;
ALTER TABLE fichas_treino_web ADD COLUMN cadencia TEXT DEFAULT '4010';
ALTER TABLE fichas_treino_web ADD COLUMN rest_seconds INTEGER DEFAULT 60;
