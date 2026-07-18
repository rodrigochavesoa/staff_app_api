-- Migration Up: Add periodization and tracking columns to fichas_treino_web
ALTER TABLE fichas_treino_web ADD COLUMN tipo_ficha TEXT DEFAULT 'manual';
ALTER TABLE fichas_treino_web ADD COLUMN num_treinos INTEGER DEFAULT 1;
ALTER TABLE fichas_treino_web ADD COLUMN versao INTEGER DEFAULT 1;
ALTER TABLE fichas_treino_web ADD COLUMN ficha_anterior_id INTEGER;
ALTER TABLE fichas_treino_web ADD COLUMN data_arquivamento TEXT;
