package csvsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"staff_app/internal/domain"
)

type fakeRepo struct {
	byCodigo map[int]*domain.ExercicioReabilitacao
	byNome   map[string]int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byCodigo: make(map[int]*domain.ExercicioReabilitacao),
		byNome:   make(map[string]int),
	}
}

func (f *fakeRepo) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func (f *fakeRepo) GetByCodigo(_ context.Context, codigo int) (*domain.ExercicioReabilitacao, error) {
	ex, ok := f.byCodigo[codigo]
	if !ok {
		return nil, nil
	}
	cp := *ex
	return &cp, nil
}

func (f *fakeRepo) UpsertCatalogExercise(_ context.Context, ex *domain.ExercicioReabilitacao) (bool, error) {
	if ex == nil {
		return false, errors.New("nil")
	}
	key := strings.ToLower(ex.Nome)
	if existingCode, ok := f.byNome[key]; ok && existingCode != ex.Codigo {
		return false, ErrNameConflict
	}
	if _, ok := f.byCodigo[ex.Codigo]; !ok {
		cp := *ex
		f.byCodigo[ex.Codigo] = &cp
		f.byNome[key] = ex.Codigo
		return true, nil
	}
	cur := f.byCodigo[ex.Codigo]
	if cur.CriadoPor != CatalogMarker {
		return false, fmt.Errorf("not csv-managed")
	}
	delete(f.byNome, strings.ToLower(cur.Nome))
	cur.Nome, cur.GrupoMuscular, cur.MusculoFoco, cur.Url, cur.Status = ex.Nome, ex.GrupoMuscular, ex.MusculoFoco, ex.Url, "ativo"
	f.byNome[key] = ex.Codigo
	return false, nil
}

func seed(f *fakeRepo, codigo int, nome, criadoPor, grupo string) {
	f.byCodigo[codigo] = &domain.ExercicioReabilitacao{
		Codigo: codigo, Nome: nome, CriadoPor: criadoPor, Status: "ativo", GrupoMuscular: grupo,
	}
	f.byNome[strings.ToLower(nome)] = codigo
}

func TestSyncBehaviors(t *testing.T) {
	t.Run("insert and idempotent", func(t *testing.T) {
		repo := newFakeRepo()
		recs := []Record{{Codigo: 100, Nome: "Supino", GrupoMuscular: "Peito", URL: "https://ex/100"}}
		res, err := Sync(t.Context(), repo, recs)
		if err != nil || res.Inserted != 1 {
			t.Fatalf("first=%+v err=%v", res, err)
		}
		res2, err := Sync(t.Context(), repo, recs)
		if err != nil || res2.Inserted != 0 || res2.Updated != 0 {
			t.Fatalf("idempotent=%+v err=%v", res2, err)
		}
	})

	t.Run("update csv-managed", func(t *testing.T) {
		repo := newFakeRepo()
		_, _ = Sync(t.Context(), repo, []Record{{Codigo: 100, Nome: "Supino", GrupoMuscular: "Peito", URL: "https://ex/old"}})
		res, err := Sync(t.Context(), repo, []Record{{Codigo: 100, Nome: "Supino Reto", GrupoMuscular: "Peitoral", URL: "https://ex/new"}})
		if err != nil || res.Updated != 1 {
			t.Fatalf("result=%+v err=%v", res, err)
		}
		got, _ := repo.GetByCodigo(t.Context(), 100)
		if got.Nome != "Supino Reto" || got.Url != "https://ex/new" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("skip owner conflict", func(t *testing.T) {
		repo := newFakeRepo()
		seed(repo, 100, "Manual", "admin", "Keep")
		res, err := Sync(t.Context(), repo, []Record{{Codigo: 100, Nome: "From CSV", GrupoMuscular: "Overwrite"}})
		if err != nil || res.SkippedDBOwnerConflict != 1 {
			t.Fatalf("result=%+v err=%v", res, err)
		}
		got, _ := repo.GetByCodigo(t.Context(), 100)
		if got.Nome != "Manual" || got.GrupoMuscular != "Keep" {
			t.Fatalf("mutated %+v", got)
		}
	})

	t.Run("skip name conflict", func(t *testing.T) {
		repo := newFakeRepo()
		seed(repo, 200, "Nome Em Uso", "admin", "")
		res, err := Sync(t.Context(), repo, []Record{{Codigo: 100, Nome: "Nome Em Uso", GrupoMuscular: "Peito"}})
		if err != nil || res.SkippedNameConflict != 1 || res.Inserted != 0 {
			t.Fatalf("result=%+v err=%v", res, err)
		}
	})

	t.Run("personalizado untouched", func(t *testing.T) {
		repo := newFakeRepo()
		seed(repo, 5000, "Personalizado", "admin", "")
		res, err := Sync(t.Context(), repo, []Record{{Codigo: 100, Nome: "Catalogo", GrupoMuscular: "Peito"}})
		if err != nil || res.Inserted != 1 {
			t.Fatalf("result=%+v err=%v", res, err)
		}
		got, _ := repo.GetByCodigo(t.Context(), 5000)
		if got == nil || got.Nome != "Personalizado" {
			t.Fatalf("personalizado=%+v", got)
		}
	})

	t.Run("empty noop and defaults", func(t *testing.T) {
		repo := newFakeRepo()
		if res, err := Sync(t.Context(), repo, nil); err != nil || res != (Result{}) {
			t.Fatalf("nil noop: %+v %v", res, err)
		}
		_, err := Sync(t.Context(), repo, []Record{{Codigo: 42, Nome: "Sem URL"}})
		if err != nil {
			t.Fatal(err)
		}
		got, _ := repo.GetByCodigo(t.Context(), 42)
		if got.Url != "https://rcstorestaff.com.br/exercicios_html/42" || got.CriadoPor != CatalogMarker || got.NivelPrioridade != 2 {
			t.Fatalf("defaults %+v", got)
		}
	})
}
