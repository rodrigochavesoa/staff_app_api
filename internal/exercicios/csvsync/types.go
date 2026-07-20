// Package csvsync parses and syncs the exercise catalog CSV into SQLite.
package csvsync

import "path/filepath"

// Record represents a normalized catalog row from the CSV.
type Record struct {
	Codigo        int
	Nome          string
	GrupoMuscular string
	MusculoFoco   string
	URL           string
}

// Result summarizes effects of a Sync (populated in PR2).
type Result struct {
	Inserted               int
	Updated                int
	SkippedEmptyName       int
	SkippedInvalidCode     int
	SkippedReservedRange   int
	SkippedDBOwnerConflict int
	SkippedNameConflict    int
}

// CatalogMarker is stored in criado_por for CSV-managed rows.
const CatalogMarker = "csv"

// DefaultCSVPath returns the canonical catalog path relative to the process CWD.
func DefaultCSVPath() string {
	return filepath.Join("data", "csv", "exercicios_com_grupos.csv")
}
