package csvsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"staff_app/internal/domain"
)

// ErrNameConflict is returned by UpsertCatalogExercise when INSERT hits UNIQUE(nome).
var ErrNameConflict = errors.New("csvsync: unique name conflict")

// SyncRepository is implemented by sqlite.ExercicioRepository (consumer-side interface).
type SyncRepository interface {
	GetByCodigo(ctx context.Context, codigo int) (*domain.ExercicioReabilitacao, error)
	UpsertCatalogExercise(ctx context.Context, ex *domain.ExercicioReabilitacao) (inserted bool, err error)
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Sync materializes catalog records into the repository in a single transaction.
// nil or empty records are a no-op. Duplicate codes should already be collapsed by ParseFile.
func Sync(ctx context.Context, repo SyncRepository, records []Record) (Result, error) {
	var result Result
	if len(records) == 0 {
		return result, nil
	}

	err := repo.WithTx(ctx, func(ctx context.Context) error {
		now := time.Now().UTC()
		for _, rec := range records {
			if err := syncOne(ctx, repo, rec, now, &result); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func syncOne(ctx context.Context, repo SyncRepository, rec Record, now time.Time, result *Result) error {
	if strings.TrimSpace(rec.Nome) == "" {
		result.SkippedEmptyName++
		return nil
	}
	if rec.Codigo <= 0 {
		result.SkippedInvalidCode++
		return nil
	}
	if rec.Codigo >= 5000 {
		result.SkippedReservedRange++
		return nil
	}

	existing, err := repo.GetByCodigo(ctx, rec.Codigo)
	if err != nil {
		return fmt.Errorf("get codigo %d: %w", rec.Codigo, err)
	}

	if existing != nil && existing.CriadoPor != CatalogMarker {
		result.SkippedDBOwnerConflict++
		return nil
	}

	ex := catalogExerciseFromRecord(rec, now)
	if existing != nil && !catalogFieldsChanged(existing, ex) {
		return nil
	}

	inserted, err := repo.UpsertCatalogExercise(ctx, ex)
	if errors.Is(err, ErrNameConflict) {
		result.SkippedNameConflict++
		return nil
	}
	if err != nil {
		return fmt.Errorf("upsert codigo %d: %w", rec.Codigo, err)
	}
	if inserted {
		result.Inserted++
	} else {
		result.Updated++
	}
	return nil
}

func catalogExerciseFromRecord(rec Record, now time.Time) *domain.ExercicioReabilitacao {
	url := strings.TrimSpace(rec.URL)
	if url == "" {
		url = fmt.Sprintf("https://rcstorestaff.com.br/exercicios_html/%d", rec.Codigo)
	}
	return &domain.ExercicioReabilitacao{
		Codigo:          rec.Codigo,
		Nome:            strings.TrimSpace(rec.Nome),
		Categoria:       "normal",
		GrupoMuscular:   strings.TrimSpace(rec.GrupoMuscular),
		MusculoFoco:     strings.TrimSpace(rec.MusculoFoco),
		NivelPrioridade: 2,
		Url:             url,
		CriadoPor:       CatalogMarker,
		CriadoEm:        now,
		Status:          "ativo",
	}
}

func catalogFieldsChanged(existing, next *domain.ExercicioReabilitacao) bool {
	return existing.Nome != next.Nome ||
		existing.GrupoMuscular != next.GrupoMuscular ||
		existing.MusculoFoco != next.MusculoFoco ||
		existing.Url != next.Url ||
		existing.Status != "ativo"
}
