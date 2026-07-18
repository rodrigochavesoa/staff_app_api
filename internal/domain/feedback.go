package domain

import "time"

type FeedbackFicha struct {
	ID            int64     `json:"id"`
	HashFicha     string    `json:"hash_ficha"`
	AlunoID       int64     `json:"aluno_id"`
	Rating        int       `json:"rating"`
	Comentario    *string   `json:"comentario"`
	CreatedAt     time.Time `json:"created_at"`
	AlunoNome     string    `json:"aluno_nome,omitempty"`
	NotificacaoID int64     `json:"notificacao_id,omitempty"`
}

type FeedbackNotificacao struct {
	ID         int64      `json:"id"`
	FeedbackID int64      `json:"feedback_id"`
	UserID     *int64     `json:"user_id"`
	Lido       bool       `json:"lido"`
	LidoEm     *time.Time `json:"lido_em"`
	CriadoEm   time.Time  `json:"criado_em"`
}
