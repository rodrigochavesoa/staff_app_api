package csvsync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFileReadsGrupoMuscularAndFoco(t *testing.T) {
	path := writeCSV(t, "exercicios.csv",
		"Código,Nome do Exercício,grupo_muscular,musculo_foco,url\n"+
			"100,Abdominal Amplitude Máxima,Abdome,Reto Abdominal,https://example.com/100\n"+
			"200,Supino Reto,Peito,Peitoral Maior,https://example.com/200\n",
	)

	list, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d want 2", len(list))
	}
	if list[0].GrupoMuscular != "Abdome" || list[0].MusculoFoco != "Reto Abdominal" {
		t.Fatalf("row0=%+v", list[0])
	}
	if list[1].GrupoMuscular != "Peito" || list[1].MusculoFoco != "Peitoral Maior" {
		t.Fatalf("row1=%+v", list[1])
	}
	if list[0].URL != "https://example.com/100" || list[1].URL != "https://example.com/200" {
		t.Fatalf("urls=%q,%q", list[0].URL, list[1].URL)
	}
}

func TestParseFileLegacyGrupoMuscularHeader(t *testing.T) {
	path := writeCSV(t, "legacy.csv", "codigo,nome,grupo muscular\n1,Remada Curvada,Costas\n")

	list, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(list) != 1 || list[0].GrupoMuscular != "Costas" || list[0].Codigo != 1 {
		t.Fatalf("got %+v", list)
	}
}

func TestParseFileMissingFileReturnsNil(t *testing.T) {
	list, err := ParseFile(filepath.Join(t.TempDir(), "missing.csv"))
	if err != nil {
		t.Fatalf("err=%v want nil", err)
	}
	if list != nil {
		t.Fatalf("list=%v want nil", list)
	}
}

func TestParseFileSkipsReservedAndInvalidCodigo(t *testing.T) {
	path := writeCSV(t, "reserved.csv",
		"codigo,nome,grupo\n"+
			"0,Zero Invalid,Abdome\n"+
			"-1,Negativo,Abdome\n"+
			"abc,Nao Numerico,Abdome\n"+
			"5000,Reserved Personalizado,Abdome\n"+
			"4999,Ultimo Catalogo,Peito\n"+
			",Sem Codigo,Costas\n",
	)

	list, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len=%d want 1; got %+v", len(list), list)
	}
	if list[0].Codigo != 4999 || list[0].Nome != "Ultimo Catalogo" {
		t.Fatalf("got %+v", list[0])
	}
}

func TestParseFileSkipsEmptyName(t *testing.T) {
	path := writeCSV(t, "empty-name.csv",
		"codigo,nome,grupo\n"+
			"10,,Abdome\n"+
			"11,  ,Peito\n"+
			"12,Valido,Costas\n",
	)

	list, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(list) != 1 || list[0].Codigo != 12 {
		t.Fatalf("got %+v", list)
	}
}

func TestParseFileDuplicateCodigoLastWins(t *testing.T) {
	path := writeCSV(t, "dup.csv",
		"codigo,nome,grupo_muscular,url\n"+
			"100,Primeiro Nome,Abdome,https://example.com/old\n"+
			"200,Outro,Peito,https://example.com/200\n"+
			"100,Ultimo Nome,Costas,https://example.com/new\n",
	)

	list, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d want 2; got %+v", len(list), list)
	}
	if list[0].Codigo != 100 || list[0].Nome != "Ultimo Nome" || list[0].GrupoMuscular != "Costas" {
		t.Fatalf("dup row=%+v", list[0])
	}
	if list[0].URL != "https://example.com/new" {
		t.Fatalf("url=%q", list[0].URL)
	}
	if list[1].Codigo != 200 {
		t.Fatalf("second=%+v", list[1])
	}
}

func TestCatalogMarkerAndDefaultCSVPath(t *testing.T) {
	if CatalogMarker != "csv" {
		t.Fatalf("CatalogMarker=%q want csv", CatalogMarker)
	}
	want := filepath.Join("data", "csv", "exercicios_com_grupos.csv")
	if got := DefaultCSVPath(); got != want {
		t.Fatalf("DefaultCSVPath=%q want %q", got, want)
	}
}

func writeCSV(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	return path
}
