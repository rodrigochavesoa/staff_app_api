package domain

type RelatorioPatologiaItem struct {
	PatologiaAlvo        string  `json:"patologia_alvo"`
	TotalExercicios      int     `json:"total_exercicios"`
	TotalUtilizacoes     int     `json:"total_utilizacoes"`
	MediaUsoPorExercicio float64 `json:"media_uso_por_exercicio"`
}

type ExercicioSubutilizadoItem struct {
	Codigo           int     `json:"codigo"`
	Nome             string  `json:"nome"`
	GrupoMuscular    string  `json:"grupo_muscular"`
	PatologiaAlvo    string  `json:"patologia_alvo"`
	VezesRecomendado int     `json:"vezes_recomendado"`
	VezesUsado       int     `json:"vezes_usado"`
	TaxaUsoPct       float64 `json:"taxa_uso_pct"`
}

type RelatorioAprovacaoItem struct {
	CodigoExercicio  int     `json:"codigo_exercicio"`
	NomeExercicio    string  `json:"nome_exercicio"`
	TotalSugestoes   int     `json:"total_sugestoes"`
	Aprovadas        int     `json:"aprovadas"`
	Rejeitadas       int     `json:"rejeitadas"`
	TaxaAprovacaoPct float64 `json:"taxa_aprovacao_pct"`
}

type RelatoriosDashboardResumo struct {
	TotalExerciciosAtivos int     `json:"total_exercicios_ativos"`
	TaxaUsoGlobalPct      float64 `json:"taxa_uso_global_pct"`
	TotalUtilizacoes      int     `json:"total_utilizacoes"`
	TotalRecomendacoes    int     `json:"total_recomendacoes"`
	ExerciciosNuncaUsados int     `json:"exercicios_nunca_usados"`
	SugestoesPendentes    int     `json:"sugestoes_pendentes"`
	TaxaAprovacao30dPct   float64 `json:"taxa_aprovacao_30d_pct"`
	SugestoesUltimos30d   int     `json:"sugestoes_ultimos_30d"`
	DataAtualizacao       string  `json:"data_atualizacao"`
}
