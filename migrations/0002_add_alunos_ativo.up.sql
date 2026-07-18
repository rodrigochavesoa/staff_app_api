-- Migration Up: Add ativo column to Alunos table
ALTER TABLE alunos ADD COLUMN ativo INTEGER DEFAULT 1;
