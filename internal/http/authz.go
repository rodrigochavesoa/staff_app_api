package http

import (
	"context"
	"errors"

	"staff_app/internal/domain"
	"staff_app/internal/repositories"
)

var errUnauthorized = errors.New("unauthorized")

// LinkedAluno resolves the aluno profile linked to the authenticated user via alunos.usuario_id.
// Returns (nil, nil) when the user has no linked aluno.
func LinkedAluno(ctx context.Context, alunos repositories.AlunoRepository) (*domain.Aluno, error) {
	user, ok := UserFromContext(ctx)
	if !ok {
		return nil, errUnauthorized
	}
	return alunos.GetByUsuarioID(ctx, user.ID)
}
