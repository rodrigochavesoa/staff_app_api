package domain

type Plano struct {
	ID           int64   `json:"id"`
	Nome         string  `json:"nome"`
	PrecoDefault float64 `json:"preco_default"`
	Descricao    string  `json:"descricao"`
	Ativo        bool    `json:"ativo"`
}
