package domain

import "time"

type PreRegistro struct {
	ID             int64      `json:"id"`
	Nome           string     `json:"nome"`
	Email          string     `json:"email"`
	Telefone       string     `json:"telefone"`
	DataNascimento string     `json:"data_nascimento"`
	Genero         string     `json:"genero"`
	PaymentRef     *string    `json:"payment_ref"`
	PlanoID        *int64     `json:"plano_id"`
	PlanoValor     *float64   `json:"plano_valor"`
	IpOrigem       string     `json:"ip_origem"`
	UserAgent      string     `json:"user_agent"`
	ExpiraEm       time.Time  `json:"expira_em"`
	CriadoEm       time.Time  `json:"criado_em"`
	Usado          bool       `json:"usado"`
	Status         string     `json:"status"` // 'aguardando_aprovacao', 'aprovado', 'rejeitado'
	AprovadoPor    *int64     `json:"aprovado_por"`
	AprovadoEm     *time.Time `json:"aprovado_em"`
	AlunoIDCriado  *int64     `json:"aluno_id_criado"`
	MotivoRejeicao *string    `json:"motivo_rejeicao"`
}

type PreRegistroAudit struct {
	ID            int64     `json:"id"`
	PreRegistroID int64     `json:"pre_registro_id"`
	Evento        string    `json:"evento"` // 'CRIADO', 'APROVADO', 'REJEITADO', 'EXPIRADO'
	UsuarioID     *int64    `json:"usuario_id"`
	Detalhes      string    `json:"detalhes"`
	IpOrigem      string    `json:"ip_origem"`
	UserAgent     string    `json:"user_agent"`
	CriadoEm      time.Time `json:"criado_em"`
}
