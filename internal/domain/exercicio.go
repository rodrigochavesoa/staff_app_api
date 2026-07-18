package domain

import "time"

// ExercicioReabilitacao representa um exercício cadastrado (Terapêutico ou Normal)
type ExercicioReabilitacao struct {
	Codigo               int        `json:"codigo"`
	Nome                 string     `json:"nome"`
	Categoria            string     `json:"categoria"` // "terapeutico" ou "normal"
	DescricaoTerapeutica string     `json:"descricao_terapeutica"`
	Descricao            string     `json:"descricao"` // Aliasing Go v1
	Indicacoes           string     `json:"indicacoes"`
	Contraindicacoes     string     `json:"contraindicacoes"`
	RestricoesSugeridas  string     `json:"restricoes_sugeridas"` // Aliasing Go v1
	GrupoMuscular        string     `json:"grupo_muscular"`
	MusculoFoco          string     `json:"musculo_foco"`
	TipoExercicio        string     `json:"tipo_exercicio"`
	Intensidade          string     `json:"intensidade"`
	NivelPrioridade      int        `json:"nivel_prioridade"`
	FonteCientifica      string     `json:"fonte_cientifica"`
	Url                  string     `json:"url"`
	UrlSecundaria        string     `json:"url_secundaria"`
	VideoUrl             string     `json:"video_url"` // Aliasing Go v1
	CriadoPor            string     `json:"criado_por"`
	CriadoEm             time.Time  `json:"criado_em"`
	Status               string     `json:"status"` // "ativo" ou "inativo"
	NotasProfissional    string     `json:"notas_profissional"`
	AtualizadoEm         *time.Time `json:"atualizado_em,omitempty"`
	AtualizadoPor        string     `json:"atualizado_por,omitempty"`
}

// SugestaoExercicioRehab representa uma sugestão de IA/RAG pendente de moderação
type SugestaoExercicioRehab struct {
	ID                          int        `json:"id"`
	NomeExercicio               string     `json:"nome_exercicio"`
	TipoExercicio               string     `json:"tipo_exercicio"`
	NivelPrioridade             int        `json:"nivel_prioridade"`
	FrequenciaSugestao          int        `json:"frequencia_sugestao"`
	ExercicioSimilarNome        string     `json:"exercicio_similar_nome"`
	RagFonte                    string     `json:"rag_fonte"`
	JustificativaClinica        string     `json:"justificativa_clinica"`
	Status                      string     `json:"status"` // "pendente", "aprovado", "rejeitado"
	AprovadoEm                  *time.Time `json:"aprovado_em,omitempty"`
	AprovadoPor                 string     `json:"aprovado_por,omitempty"`
	ExercicioReabilitacaoCodigo *int       `json:"exercicio_reabilitacao_codigo,omitempty"`
	NotasProfissional           string     `json:"notas_profissional"`
	MotivoRejeicao              string     `json:"motivo_rejeicao,omitempty"`
	DataSugestao                time.Time  `json:"data_sugestao"`
}
