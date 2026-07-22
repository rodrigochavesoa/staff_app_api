package repositories

import (
	"context"
	"time"

	"staff_app/internal/domain"
)

// DatabaseHealth is the minimal DB surface used by health checks.
type DatabaseHealth interface {
	PingContext(ctx context.Context) error
}

type UserRepository interface {
	Count(ctx context.Context) (int, error)
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	GetByLogin(ctx context.Context, login string) (*domain.User, error)
	List(ctx context.Context) ([]*domain.User, error)
	UpdateLastLogin(ctx context.Context, id int64, t time.Time) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	Approve(ctx context.Context, id int64) error
	RejectPending(ctx context.Context, id int64) error
	ToggleActive(ctx context.Context, id int64) error
}

type AlunoRepository interface {
	Create(ctx context.Context, aluno *domain.Aluno) error
	GetByID(ctx context.Context, id int64) (*domain.Aluno, error)
	GetByEmail(ctx context.Context, email string) (*domain.Aluno, error)
	GetByUsuarioID(ctx context.Context, userID int64) (*domain.Aluno, error)
	List(ctx context.Context, busca string, includeInactives bool) ([]*domain.Aluno, error)
	Update(ctx context.Context, aluno *domain.Aluno) error
	Delete(ctx context.Context, id int64) error
	Reactivate(ctx context.Context, id int64) error
}

// FichaRepository is implemented by sqlite.FichaWebRepository.
type FichaRepository interface {
	GetFichaJSON(ctx context.Context, id int64) (string, error)
	Create(ctx context.Context, ficha *domain.FichaWeb) error
	GetByHash(ctx context.Context, hash string) (*domain.FichaWeb, error)
	IncrementAccessCount(ctx context.Context, hash string, userAgent, ipAddress string) error
	GetStats(ctx context.Context, hash string) (*domain.FichaWebStats, error)
	Renew(ctx context.Context, hash string, newExpiration time.Time, newContent *string) error
	Deactivate(ctx context.Context, hash string) error
	ListByAlunoID(ctx context.Context, alunoID int64, includeExpired bool) ([]*domain.FichaWeb, error)
}

type FeedbackRepository interface {
	CreateFeedback(ctx context.Context, fb *domain.FeedbackFicha) (int64, error)
	GetFeedbackByHash(ctx context.Context, hash string) (*domain.FeedbackFicha, error)
	ListPendingFeedbacks(ctx context.Context, userID *int64) ([]*domain.FeedbackFicha, error)
	MarkNotificationLida(ctx context.Context, notificationID int64) error
}

type PeriodizacaoCorridaRepository interface {
	CreateWithArchiveActive(ctx context.Context, pc *domain.PeriodizacaoCorrida, dataArquivamento string) error
	GetByID(ctx context.Context, id int64) (*domain.PeriodizacaoCorrida, error)
	ListByAlunoID(ctx context.Context, alunoID int64) ([]*domain.PeriodizacaoCorrida, error)
	Update(ctx context.Context, pc *domain.PeriodizacaoCorrida) error
	Delete(ctx context.Context, id int64) error
	CreatePublicLink(ctx context.Context, link *domain.PeriodizacaoCorridaWeb) error
	GetPublicLinkByHash(ctx context.Context, hash string) (*domain.PeriodizacaoCorridaWeb, error)
	GetPublicLinkByPeriodizacaoID(ctx context.Context, periodizacaoID int64) (*domain.PeriodizacaoCorridaWeb, error)
	UpdatePublicLink(ctx context.Context, link *domain.PeriodizacaoCorridaWeb) error
	IncrementPublicLinkAccess(ctx context.Context, hash string) error
}

