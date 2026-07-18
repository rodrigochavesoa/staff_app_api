-- Migration Up: Create legacy fichas table if it does not exist
CREATE TABLE IF NOT EXISTS fichas (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    aluno_id INTEGER,
    feedback_rating INTEGER,
    FOREIGN KEY (aluno_id) REFERENCES alunos(id)
);
