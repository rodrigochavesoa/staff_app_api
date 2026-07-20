package http

import (
	"staff_app/internal/sqlite"
)

func depsForTestDB(db *sqlite.DB) Deps {
	return NewSQLiteDeps(db)
}
