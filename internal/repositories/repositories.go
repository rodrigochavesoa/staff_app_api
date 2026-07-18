package repositories

import (
	"context"
	"staff_app/internal/domain"
)

type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	Create(ctx context.Context, user *domain.User) error
	Update(ctx context.Context, user *domain.User) error
}

type AlunoRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.Aluno, error)
	List(ctx context.Context) ([]*domain.Aluno, error)
	Create(ctx context.Context, aluno *domain.Aluno) error
	Update(ctx context.Context, aluno *domain.Aluno) error
}

type FichaRepository interface {
	GetByHash(ctx context.Context, hash string) (*domain.FichaWeb, error)
	CreateFichaWeb(ctx context.Context, ficha *domain.FichaWeb) error
	UpdateFichaWeb(ctx context.Context, ficha *domain.FichaWeb) error
	ListByAlunoID(ctx context.Context, alunoID int64, includeExpired bool) ([]*domain.FichaWeb, error)
	IncrementAccess(ctx context.Context, hash string) error
}

type FeedbackRepository interface {
	CreateFeedback(ctx context.Context, fb *domain.FeedbackFicha) (int64, error)
	GetFeedbackByHash(ctx context.Context, hash string) (*domain.FeedbackFicha, error)
	ListPendingFeedbacks(ctx context.Context, userID *int64) ([]*domain.FeedbackFicha, error) // Can join with aluno details
	MarkNotificationLida(ctx context.Context, notificationID int64) error
	CreateNotification(ctx context.Context, feedbackID int64) error
}

type PeriodizacaoCorridaRepository interface {
	Create(ctx context.Context, pc *domain.PeriodizacaoCorrida) error
	CreateWithArchiveActive(ctx context.Context, pc *domain.PeriodizacaoCorrida, dataArquivamento string) error
	GetByID(ctx context.Context, id int64) (*domain.PeriodizacaoCorrida, error)
	ListByAlunoID(ctx context.Context, alunoID int64) ([]*domain.PeriodizacaoCorrida, error)
	Update(ctx context.Context, pc *domain.PeriodizacaoCorrida) error // Updates using OCC (WHERE id = ? AND versao = ?)
	Delete(ctx context.Context, id int64) error
	ArchiveActiveByAlunoID(ctx context.Context, alunoID int64, dataArquivamento string) (int64, error)
	
	CreatePublicLink(ctx context.Context, link *domain.PeriodizacaoCorridaWeb) error
	GetPublicLinkByHash(ctx context.Context, hash string) (*domain.PeriodizacaoCorridaWeb, error)
	GetPublicLinkByPeriodizacaoID(ctx context.Context, periodizacaoID int64) (*domain.PeriodizacaoCorridaWeb, error)
	UpdatePublicLink(ctx context.Context, link *domain.PeriodizacaoCorridaWeb) error
	IncrementPublicLinkAccess(ctx context.Context, hash string) error
}

type FichaTreinoRepository interface {
	Create(ctx context.Context, f *domain.FichaTreinoWeb) error
	GetByID(ctx context.Context, id int64) (*domain.FichaTreinoWeb, error)
	Update(ctx context.Context, f *domain.FichaTreinoWeb) error
	Delete(ctx context.Context, id int64) error
	ArchiveActiveByAlunoName(ctx context.Context, studentName string, dataArquivamento string) error
	CreatePeriodizadaWithArchiveAndLink(ctx context.Context, f *domain.FichaTreinoWeb, hash string, validDays int, alunoID int64) (string, error)
}

type ConfiguracaoRepository interface {
	GetByChave(ctx context.Context, chave string) (*domain.Configuracao, error)
	List(ctx context.Context) ([]*domain.Configuracao, error)
	Update(ctx context.Context, config *domain.Configuracao) error
}

type DashboardRepository interface {
	GetStats(ctx context.Context) (*domain.DashboardStats, error)
}

