package domain

import "time"

type FichaTreinoWeb struct {
	ID                int64      `json:"id"`
	Aluno             string     `json:"aluno"`
	Idade             int        `json:"idade"`
	Sexo              string     `json:"sexo"`
	Objetivo          string     `json:"objetivo"`
	Modalidade        string     `json:"modalidade"`
	Nivel             string     `json:"nivel"`
	FrequenciaSemanal int        `json:"frequencia_semanal"`
	DuracaoTreino     int        `json:"duracao_treino"`
	Restricoes        string     `json:"restricoes"`
	Feedback          string     `json:"feedback"`
	Turma             string     `json:"turma"`
	ListaExercicios   string     `json:"lista_exercicios"`
	DataCriacao       time.Time  `json:"data_criacao"`
	FichaJSON         string     `json:"ficha_json"`
	TipoFicha         string     `json:"tipo_ficha"`
	NumTreinos        int        `json:"num_treinos"`
	Versao            int        `json:"versao"`
	FichaAnteriorID   *int64     `json:"ficha_anterior_id"`
	DataArquivamento  *time.Time `json:"data_arquivamento"`
	IesScore          float64    `json:"ies_score"`
	VolumeSved        int        `json:"volume_sved"`
	Densidade         float64    `json:"densidade"`
	TutTotal          int        `json:"tut_total"`
	Series            string     `json:"series"`
	RIR               int        `json:"rir"`
	Cadencia          string     `json:"cadencia"`
	RestSeconds       int        `json:"rest_seconds"`
}

// FichaTreinoListItem is a summary row for portal/staff list endpoints (no ficha_json).
type FichaTreinoListItem struct {
	ID          int64     `json:"id"`
	TipoFicha   string    `json:"tipo_ficha"`
	Versao      int       `json:"versao"`
	NumTreinos  int       `json:"num_treinos"`
	DataCriacao time.Time `json:"data_criacao"`
	Modalidade  string    `json:"modalidade"`
	Objetivo    string    `json:"objetivo"`
}

type FichaWeb struct {
	ID           int64      `json:"id"`
	Hash         string     `json:"hash"`
	FichaID      int64      `json:"ficha_id"`
	AlunoID      int64      `json:"aluno_id"`
	UserID       *int64     `json:"user_id"`
	ConteudoJSON string     `json:"conteudo_json"`
	CriadoEm     time.Time  `json:"criado_em"`
	ExpiraEm     time.Time  `json:"expira_em"`
	Acessos      int        `json:"acessos"`
	UltimoAcesso *time.Time `json:"ultimo_acesso"`
	Ativo        bool       `json:"ativo"`
	RenovadoDe   *int64     `json:"renovado_de"`
}

type FichaWebAcesso struct {
	ID         int64     `json:"id"`
	Hash       string    `json:"hash"`
	DataAcesso time.Time `json:"data_acesso"`
	UserAgent  string    `json:"user_agent"`
	IPAddress  string    `json:"ip_address"`
}

type FichaWebStats struct {
	Hash             string            `json:"hash"`
	CriadoEm         time.Time         `json:"criado_em"`
	ExpiraEm         time.Time         `json:"expira_em"`
	Acessos          int               `json:"acessos"`
	Ativo            bool              `json:"ativo"`
	HistoricoAcessos []*FichaWebAcesso `json:"historico_acessos"`
}
