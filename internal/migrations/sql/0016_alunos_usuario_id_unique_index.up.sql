CREATE UNIQUE INDEX IF NOT EXISTS idx_alunos_usuario_id ON alunos(usuario_id) WHERE usuario_id IS NOT NULL;
