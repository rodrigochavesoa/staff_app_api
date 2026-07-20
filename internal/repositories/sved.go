package repositories

import "context"

// SVEDSheet is a ficha row used to rebuild SVED history from ficha_json.
type SVEDSheet struct {
	ID          int64
	DataCriacao string
	FichaJSON   string
}

// SVEDFichaDetail is the current ficha payload for sugestões por ficha.
type SVEDFichaDetail struct {
	AlunoNome string
	Titulo    string
	FichaJSON string
}

// SVEDAggregatedStats are AVG metrics across a student's sheets.
type SVEDAggregatedStats struct {
	IesMedio       float64
	DensidadeMedia float64
	VolumeEfetivo  float64
}

// SVEDDashboardSheet is one sheet row for the SVED dashboard.
type SVEDDashboardSheet struct {
	ID          int64
	Turma       string
	DataCriacao string
	IesScore    float64
	TutTotal    int
	Densidade   float64
	VolumeSved  int
	FichaJSON   string
}

// SVEDRepository isolates SVED SQL from the HTTP handler.
type SVEDRepository interface {
	GetAlunoNomeByID(ctx context.Context, alunoID int64) (string, error)
	ListFichaSheetsByAlunoNome(ctx context.Context, nome string, limit int) ([]SVEDSheet, error)
	GetFichaAlunoByID(ctx context.Context, fichaID int64) (string, error)
	GetFichaDetailByID(ctx context.Context, fichaID int64) (*SVEDFichaDetail, error)
	GetAlunoIDByNomeLatest(ctx context.Context, nome string) (int64, bool, error)
	GetAggregatedStatsByAluno(ctx context.Context, nome string) (*SVEDAggregatedStats, error)
	ListDashboardSheetsByAluno(ctx context.Context, nome string, limit int) ([]SVEDDashboardSheet, error)
}
