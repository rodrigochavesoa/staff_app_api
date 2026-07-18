package domain

import "time"

// TreinoRealizado represents a completed workout session, typically for weight training/musculação.
type TreinoRealizado struct {
	ID          int64     `json:"id"`
	FichaID     int64     `json:"ficha_id"`
	AlunoID     *int64    `json:"aluno_id"`
	HashFicha   *string   `json:"hash_ficha"`
	DataTreino  string    `json:"data_treino"` // YYYY-MM-DD
	TipoTreino  *string   `json:"tipo_treino"` // e.g. "A", "B", "C", "D"
	TipoFicha   string    `json:"tipo_ficha"`   // e.g. "musculacao"
	Observacao  *string   `json:"observacao"`
	CriadoEm    time.Time `json:"criado_em"`
}

// HistoricoFicha represents a snapshot of an archived training sheet or running periodization.
type HistoricoFicha struct {
	ID                       int64     `json:"id"`
	AlunoID                  int64     `json:"aluno_id"`
	TipoFicha                string    `json:"tipo_ficha"` // 'musculacao' or 'corrida'
	Versao                   int       `json:"versao"`
	Status                   string    `json:"status"`
	DataCriacao              *string   `json:"data_criacao"`
	DataArquivamento         time.Time `json:"data_arquivamento"`
	DataInicioUso            *string   `json:"data_inicio_uso,omitempty"`
	FichaOrigemID            *int64    `json:"ficha_origem_id,omitempty"`
	FichaOrigemTabela        *string   `json:"ficha_origem_tabela,omitempty"`
	FichaJSON                *string   `json:"ficha_json,omitempty"`
	PlanoJSON                *string   `json:"plano_json,omitempty"`
	VDOT                     *float64  `json:"vdot,omitempty"`
	PaceBase                 *string   `json:"pace_base,omitempty"`
	DistanciaProva           *float64  `json:"distancia_prova,omitempty"`
	Nivel                    *string   `json:"nivel,omitempty"`
	DuracaoSemanas           *int      `json:"duracao_semanas,omitempty"`
	Modo                     *string   `json:"modo,omitempty"`
	SemanasCompletadas       int       `json:"semanas_completadas"`
	TaxaCompletude           float64   `json:"taxa_completude"`
	FeedbackDificuldadeMedio float64   `json:"feedback_dificuldade_medio"`
	DoresReportadas          *string   `json:"dores_reportadas,omitempty"`
	TotalTreinosPlanejados   int       `json:"total_treinos_planejados"`
	TotalTreinosRealizados   int       `json:"total_treinos_realizados"`
	DiasUso                  int       `json:"dias_uso"`
	Objetivo                 *string   `json:"objetivo,omitempty"`
	Modalidade               *string   `json:"modalidade,omitempty"`
	FrequenciaSemanal        *int      `json:"frequencia_semanal,omitempty"`
	ObservacoesGerais        *string   `json:"observacoes_gerais,omitempty"`
	CoachNotes               *string   `json:"coach_notes,omitempty"`
}

// AlunoSearchResponse is a lightweight payload for student search/autocomplete
type AlunoSearchResponse struct {
	ID         int64  `json:"id"`
	Nome       string `json:"nome"`
	Email      string `json:"email"`
	Turma      string `json:"turma"`
	Ativo      bool   `json:"ativo"`
	PlanoAtivo bool   `json:"plano_ativo"`
	PlanoNome  string `json:"plano_nome"`
}

// FrequenciaMensalResponse compiles monthly frequency data and calendar details
type FrequenciaMensalResponse struct {
	AlunoID             int64                 `json:"aluno_id"`
	Mes                 int                   `json:"mes"`
	Ano                 int                   `json:"ano"`
	EstatisticasMensais FrequenciaEstatisticas `json:"estatisticas_mensais"`
	DiasFrequencia      []DiaFrequencia       `json:"dias_frequencia"`
}

type FrequenciaEstatisticas struct {
	TotalRealizados int     `json:"total_realizados"`
	TotalPlanejados int     `json:"total_planejados"`
	TaxaCompletude  float64 `json:"taxa_completude"`
	DiasComDor      int     `json:"dias_com_dor"`
}

type DiaFrequencia struct {
	Data       string `json:"data"` // YYYY-MM-DD
	Realizado  bool   `json:"realizado"`
	TipoTreino string `json:"tipo_treino,omitempty"`
	TipoFicha  string `json:"tipo_ficha,omitempty"`
	Observacao string `json:"observacao,omitempty"`
}
