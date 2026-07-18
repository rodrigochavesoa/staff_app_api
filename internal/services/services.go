package services

import (
	"context"
	"staff_app/internal/domain"
)

type UserService interface {
	GetUserByID(ctx context.Context, id int64) (*domain.User, error)
	Authenticate(ctx context.Context, username, password string) (*domain.User, string, error) // Returns User, Token (JWT)
}

type AlunoService interface {
	GetAlunoByID(ctx context.Context, id int64) (*domain.Aluno, error)
	ListAlunos(ctx context.Context) ([]*domain.Aluno, error)
}

type FichaService interface {
	GetFichaByHash(ctx context.Context, hash string) (*domain.FichaWeb, error)
	CreateFichaWeb(ctx context.Context, fichaID, alunoID int64, userID *int64, content map[string]any) (*domain.FichaWeb, error)
	RenewFichaWeb(ctx context.Context, hash string, days int) (*domain.FichaWeb, error)
	DeactivateFichaWeb(ctx context.Context, hash string) error
	ListAlunoFichas(ctx context.Context, alunoID int64, includeExpired bool) ([]*domain.FichaWeb, error)
}

type FeedbackService interface {
	SubmitFeedback(ctx context.Context, hash string, rating int, comment string) error
	GetFeedback(ctx context.Context, hash string) (*domain.FeedbackFicha, error)
}

type PeriodizacaoCorridaService interface {
	Generate(ctx context.Context, req *domain.PeriodizacaoRequest, coachUserID *int64) (*domain.PeriodizacaoCorrida, error)
	GetByID(ctx context.Context, id int64) (*domain.PeriodizacaoCorrida, error)
	ListByAlunoID(ctx context.Context, alunoID int64) ([]*domain.PeriodizacaoCorrida, error)
	Delete(ctx context.Context, id int64) error
	EditTreino(ctx context.Context, id int64, semana, dia int, tipo string, dist float64, zona, pace, desc string) error
	UpdateSemanaAtual(ctx context.Context, id int64, semanaAtual int) error

	GeneratePublicLink(ctx context.Context, id int64, userID *int64) (*domain.PeriodizacaoCorridaWeb, error)
	GetPublicLinkByHash(ctx context.Context, hash string) (*domain.PeriodizacaoCorridaWeb, error)
	GetPublicLinkByPeriodizacaoID(ctx context.Context, periodizacaoID int64) (*domain.PeriodizacaoCorridaWeb, error)
	RenewPublicLink(ctx context.Context, id int64) (*domain.PeriodizacaoCorridaWeb, error)
	DeactivatePublicLink(ctx context.Context, id int64) error
	GetPublicPlano(ctx context.Context, hash string) (*domain.PeriodizacaoCorrida, error)
	ConcluirTreinoPublico(ctx context.Context, hash string, semana, dia int, concluido bool) error
}

