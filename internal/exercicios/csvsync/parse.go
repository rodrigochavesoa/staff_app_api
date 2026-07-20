package csvsync

import (
	"encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseFile reads and validates the catalog CSV.
// A missing file returns (nil, nil). Invalid rows are skipped; duplicate
// codigo keeps the last occurrence (first-seen order among unique codes).
func ParseFile(path string) ([]Record, error) {
	file, err := os.Open(filepath.Clean(path)) // #nosec G304 -- path from caller/DefaultCSVPath
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}

	colCodigo, colNome, colGrupo, colFoco, colURL := -1, -1, -1, -1, -1
	for i, h := range headers {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "código", "codigo":
			colCodigo = i
		case "nome do exercício", "nome do exercicio", "nome":
			colNome = i
		case "grupo_muscular", "grupo muscular", "grupo":
			colGrupo = i
		case "musculo_foco", "músculo_foco", "musculo foco", "músculo foco":
			colFoco = i
		case "url":
			colURL = i
		}
	}

	byCode := make(map[int]int) // codigo -> index in list
	var list []Record

	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		nome := cell(row, colNome)
		if nome == "" {
			continue
		}

		codigo, ok := parseCodigo(row, colCodigo)
		if !ok {
			continue
		}

		rec := Record{
			Codigo:        codigo,
			Nome:          nome,
			GrupoMuscular: cell(row, colGrupo),
			MusculoFoco:   cell(row, colFoco),
			URL:           cell(row, colURL),
		}

		if idx, exists := byCode[codigo]; exists {
			list[idx] = rec
			continue
		}
		byCode[codigo] = len(list)
		list = append(list, rec)
	}

	return list, nil
}

func cell(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}

func parseCodigo(row []string, col int) (int, bool) {
	raw := cell(row, col)
	if raw == "" {
		return 0, false
	}
	code, err := strconv.Atoi(raw)
	if err != nil || code <= 0 || code >= 5000 {
		return 0, false
	}
	return code, true
}
