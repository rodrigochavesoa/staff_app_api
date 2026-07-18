-- Migration Down: Remove periodization and tracking columns from fichas_treino_web
ALTER TABLE fichas_treino_web DROP COLUMN tipo_ficha;
ALTER TABLE fichas_treino_web DROP COLUMN num_treinos;
ALTER TABLE fichas_treino_web DROP COLUMN versao;
ALTER TABLE fichas_treino_web DROP COLUMN ficha_anterior_id;
ALTER TABLE fichas_treino_web DROP COLUMN data_arquivamento;
