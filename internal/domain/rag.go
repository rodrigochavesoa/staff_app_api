package domain

import "time"

type KnowledgeDocument struct {
	Rank       int      `json:"rank"`
	Fonte      string   `json:"fonte"`
	Titulo     string   `json:"titulo,omitempty"`
	Conteudo   string   `json:"conteudo"`
	Tags       []string `json:"tags"`
	Modalidade string   `json:"modalidade,omitempty"`
	Relevancia float64  `json:"relevancia"`
}

type ConsultaBaseConhecimento struct {
	ID               int64      `json:"id"`
	QueryOriginal    string     `json:"query_original"`
	QueryNormalizada string     `json:"query_normalizada"`
	Modalidade       *string    `json:"modalidade"`
	Objetivo         *string    `json:"objetivo"`
	Perfil           *string    `json:"perfil"`
	K                int        `json:"k"`
	TotalResultados  int        `json:"total_resultados"`
	Hits             int        `json:"hits"`
	ResultadosJSON   string     `json:"resultados_json"`
	UsuarioID        *int64     `json:"usuario_id"`
	CriadoEm         time.Time  `json:"criado_em"`
	UltimaUtilizacao time.Time  `json:"ultima_utilizacao"`
}

type BaseConhecimentoDocumento struct {
	ID         int64     `json:"id"`
	Fonte      string    `json:"fonte"`
	Titulo     *string   `json:"titulo"`
	Conteudo   string    `json:"conteudo"`
	Tags       *string   `json:"tags"` // comma-separated tags
	Modalidade *string   `json:"modalidade"`
	Ativo      int       `json:"ativo"`
	CriadoEm   time.Time `json:"criado_em"`
}