type FichaTreinoRepository interface {
	Create(ctx context.Context, f *domain.FichaTreinoWeb) error
	GetByID(ctx context.Context, id int64) (*domain.FichaTreinoWeb, error)
	ListActiveByAlunoNome(ctx context.Context, alunoNome string) ([]*domain.FichaTreinoListItem, error)
	Update(ctx context.Context, f *domain.FichaTreinoWeb) error
	Delete(ctx context.Context, id int64) error
	CreatePeriodizadaWithArchiveAndLink(ctx context.Context, f *domain.FichaTreinoWeb, hash string, validDays int, alunoID int64) (string, error)
}

type ConfiguracaoRepository interface {
	List(ctx context.Context) ([]*domain.Configuracao, error)
	UpdateMultiple(ctx context.Context, configs []*domain.Configuracao) error
}

type DashboardRepository interface {
	GetStats(ctx context.Context) (*domain.DashboardStats, error)
}

type AnamneseRepository interface {
	Create(ctx context.Context, a *domain.Anamnese) error
	FindByID(ctx context.Context, id int64) (*domain.Anamnese, error)
	FindActiveByAlunoID(ctx context.Context, alunoID int64) (*domain.Anamnese, error)
	List(ctx context.Context) ([]domain.Anamnese, error)
	ListPending(ctx context.Context) ([]domain.Anamnese, error)
	Update(ctx context.Context, a *domain.Anamnese) error
	DeactivateAllPreviousForAluno(ctx context.Context, alunoID int64, currentAnamneseID int64) error
	Delete(ctx context.Context, id int64) error
	CreateToken(ctx context.Context, t *domain.AnamneseToken) error
	FindToken(ctx context.Context, tokenStr string) (*domain.AnamneseToken, error)
	UpdateToken(ctx context.Context, t *domain.AnamneseToken) error
	AddTokenAudit(ctx context.Context, a *domain.AnamneseTokenAudit) error
}

type PreRegistroRepository interface {
	Create(ctx context.Context, p *domain.PreRegistro) error
	FindByID(ctx context.Context, id int64) (*domain.PreRegistro, error)
	FindByEmail(ctx context.Context, email string) (*domain.PreRegistro, error)
	List(ctx context.Context, status string, nomeQuery string) ([]domain.PreRegistro, error)
	Update(ctx context.Context, p *domain.PreRegistro) error
	AddAudit(ctx context.Context, a *domain.PreRegistroAudit) error
	GetAuditTrail(ctx context.Context, preRegistroID int64) ([]domain.PreRegistroAudit, error)
}

