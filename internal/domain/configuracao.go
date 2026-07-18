package domain

import "time"

// Configuracao represents a system configuration key-value pair.
type Configuracao struct {
	Chave          string    `json:"chave"`
	Valor          string    `json:"valor"`
	Tipo           string    `json:"tipo"`
	Sensivel       bool      `json:"sensivel"`
	ValorMascarado bool      `json:"valor_mascarado,omitempty"`
	Descricao      string    `json:"descricao"`
	AtualizadoEm   time.Time `json:"atualizado_em"`
	AtualizadoPor  *int64    `json:"atualizado_por,omitempty"`
}

// DashboardStats contains the consolidated metrics for the admin panel.
type DashboardStats struct {
	Alunos struct {
		Total       int64 `json:"total"`
		Ativos      int64 `json:"ativos"`
		Inativos    int64 `json:"inativos"`
		SemAnamnese int64 `json:"sem_anamnese"`
	} `json:"alunos"`
	Anamneses struct {
		Total              int64 `json:"total"`
		PendentesAprovacao int64 `json:"pendentes_aprovacao"`
		AltoRisco          int64 `json:"alto_risco"`
	} `json:"anamneses"`
	PreRegistrosPendentes int64               `json:"pre_registros_pendentes"`
	AtividadesGarmin24h   int64               `json:"atividades_garmin_24h"`
	DistribuicaoPlanos    []PlanoDistribuicao `json:"distribuicao_planos"`
}

type PlanoDistribuicao struct {
	PlanoID          int64  `json:"plano_id"`
	Nome             string `json:"nome"`
	QuantidadeAlunos int64  `json:"quantidade_alunos"`
}
