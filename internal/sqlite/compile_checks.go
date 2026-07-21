package sqlite

import "staff_app/internal/repositories"

// Verificações em tempo de compilação: tipos sqlite implementam repositories.* usados por HTTP/cmd.
var (
	_ repositories.DatabaseHealth              = (*DB)(nil)
	_ repositories.UserRepository              = (*UserRepository)(nil)
	_ repositories.AlunoRepository             = (*AlunoRepository)(nil)
	_ repositories.FichaRepository             = (*FichaWebRepository)(nil)
	_ repositories.FeedbackRepository          = (*FeedbackRepository)(nil)
	_ repositories.FichaTreinoRepository       = (*FichaTreinoRepository)(nil)
	_ repositories.PeriodizacaoCorridaRepository = (*PeriodizacaoCorridaRepository)(nil)
	_ repositories.ConfiguracaoRepository      = (*ConfiguracaoRepository)(nil)
	_ repositories.DashboardRepository         = (*DashboardRepository)(nil)
	_ repositories.SVEDRepository              = (*SVEDRepository)(nil)
	_ repositories.AnamneseRepository          = (*AnamneseRepository)(nil)
	_ repositories.PreRegistroRepository       = (*PreRegistroRepository)(nil)
	_ repositories.PlanoRepository             = (*PlanoRepository)(nil)
	_ repositories.HistoricoRepository         = (*HistoricoRepository)(nil)
	_ repositories.ExercicioRepository         = (*ExercicioRepository)(nil)
	_ repositories.GarminRepository            = (*GarminRepository)(nil)
	_ repositories.RAGRepository               = (*RAGRepository)(nil)
	_ repositories.RelatoriosRepository        = (*RelatoriosRepository)(nil)
	_ repositories.Teste3kmRepository          = (*Teste3kmRepository)(nil)
)
