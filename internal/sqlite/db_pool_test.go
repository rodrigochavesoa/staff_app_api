package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"staff_app/internal/platform/logger"
)

func TestSQLitePoolConcurrencyRespectsMaxOpenConns(t *testing.T) {
	logger.Setup("development", false)

	db, err := Connect(filepath.Join(t.TempDir(), "pool.db"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const workers = 64
	start := make(chan struct{})
	statsCh := make(chan int, workers*2)
	errs := make(chan error, workers)

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			<-start
			statsCh <- db.Stats().OpenConnections

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			if err := db.PingContext(ctx); err != nil {
				errs <- fmt.Errorf("ping: %w", err)
				return
			}

			var result int
			if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
				errs <- fmt.Errorf("select 1: %w", err)
				return
			}
			if result != 1 {
				errs <- fmt.Errorf("select 1 returned %d", result)
				return
			}

			statsCh <- db.Stats().OpenConnections
		})
	}

	close(start)
	wg.Wait()
	close(statsCh)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent database operation failed: %v", err)
		}
	}

	maxOpen := db.Stats().OpenConnections
	for openConnections := range statsCh {
		if openConnections > maxOpen {
			maxOpen = openConnections
		}
	}
	if maxOpen > 1 {
		t.Fatalf("pool opened %d connections during concurrent load, want at most 1", maxOpen)
	}

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for {
		stats := db.Stats()
		if stats.OpenConnections > 1 {
			t.Fatalf("pool opened %d connections after concurrent load, want at most 1", stats.OpenConnections)
		}
		if stats.InUse == 0 {
			return
		}

		select {
		case <-ticker.C:
		case <-timeout.C:
			t.Fatalf("pool still has %d connections in use after concurrent load", stats.InUse)
		}
	}
}
