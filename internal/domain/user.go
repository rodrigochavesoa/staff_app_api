package domain

import "time"

type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	NomeCompleto string     `json:"nome_completo"`
	IsAdmin      bool       `json:"is_admin"`
	CriadoEm     time.Time  `json:"criado_em"`
	UltimoLogin  *time.Time `json:"ultimo_login"`
	Ativo        bool       `json:"ativo"`
	Aprovado     bool       `json:"aprovado"`
	FotoPerfil   *string    `json:"foto_perfil"`
}
