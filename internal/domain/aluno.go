package domain

import "time"

type Aluno struct {
	ID                   int64      `json:"id"`
	Nome                 string     `json:"nome"`
	Idade                int        `json:"idade"`
	Sexo                 string     `json:"sexo"`
	Email                string     `json:"email"`
	Telefone             string     `json:"telefone"`
	Objetivo             string     `json:"objetivo"`
	ExclusoesPermanentes string     `json:"exclusoes_permanentes"`
	Turma                string     `json:"turma"`
	UsuarioID            *int64     `json:"usuario_id"`
	PlanoID              *int64     `json:"plano_id"`
	PlanoValor           *float64   `json:"plano_valor"`
	PlanoPago            bool       `json:"plano_pago"`
	PlanoAtivo           bool       `json:"plano_ativo"`
	PlanoInicio          *string    `json:"plano_inicio"`
	PlanoFim             *string    `json:"plano_fim"`
	CadastroAprovado     bool       `json:"cadastro_aprovado"`
	CadastroAprovadoPor  *int64     `json:"cadastro_aprovado_por"`
	CadastroAprovadoEm   *time.Time `json:"cadastro_aprovado_em"`
	PreRegistroID        *int64     `json:"pre_registro_id"`
	Ativo                bool       `json:"ativo"`
}
