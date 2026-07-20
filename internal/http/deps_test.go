package http

import (
	"path/filepath"
	"testing"

	"staff_app/internal/sqlite"
)

func newTestDeps(t *testing.T) (Deps, *sqlite.DB) {
	t.Helper()

	db, err := sqlite.Connect(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return NewSQLiteDeps(db), db
}

func depsForTestDB(db *sqlite.DB) Deps {
	return NewSQLiteDeps(db)
}
