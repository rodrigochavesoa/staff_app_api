package http

import (
	"context"
	"errors"
	"net/http"

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

// RequireAlunoOwnerOrAdmin allows admins any alunoID; non-admins only their linked aluno.
// On failure it writes the HTTP error response and returns false.
func RequireAlunoOwnerOrAdmin(w http.ResponseWriter, r *http.Request, alunos repositories.AlunoRepository, alunoID int64) bool {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	if user.IsAdmin {
		return true
	}

	linked, err := LinkedAluno(r.Context(), alunos)
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return false
	}
	if linked == nil || linked.ID != alunoID {
		writeJSONError(w, "Acesso negado.", http.StatusForbidden)
		return false
	}
	return true
}
