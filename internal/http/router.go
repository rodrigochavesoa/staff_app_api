package http

import (
	"net/http"
	"os"
	"staff_app/internal/config"
	"staff_app/internal/services"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type RouterOption func(*routerOptions)

type routerOptions struct {
	embedProvider     services.EmbeddingProvider
	vectorStore       services.VectorStore
	trainingProviders []services.TrainingProvider
}

func WithRAGProviders(embed services.EmbeddingProvider, store services.VectorStore) RouterOption {
	return func(o *routerOptions) {
		o.embedProvider = embed
		o.vectorStore = store
	}
}

func WithTrainingProviders(providers ...services.TrainingProvider) RouterOption {
	return func(o *routerOptions) {
		o.trainingProviders = providers
	}
}

func NewRouter(cfg *config.Config, deps Deps, opts ...RouterOption) http.Handler {
	options := &routerOptions{}
	for _, opt := range opts {
		opt(options)
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)

	r.Use(LoggerMiddleware)
	r.Use(RecoveryMiddleware)
	r.Use(CorsMiddleware(cfg.CorsOrigins))

	healthHandler := NewHealthHandler(deps.Health)

	r.Get("/", healthHandler.Index)
	r.Get("/health", healthHandler.Health)
	r.Get("/ping", healthHandler.Ping)

	vdotHandler := NewVDOTHandler(deps.Teste3km)

	alunoHandler := NewAlunoHandler(deps.Alunos)

	fichaWebHandler := NewFichaWebHandler(deps.Fichas, deps.Alunos)

	feedbackHandler := NewFeedbackHandler(deps.Feedback, deps.Fichas)

	garminHandler := NewGarminHandler(cfg, deps.Garmin, deps.Alunos)

	authHandler := NewAuthHandler(deps.Users, deps.Alunos, cfg.SecretKey)
	planoHandler := NewPlanoHandler(deps.Planos)

	preCadastroHandler := NewPreCadastroHandler(
		deps.PreRegistro,
		deps.Alunos,
		deps.Planos,
		deps.Anamnese,
		deps.Configuracao,
	)
	anamneseHandler := NewAnamneseHandler(
		deps.Anamnese,
		deps.Alunos,
		deps.Users,
		deps.Configuracao,
	)
	exercicioHandler := NewExercicioHandler(deps.Exercicios)
	periodizacaoCorridaHandler := NewPeriodizacaoCorridaHandler(
		deps.PeriodizacaoCorrida,
		deps.Alunos,
		deps.Garmin,
		deps.Anamnese,
		cfg,
	)
	adminConfigHandler := NewAdminConfigHandler(
		deps.Configuracao,
		deps.Dashboard,
	)
	historicoHandler := NewHistoricoHandler(
		deps.Historico,
		deps.Alunos,
		deps.Fichas,
		deps.PeriodizacaoCorrida,
		cfg.SecretKey,
	)
	trainingChain := services.NewTrainingProviderChain(
		cfg.AITrainingMode,
		time.Duration(cfg.AITrainingTimeoutSec)*time.Second,
		buildTrainingProviders(cfg, options.trainingProviders),
		services.DefaultTrainingSafetyValidator{},
		services.DefaultTrainingQualityValidator{},
		services.NoopTrainingTelemetryRecorder{},
	)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/ping", healthHandler.Ping)

		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/register", authHandler.Register)

		r.Get("/planos", planoHandler.List)

		r.Get("/ficha/{hash}/json", fichaWebHandler.GetByHashJSON)
		r.Get("/ficha/{hash}/treino/{letra}", fichaWebHandler.GetFichaTreinoLetra)
		r.Get("/treinos/mes", historicoHandler.GetTreinosMes)
		r.Post("/feedback/{hash}", feedbackHandler.Submit)
		r.Get("/feedback/{hash}", feedbackHandler.Verify)

		r.Post("/pre-cadastro", preCadastroHandler.Create)
		r.Get("/anamnese/submit/{token}", anamneseHandler.GetMetadata)
		r.Post("/anamnese/submit/{token}", anamneseHandler.Submit)

		r.Get("/corrida/publica/{hash}", periodizacaoCorridaHandler.GetPublicPlano)
		r.Post("/corrida/publica/{hash}/concluir", periodizacaoCorridaHandler.ConcluirTreinoPublico)

		r.With(authHandler.OptionalAuthMiddleware).Post("/treinos/marcar", historicoHandler.MarkTreino)
		r.With(authHandler.OptionalAuthMiddleware).Post("/treinos/desmarcar", historicoHandler.UnmarkTreino)

		r.Group(func(r chi.Router) {
			r.Use(authHandler.AuthMiddleware)

			r.Get("/auth/me", authHandler.Me)
			r.Post("/auth/alterar-senha", authHandler.ChangePassword)

			r.Post("/criar-ficha", fichaWebHandler.Create)
			r.Get("/stats/{hash}", fichaWebHandler.GetStats)
			r.Post("/renovar/{hash}", fichaWebHandler.Renew)
			r.Post("/desativar/{hash}", fichaWebHandler.Deactivate)
			r.Get("/aluno/{aluno_id}/fichas", fichaWebHandler.ListByAluno)

			fichaTreinoHandler := NewFichaTreinoHandler(
				deps.FichaTreino,
				deps.Alunos,
				deps.Fichas,
				deps.Anamnese,
				deps.RAG,
				deps.EvidencePipeline,
				deps.EvidenceTelemetry,
				trainingChain,
			)
			r.Route("/fichas", func(r chi.Router) {
				r.Post("/manual/criar", fichaTreinoHandler.CreateManual)
				r.Get("/{id}", fichaTreinoHandler.GetByID)
				r.Put("/{id}/editar-manual", fichaTreinoHandler.EditManual)
				r.Delete("/{id}", fichaTreinoHandler.Delete)
				r.Post("/gerar-periodizada", fichaTreinoHandler.GerarPeriodizada)
			})
			r.Get("/metodos/{metodo}", fichaTreinoHandler.GetMetodoInfo)

			svedHandler := NewSVEDHandler(deps.SVED)
			r.Route("/sved", func(r chi.Router) {
				r.Post("/calcular", svedHandler.Calcular)
				r.Get("/historico/{aluno_id}/{exercicio_nome}", svedHandler.GetHistorico)
				r.Get("/sugestao/{ficha_id}/{exercicio_nome}", svedHandler.GetSugestao)
				r.Get("/sugestoes/{ficha_id}", svedHandler.GetSugestoesFicha)
				r.Get("/dashboard/{aluno_id}", svedHandler.GetDashboard)
			})

			r.Get("/historico/fichas/{id}/detalhes", historicoHandler.GetHistoricoDetalhes)

			r.Get("/feedback/pendentes", feedbackHandler.ListPending)
			r.Post("/feedback/notificacao/{notificacao_id}/marcar-lido", feedbackHandler.MarkRead)

			r.Route("/alunos", func(r chi.Router) {
				r.Get("/search", historicoHandler.SearchAlunos)
				r.Post("/", alunoHandler.Create)
				r.Get("/", alunoHandler.List)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", alunoHandler.GetByID)
					r.Put("/", alunoHandler.Update)
					r.Delete("/", alunoHandler.Delete)
					r.Post("/reativar", alunoHandler.Reactivate)

					r.Get("/frequencia", historicoHandler.GetFrequencia)
					r.Get("/treinos", historicoHandler.GetTreinos)

					r.Post("/vdot", vdotHandler.Create)
					r.Get("/vdot", vdotHandler.List)
					r.Delete("/vdot/{teste_id}", vdotHandler.Delete)

					r.Get("/corrida", periodizacaoCorridaHandler.ListByAluno)
					r.Get("/corrida/treinos-dia", periodizacaoCorridaHandler.GetCorridaTreinosDia)
					r.Get("/corrida/historico-stats", periodizacaoCorridaHandler.HistoricoStats)
				})
			})

			r.Route("/corrida", func(r chi.Router) {
				r.Post("/gerar", periodizacaoCorridaHandler.Gerar)
				r.Post("/gerar-blocos", periodizacaoCorridaHandler.GerarBlocos)
				r.Get("/{id}", periodizacaoCorridaHandler.GetByID)
				r.Delete("/{id}", periodizacaoCorridaHandler.Delete)
				r.Put("/{id}/editar-treino", periodizacaoCorridaHandler.EditarTreino)
				r.Put("/{id}/semana-atual", periodizacaoCorridaHandler.UpdateSemana)
				r.Post("/{id}/gerar-link", periodizacaoCorridaHandler.GerarLink)
				r.Post("/{id}/renovar-link", periodizacaoCorridaHandler.RenovarLink)
				r.Post("/{id}/desativar-link", periodizacaoCorridaHandler.DesativarLink)
				r.Post("/{id}/gerar-proxima-semana", periodizacaoCorridaHandler.GerarProximaSemana)
				r.Get("/{id}/semana/{semana}/dia/{dia}", periodizacaoCorridaHandler.GetTreinoDia)
				r.Put("/{id}/semana/{semana}/dia/{dia}/blocos", periodizacaoCorridaHandler.SaveBlocosDia)
			})

			r.Route("/admin/pre-cadastros", func(r chi.Router) {
				r.Get("/", preCadastroHandler.List)
				r.Get("/{id}", preCadastroHandler.GetByID)
				r.Post("/{id}/aprovar", preCadastroHandler.Approve)
				r.Post("/{id}/rejeitar", preCadastroHandler.Reject)
			})
			r.Route("/admin/anamnese", func(r chi.Router) {
				r.Get("/", anamneseHandler.List)
				r.Get("/pendentes", anamneseHandler.ListPending)
				r.Get("/{id}", anamneseHandler.GetByID)
				r.Post("/{id}/aprovar", anamneseHandler.Approve)
				r.Post("/{id}/rejeitar", anamneseHandler.Reject)
				r.Delete("/{id}", anamneseHandler.Delete)
			})
			r.Post("/admin/alunos/{id}/gerar-anamnese-link", anamneseHandler.GenerateLink)
			r.Post("/admin/alunos/{id}/anamnese/reenviar-email", anamneseHandler.ReenviarEmail)

			r.Get("/exercicios/biblioteca", exercicioHandler.ListBiblioteca)
			r.Get("/exercicios/grupos", exercicioHandler.Grupos)
			r.Get("/exercicios/tipos", exercicioHandler.Tipos)
			r.Route("/exercicios/personalizados", func(r chi.Router) {
				r.Post("/", exercicioHandler.Create)
				r.Get("/", exercicioHandler.List)
				r.Get("/estatisticas", exercicioHandler.Estatisticas)
				r.Route("/{codigo}", func(r chi.Router) {
					r.Get("/", exercicioHandler.GetByID)
					r.Put("/", exercicioHandler.Update)
					r.Post("/ativar", exercicioHandler.Activar)
					r.Post("/desativar", exercicioHandler.Desactivar)
					r.Delete("/", exercicioHandler.Delete)
				})
			})

			r.Route("/exercicios/sugestoes", func(r chi.Router) {
				r.Get("/", exercicioHandler.ListSugestoes)
				r.Post("/{id}/aprovar", exercicioHandler.ApproveSugestao)
				r.Post("/{id}/rejeitar", exercicioHandler.RejectSugestao)
			})

			r.Route("/admin", func(r chi.Router) {
				r.Use(AdminOnly)
				r.Get("/usuarios", authHandler.ListUsers)
				r.Post("/usuarios/{id}/aprovar", authHandler.ApproveUser)
				r.Post("/usuarios/{id}/rejeitar", authHandler.RejectUser)
				r.Post("/usuarios/{id}/toggle", authHandler.ToggleUser)

				r.Post("/planos", planoHandler.Create)
				r.Put("/planos/{id}", planoHandler.Update)
				r.Delete("/planos/{id}", planoHandler.Delete)

				r.Get("/configuracoes", adminConfigHandler.List)
				r.Put("/configuracoes", adminConfigHandler.Update)
				r.Post("/configuracoes/testar-smtp", adminConfigHandler.TestSMTP)
				r.Get("/dashboard/stats", adminConfigHandler.DashboardStats)

				relatoriosHandler := NewRelatoriosHandler(deps.Relatorios)
				r.Route("/relatorios", func(r chi.Router) {
					r.Get("/dashboard", relatoriosHandler.GetDashboardResumo)
					r.Get("/patologias", relatoriosHandler.GetPatologias)
					r.Get("/subutilizados", relatoriosHandler.GetSubutilizados)
					r.Get("/aprovacao", relatoriosHandler.GetAprovacao)
				})

				embedProvider := options.embedProvider
				vectorStore := options.vectorStore

				if embedProvider == nil {
					openAIKey := os.Getenv("OPENAI_API_KEY")
					if openAIKey != "" {
						embedProvider = services.NewOpenAIEmbeddingProvider(openAIKey)
					}
				}
				if vectorStore == nil {
					chromaURL := os.Getenv("CHROMA_URL")
					if chromaURL != "" {
						vectorStore = services.NewChromaVectorStore(chromaURL, os.Getenv("CHROMA_COLLECTION"))
					}
				}

				ragHandler := NewRAGHandler(deps.RAG, embedProvider, vectorStore)
				r.Route("/consulta-base", func(r chi.Router) {
					r.Post("/", ragHandler.Search)
					r.Get("/historico", ragHandler.GetHistorico)
					r.Get("/estatisticas", ragHandler.GetEstatisticas)
					r.Get("/populares", ragHandler.GetPopulares)
				})
			})
		})
	})

	r.Route("/api/garmin", func(r chi.Router) {
		r.Use(authHandler.AuthMiddleware)
		r.Post("/upload", garminHandler.Upload)
		r.Get("/activity/{atividade_id}", garminHandler.Activity)
		r.Delete("/activity/{atividade_id}/delete", garminHandler.DeleteActivity)
		r.Get("/aluno/{aluno_id}/activities", garminHandler.ListAlunoActivities)
		r.Get("/aluno/{aluno_id}/stats", garminHandler.AlunoStats)
		r.Get("/charts/distance/{aluno_id}", garminHandler.ChartDistance)
		r.Get("/charts/hr-zones/{aluno_id}", garminHandler.ChartHRZones)
		r.Get("/charts/activity-types/{aluno_id}", garminHandler.ChartActivityTypes)
		r.Get("/charts/velocity-scatter/{aluno_id}", garminHandler.ChartVelocityScatter)
		r.Get("/charts/dashboard/{aluno_id}", garminHandler.ChartDashboard)
		r.Get("/charts/hr-series/{aluno_id}", garminHandler.ChartHRSeries)
		r.Get("/charts/calories/{aluno_id}", garminHandler.ChartCalories)
	})

	return r
}

func buildTrainingProviders(cfg *config.Config, override []services.TrainingProvider) []services.TrainingProvider {
	if len(override) > 0 {
		return override
	}
	timeout := time.Duration(cfg.AITrainingTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	providers := make([]services.TrainingProvider, 0, len(cfg.AITrainingProviders))
	for _, name := range cfg.AITrainingProviders {
		switch name {
		case "gemini":
			if cfg.GeminiAPIKey != "" {
				providers = append(providers, services.NewGeminiTrainingProvider(cfg.GeminiAPIKey, cfg.GeminiModel, timeout))
			}
		case "openai":
			openAIKey := os.Getenv("OPENAI_API_KEY")
			if openAIKey != "" {
				providers = append(providers, services.NewOpenAITrainingProvider(openAIKey, cfg.OpenAITrainingModel, timeout))
			}
		case "claude", "anthropic":
			if cfg.ClaudeAPIKey != "" {
				providers = append(providers, services.NewClaudeTrainingProvider(cfg.ClaudeAPIKey, cfg.ClaudeModel, timeout))
			}
		case "local":
			providers = append(providers, services.LocalTrainingProvider{})
		}
	}
	return providers
}
