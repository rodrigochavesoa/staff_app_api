package http

import (
	"staff_app/internal/repositories"
	"staff_app/internal/services"
	"staff_app/internal/sqlite"
)

type Deps struct {
	Health repositories.DatabaseHealth

	Users               repositories.UserRepository
	Alunos              repositories.AlunoRepository
	Fichas              repositories.FichaRepository
	Feedback            repositories.FeedbackRepository
	FichaTreino         repositories.FichaTreinoRepository
	Historico           repositories.HistoricoRepository
	PreRegistro         repositories.PreRegistroRepository
	Planos              repositories.PlanoRepository
	Anamnese            repositories.AnamneseRepository
	Exercicios          repositories.ExercicioRepository
	PeriodizacaoCorrida repositories.PeriodizacaoCorridaRepository
	Garmin              repositories.GarminRepository
	Configuracao        repositories.ConfiguracaoRepository
	Dashboard           repositories.DashboardRepository
	RAG                 repositories.RAGRepository
	Relatorios          repositories.RelatoriosRepository
	Teste3km            repositories.Teste3kmRepository
	SVED                repositories.SVEDRepository

	EvidencePipeline  *services.EvidencePipeline
	EvidenceTelemetry services.EvidencePipelineTelemetryRecorder

	// Shutdown fecha o pool SQLite no encerramento gracioso.
	Shutdown func() error
}

func NewSQLiteDeps(db *sqlite.DB) Deps {
	users := sqlite.NewUserRepository(db)
	alunos := sqlite.NewAlunoRepository(db)
	fichas := sqlite.NewFichaWebRepository(db)
	planos := sqlite.NewPlanoRepository(db)
	anamnese := sqlite.NewAnamneseRepository(db)
	rag := sqlite.NewRAGRepository(db)

	return Deps{
		Health:              db,
		Users:               users,
		Alunos:              alunos,
		Fichas:              fichas,
		Feedback:            sqlite.NewFeedbackRepository(db),
		FichaTreino:         sqlite.NewFichaTreinoRepository(db),
		Historico:           sqlite.NewHistoricoRepository(db),
		PreRegistro:         sqlite.NewPreRegistroRepository(db),
		Planos:              planos,
		Anamnese:            anamnese,
		Exercicios:          sqlite.NewExercicioRepository(db),
		PeriodizacaoCorrida: sqlite.NewPeriodizacaoCorridaRepository(db),
		Garmin:              sqlite.NewGarminRepository(db),
		Configuracao:        sqlite.NewConfiguracaoRepository(db),
		Dashboard:           sqlite.NewDashboardRepository(db),
		RAG:                 rag,
		Relatorios:          sqlite.NewRelatoriosRepository(db),
		Teste3km:            sqlite.NewTeste3kmRepository(db),
		SVED:                sqlite.NewSVEDRepository(db),

		EvidencePipeline:  services.NewEvidencePipeline(db, anamnese, rag),
		EvidenceTelemetry: sqlite.NewEvidencePipelineTelemetryRecorder(db),
		Shutdown:          db.Close,
	}
}
