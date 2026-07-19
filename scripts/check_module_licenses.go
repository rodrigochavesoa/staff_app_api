//go:build ignore

// License allowlist gate for Go module dependencies.
// Usage (from repo root): go run scripts/check_module_licenses.go
package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var allowed = map[string]bool{
	"Apache-2.0":       true,
	"MIT":              true,
	"BSD-2-Clause":     true,
	"BSD-3-Clause":     true,
	"ISC":              true,
	"Zlib":             true,
	"Unicode-DFS-2016": true,
	"Unicode-3.0":      true,
	"CC0-1.0":          true, // review-required in policy; treated as noted below
	"BSL-1.0":          true, // Boost — permissive
}

// Manual classifications when LICENSE text has no SPDX header.
var overrides = map[string]string{
	"modernc.org/mathutil": "BSD-3-Clause",
}

var blockedNeedles = []string{
	"gnu general public license",
	"gpl-2.0",
	"gpl-3.0",
	"agpl",
	"sspl",
	"commons clause",
	"business source",
	"polyform",
}

func main() {
	outDir := getenv("OUT_DIR", "artifacts/licenses")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fail(err)
	}

	pkgs := getenv("LICENSE_PACKAGES", "./cmd/api")
	mods, err := listModules(strings.Fields(pkgs)...)
	if err != nil {
		fail(err)
	}

	reportPath := filepath.Join(outDir, "module-licenses.csv")
	f, err := os.Create(reportPath)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"module", "version", "license", "source"})

	var failures []string
	var review []string

	for _, m := range mods {
		if m.Path == "" || m.Main {
			continue
		}
		lic, src := classify(m)
		_ = w.Write([]string{m.Path, m.Version, lic, src})
		switch {
		case lic == "UNKNOWN":
			failures = append(failures, fmt.Sprintf("%s@%s: unknown license", m.Path, m.Version))
		case isBlocked(lic, src):
			failures = append(failures, fmt.Sprintf("%s@%s: blocked license %q", m.Path, m.Version, lic))
		case lic == "CC0-1.0" || strings.Contains(strings.ToLower(lic), "mpl") || strings.Contains(strings.ToLower(lic), "lgpl"):
			review = append(review, fmt.Sprintf("%s@%s: %s (review required)", m.Path, m.Version, lic))
		case !allowed[lic] && !strings.HasPrefix(lic, "BSD") && !strings.Contains(lic, "MIT") && !strings.Contains(lic, "Apache"):
			// permissive dual licenses often look like "MIT OR Apache-2.0"
			if !isPermissiveExpression(lic) {
				failures = append(failures, fmt.Sprintf("%s@%s: not on allowlist (%s)", m.Path, m.Version, lic))
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fail(err)
	}

	summary := filepath.Join(outDir, "module-licenses-summary.txt")
	var b strings.Builder
	fmt.Fprintf(&b, "modules_scanned=%d\nreport=%s\n", len(mods), reportPath)
	if len(review) > 0 {
		fmt.Fprintf(&b, "\nreview_required (%d):\n", len(review))
		for _, r := range review {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}
	if len(failures) > 0 {
		fmt.Fprintf(&b, "\nfailures (%d):\n", len(failures))
		for _, r := range failures {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	} else {
		b.WriteString("\nstatus=PASS\n")
	}
	_ = os.WriteFile(summary, []byte(b.String()), 0o644)
	fmt.Print(b.String())
	if len(failures) > 0 {
		os.Exit(1)
	}
}

type module struct {
	Path    string
	Version string
	Dir     string
	Main    bool
}

func listModules(packages ...string) ([]module, error) {
	// Runtime/distribution scope: module closure of the given packages
	// (default ./cmd/api), not the full go.mod graph (tools/tests unused here).
	args := append([]string{
		"list", "-deps",
		"-f", `{{if and (not .Standard) .Module}}{{.Module.Path}}{{"\t"}}{{.Module.Version}}{{"\t"}}{{.Module.Dir}}{{"\t"}}{{.Module.Main}}{{end}}`,
	}, packages...)
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -deps: %w", err)
	}
	seen := map[string]module{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		m := module{
			Path:    parts[0],
			Version: parts[1],
			Dir:     parts[2],
			Main:    parts[3] == "true",
		}
		seen[m.Path+"@"+m.Version] = m
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	mods := make([]module, 0, len(seen))
	for _, m := range seen {
		mods = append(mods, m)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Path < mods[j].Path })
	return mods, nil
}

func classify(m module) (license, source string) {
	if lic, ok := overrides[m.Path]; ok {
		return lic, "override"
	}
	if m.Dir == "" {
		return "UNKNOWN", "no-dir"
	}
	candidates := []string{
		"LICENSE", "LICENSE.txt", "LICENSE.md", "COPYING", "COPYING.txt",
		"LICENCE", "UNLICENSE", "NOTICE",
	}
	for _, name := range candidates {
		p := filepath.Join(m.Dir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := string(data)
		if id := detectSPDX(text); id != "" {
			return id, name
		}
		if id := detectHeuristic(text); id != "" {
			return id, name + "+heuristic"
		}
		return "UNKNOWN", name + "+unclassified"
	}
	// some modules nest license under subdirs (rare)
	_ = filepath.WalkDir(m.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && path != m.Dir {
				base := filepath.Base(path)
				if base == ".git" || base == "testdata" || base == "vendor" {
					return filepath.SkipDir
				}
				// keep walk shallow
				rel, _ := filepath.Rel(m.Dir, path)
				if strings.Count(rel, string(os.PathSeparator)) >= 1 && path != m.Dir {
					return filepath.SkipDir
				}
			}
			return nil
		}
		base := strings.ToUpper(d.Name())
		if !strings.Contains(base, "LICENSE") && !strings.Contains(base, "COPYING") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if license != "" {
			return nil
		}
		text := string(data)
		if id := detectSPDX(text); id != "" {
			license, source = id, filepath.Base(path)
		} else if id := detectHeuristic(text); id != "" {
			license, source = id, filepath.Base(path)+"+heuristic"
		}
		return nil
	})
	if license == "" {
		return "UNKNOWN", "missing"
	}
	return license, source
}

func detectSPDX(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "spdx-license-identifier:") {
			return strings.TrimSpace(line[len("SPDX-License-Identifier:"):])
		}
	}
	return ""
}

