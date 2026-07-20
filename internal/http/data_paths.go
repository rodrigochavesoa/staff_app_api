package http

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// resolveStaffDataPath retorna um caminho absoluto sob staff_app/data ou STAFF_DATA_DIR.
func resolveStaffDataPath(parts ...string) string {
	return filepath.Join(append([]string{staffAppDataDir()}, parts...)...)
}

func staffAppDataDir() string {
	if base := strings.TrimSpace(os.Getenv("STAFF_DATA_DIR")); base != "" {
		return base
	}
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for {
			candidate := filepath.Join(dir, "data")
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	_, file, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "data"))
	}
	return "data"
}
