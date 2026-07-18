-- Migration Down: Remove columns (not fully supported in older sqlite versions)
ALTER TABLE alunos DROP COLUMN ativo;