func detectHeuristic(text string) string {
	low := strings.ToLower(text)
	switch {
	case strings.Contains(low, "apache license") && strings.Contains(low, "version 2.0"):
		return "Apache-2.0"
	case strings.Contains(low, "permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(low, "redistribution and use in source and binary forms") &&
		strings.Contains(low, "neither the name") &&
		strings.Contains(low, "endorse"):
		return "BSD-3-Clause"
	case strings.Contains(low, "redistribution and use in source and binary forms") &&
		!strings.Contains(low, "neither the name"):
		return "BSD-2-Clause"
	case strings.Contains(low, "isc license") || (strings.Contains(low, "permission to use, copy, modify") && strings.Contains(low, "and/or distribute")):
		if strings.Contains(low, "isc license") {
			return "ISC"
		}
	case strings.Contains(low, "zlib license") || strings.Contains(low, "origin of this software must not be misrepresented"):
		return "Zlib"
	case strings.Contains(low, "boost software license"):
		return "BSL-1.0"
	}
	return ""
}

func isBlocked(lic, src string) bool {
	blob := strings.ToLower(lic + " " + src)
	for _, n := range blockedNeedles {
		if strings.Contains(blob, n) && !strings.Contains(blob, "lgpl") {
			return true
		}
	}
	return false
}

func isPermissiveExpression(lic string) bool {
	low := strings.ToLower(lic)
	if strings.Contains(low, "gpl") && !strings.Contains(low, "lgpl") {
		return false
	}
	parts := strings.FieldsFunc(lic, func(r rune) bool {
		return r == '|' || r == '/' || r == ',' || r == '(' || r == ')'
	})
	ok := 0
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "OR ")
		p = strings.TrimPrefix(p, "AND ")
		p = strings.TrimSpace(p)
		if p == "" || strings.EqualFold(p, "OR") || strings.EqualFold(p, "AND") {
			continue
		}
		if allowed[p] || strings.EqualFold(p, "MIT") || strings.Contains(p, "BSD") || strings.Contains(p, "Apache") {
			ok++
		}
	}
	return ok > 0
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
