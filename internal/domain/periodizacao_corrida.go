package domain

import "time"

// PeriodizacaoCorrida represents a running periodization in the database.
type PeriodizacaoCorrida struct {
	ID                     int64   `json:"id"`
	AlunoID                int64   `json:"aluno_id"`
	DataInicio             string  `json:"data_inicio"`
	DuracaoSemanas         int     `json:"duracao_semanas"`
	Modo                   string  `json:"modo"`
	SemanaAtual            int     `json:"semana_atual"`
	Status                 string  `json:"status"`
	DistanciaProva         float64 `json:"distancia_prova"`
	Nivel                  string  `json:"nivel"`
	VDOT                   float64 `json:"vdot"`
	PaceBase               int     `json:"pace_base"` // in seconds/km
	VolumeSemanal          float64 `json:"volume_semanal"`
	DiasDisponiveis        int     `json:"dias_disponiveis"`
	PlanoJSON              string  `json:"plano_json"`
	ModoGeracao            string  `json:"modo_geracao"`
	DataUltimaGeracao      string  `json:"data_ultima_geracao"`
	DiasSemanaSelecionados string  `json:"dias_semana_selecionados"`
	Versao                 int     `json:"versao"`
	FichaAnteriorID        *int64  `json:"ficha_anterior_id,omitempty"`
	DataArquivamento       *string `json:"data_arquivamento,omitempty"`
	AlunoNome              string  `json:"aluno_nome,omitempty"`  // populated during joins
	AlunoIdade             int     `json:"aluno_idade,omitempty"` // populated during joins
}

// PeriodizacaoCorridaWeb represents a public shared link for a periodization.
type PeriodizacaoCorridaWeb struct {
	ID             int64      `json:"id"`
	Hash           string     `json:"hash"`
	PeriodizacaoID int64      `json:"periodizacao_id"`
	AlunoID        int64      `json:"aluno_id"`
	UserID         *int64     `json:"user_id,omitempty"`
	CriadoEm       time.Time  `json:"criado_em"`
	ExpiraEm       time.Time  `json:"expira_em"`
	Acessos        int        `json:"acessos"`
	UltimoAcesso   *time.Time `json:"ultimo_acesso,omitempty"`
	Ativo          int        `json:"ativo"`
}

// ZoneDetails stores target pace and descriptive label for a training zone.
type ZoneDetails struct {
	PaceAlvo  string `json:"pace_alvo"`
	Descricao string `json:"descricao"`
}

// BlocoCorrida is an atomic or repeater block inside a dynamic running workout.
type BlocoCorrida struct {
	Type        string         `json:"type"`                   // "atomic" | "repeater"
	Intensity   string         `json:"intensity,omitempty"`    // E|M|T|I|R|Rest
	DurationMin float64        `json:"duration_min,omitempty"`
	DistanceKM  float64        `json:"distance_km,omitempty"`
	PaceMinKM   string         `json:"pace_min_km,omitempty"`
	Description string         `json:"description,omitempty"`
	Notas       string         `json:"notas,omitempty"`
	Repeat      int            `json:"repeat,omitempty"`
	Content     []BlocoCorrida `json:"content,omitempty"`
}

// TreinoJSON represents a single training session.
type TreinoJSON struct {
	Dia            int           `json:"dia"`       // 1=Mon, 7=Sun
	Tipo           string        `json:"tipo"`      // 'Corrida Fácil', 'Tempo Run', etc.
	Distancia      float64       `json:"distancia"` // in km
	Zona           string        `json:"zona"`      // 'E', 'M', 'T', 'I', 'R', 'RACE'
	PaceAlvo       string        `json:"pace_alvo"` // MM:SS
	Descricao      string        `json:"descricao"`
	Concluido      bool          `json:"concluido"`
	Nome           string        `json:"nome,omitempty"`
	TemplateID     string        `json:"template_id,omitempty"`
	ZonaPrincipal  string        `json:"zona_principal,omitempty"`
	DuracaoMinutos float64       `json:"duracao_minutos,omitempty"`
	Blocos         []BlocoCorrida `json:"blocos,omitempty"`
}

// SemanaJSON represents a training week.
type SemanaJSON struct {
	Numero                 int                    `json:"numero"`
	Fase                   string                 `json:"fase"`         // 'Base', 'Build', 'Intensidade', 'Taper'
	VolumeTotal            float64                `json:"volume_total"` // total km for the week
	Treinos                []TreinoJSON           `json:"treinos"`
	TreinamentoSuplementar map[string]interface{} `json:"treinamento_suplementar"`
}

// PlanoDetalhado represents the deserialized content of plano_json.
type PlanoDetalhado struct {
	VDOT                   float64                `json:"vdot"`
	DistanciaProva         float64                `json:"distancia_prova"`
	DuracaoSemanas         int                    `json:"duracao_semanas"`
	DiasSemanaSelecionados []int                  `json:"dias_semana_selecionados"`
	Zonas                  map[string]ZoneDetails `json:"zonas"`
	Semanas                []SemanaJSON           `json:"semanas"`
	Tipo                   string                 `json:"tipo,omitempty"`
	ModoGeracao            string                 `json:"_modo_geracao,omitempty"`
	SemanasGeradas         int                    `json:"_semanas_geradas,omitempty"`
}

type PeriodizacaoRequest struct {
	AlunoID        int64   `json:"aluno_id"`
	DistanciaProva string  `json:"distancia_prova"` // "5K", "10K", "21K", "42K"
	DataProva      string  `json:"data_prova"`      // YYYY-MM-DD
	DataInicio     string  `json:"data_inicio"`     // YYYY-MM-DD, optional
	Nivel          string  `json:"nivel"`           // "iniciante", "intermediario", "avancado", "elite"
	PaceBase       string  `json:"pace_base"`       // MM:SS
	VolumeSemanal  float64 `json:"volume_semanal"`
	DiasSemana     []int   `json:"dias_semana"`
}