type PlanoRepository interface {
	List(ctx context.Context, includeInactive bool) ([]*domain.Plano, error)
	Create(ctx context.Context, p *domain.Plano) error
	Update(ctx context.Context, p *domain.Plano) error
	Deactivate(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (*domain.Plano, error)
}

type HistoricoRepository interface {
	SearchAlunos(ctx context.Context, q string, limit int, ativo string) ([]*domain.AlunoSearchResponse, error)
	GetTreinosRealizadosByAluno(ctx context.Context, alunoID int64, mes, ano int) ([]*domain.TreinoRealizado, error)
	MarkTreinoRealizado(ctx context.Context, tr *domain.TreinoRealizado) error
	UnmarkTreinoRealizado(ctx context.Context, fichaID int64, dataTreino string) error
	GetHistoricoFichaByID(ctx context.Context, id int64) (*domain.HistoricoFicha, error)
	// Resolve aluno from musculação ficha when marking treinos as authenticated trainer.
	GetFichaTreinoAlunoNomeByID(ctx context.Context, fichaID int64) (string, error)
	GetAlunoIDByNome(ctx context.Context, nome string) (int64, error)
}

type ExercicioRepository interface {
	GetByCodigo(ctx context.Context, codigo int) (*domain.ExercicioReabilitacao, error)
	GetByNome(ctx context.Context, nome string) (*domain.ExercicioReabilitacao, error)
	GetMaxCodigoInRange(ctx context.Context, min, max int) (int, error)
	Create(ctx context.Context, ex *domain.ExercicioReabilitacao) error
	Update(ctx context.Context, ex *domain.ExercicioReabilitacao) error
	Delete(ctx context.Context, codigo int) error
	List(ctx context.Context, filters map[string]string) ([]*domain.ExercicioReabilitacao, error)
	GetUniqueGrupos(ctx context.Context) ([]string, error)
	GetUniqueTipos(ctx context.Context) ([]string, error)
	GetEstatisticas(ctx context.Context) (map[string]interface{}, error)
	GetSugestaoByID(ctx context.Context, id int) (*domain.SugestaoExercicioRehab, error)
	ListSugestoes(ctx context.Context, priorityFilter *int, order string) ([]*domain.SugestaoExercicioRehab, error)
	AprovarSugestao(ctx context.Context, sugestaoID int, ex *domain.ExercicioReabilitacao, approvedBy string) (int, error)
	RejeitarSugestao(ctx context.Context, id int, motivo string, rejectedBy string) error
	// Catalog sync (cmd/api + csvsync.SyncRepository).
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
	UpsertCatalogExercise(ctx context.Context, ex *domain.ExercicioReabilitacao, existing *domain.ExercicioReabilitacao) (inserted bool, err error)
}

type GarminRepository interface {
	SaveActivity(ctx context.Context, activity *domain.GarminActivity) (int64, error)
	Activity(ctx context.Context, id int64) (*domain.GarminActivity, error)
	ActivityRecords(ctx context.Context, activityID int64, limit int) ([]domain.ActivityRecord, error)
	ActivityAnalytics(ctx context.Context, activityID int64) (*domain.GarminAnalytics, error)
	ListAlunoActivities(ctx context.Context, alunoID int64, activityType string, limit, offset int) ([]*domain.GarminActivity, int, error)
	AlunoStats(ctx context.Context, alunoID int64) (*domain.GarminStats, error)
	DeleteActivity(ctx context.Context, id int64) error
	MaxHeartRate(ctx context.Context, alunoID int64) (int, error)
	HeartRateSamples(ctx context.Context, alunoID int64, limit int) ([]domain.HeartRateSample, bool, error)
	CaloriesSamples(ctx context.Context, alunoID int64, limit int) ([]domain.CaloriesSample, error)
}

type RAGRepository interface {
	GetCachedQuery(ctx context.Context, queryNorm, modalidade, objetivo, perfil string, k int) (*domain.ConsultaBaseConhecimento, error)
	IncrementCacheHits(ctx context.Context, id int64) error
	SaveCachedQuery(ctx context.Context, queryOrig, queryNorm, modalidade, objetivo, perfil string, k int, totalResultados int, resultadosJSON string, usuarioID *int64) error
	SearchLocalDocuments(ctx context.Context, query string, modalidade string, k int) ([]domain.KnowledgeDocument, error)
	SearchLocalDocumentCandidates(ctx context.Context, query string, modalidade string, k int) ([]domain.KnowledgeDocument, error)
	GetHistorico(ctx context.Context, limit int) ([]map[string]any, error)
	GetEstatisticas(ctx context.Context) (map[string]any, error)
	GetPopulares(ctx context.Context) ([]map[string]any, error)
}

type RelatoriosRepository interface {
	GetDashboardResumo(ctx context.Context) (*domain.RelatoriosDashboardResumo, error)
	GetPatologiasCobertura(ctx context.Context) ([]domain.RelatorioPatologiaItem, error)
	GetExerciciosSubutilizados(ctx context.Context, minRecomendacoes int) ([]domain.ExercicioSubutilizadoItem, error)
	GetRelatorioAprovacao(ctx context.Context, dias int) ([]domain.RelatorioAprovacaoItem, error)
}

type Teste3kmRepository interface {
	Create(ctx context.Context, t *domain.Teste3km) error
	ListByAlunoID(ctx context.Context, alunoID int64) ([]*domain.Teste3km, error)
	Delete(ctx context.Context, id int64, alunoID int64) error
}
