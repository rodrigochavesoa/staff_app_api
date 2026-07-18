package domain

import "time"

// Teste3km maps to the sqlite table `teste_3km` and represents a 3k test result.
type Teste3km struct {
	ID              int64     `json:"id"`
	AlunoID         int64     `json:"aluno_id"`
	DataTeste       time.Time `json:"data_teste"`
	TempoSegundos   int       `json:"tempo_segundos"`
	PSE             *int      `json:"pse,omitempty"` // Percepção Subjetiva de Esforço
	Fonte           string    `json:"fonte"`         // ex: "garmin", "manual"
	VDOT            float64   `json:"vdot"`
	FTPPaceSegundos int       `json:"ftp_pace_segundos"`
	PaceZ1Min       int       `json:"pace_z1_min"`
	PaceZ1Max       int       `json:"pace_z1_max"`
	PaceZ2Min       int       `json:"pace_z2_min"`
	PaceZ2Max       int       `json:"pace_z2_max"`
	PaceZ3Min       int       `json:"pace_z3_min"`
	PaceZ3Max       int       `json:"pace_z3_max"`
	PaceZ4Min       int       `json:"pace_z4_min"`
	PaceZ4Max       int       `json:"pace_z4_max"`
	PaceZ5Min       int       `json:"pace_z5_min"`
	PaceZ5Max       int       `json:"pace_z5_max"`
	IndiceConfianca *int      `json:"indice_confianca,omitempty"`
	Observacoes     string    `json:"observacoes,omitempty"`
}
