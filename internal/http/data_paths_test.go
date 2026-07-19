package http

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveStaffDataPath(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	want := filepath.Join(root, "data", "json", "templates_daniels_blocos.json")

	got := resolveStaffDataPath("json", "templates_daniels_blocos.json")
	if got != want {
		t.Fatalf("resolveStaffDataPath()=%q want %q", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute path, got %q", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("templates file missing at %q: %v", got, err)
	}
}
