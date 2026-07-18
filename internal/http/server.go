package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"staff_app/internal/config"
	"staff_app/internal/platform/logger"
	"staff_app/internal/sqlite"
)

// Server wraps the http.Server and handles startup and shutdown
type Server struct {
	httpServer *http.Server
	db         *sqlite.DB
}

// NewServer creates a new configured Server instance
func NewServer(cfg *config.Config, db *sqlite.DB) *Server {
	router := NewRouter(cfg, db)

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		httpServer: httpServer,
		db:         db,
	}
}

// Start runs the HTTP server and listens for OS shutdown signals
func (s *Server) Start() error {
	serverErrors := make(chan error, 1)

	// Start server in background goroutine
	go func() {
		logger.Info(fmt.Sprintf("HTTP Server listening on %s", s.httpServer.Addr))
		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// Channel to listen for interrupt/termination signals
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server startup failed: %w", err)

	case sig := <-shutdownSignal:
		logger.Info("Shutdown signal received, stopping HTTP server...", "signal", sig.String())

		// Timeout context for graceful shutdown (15 seconds)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Shutdown HTTP server gracefully
		if err := s.httpServer.Shutdown(ctx); err != nil {
			// Force shutdown if graceful fails
			_ = s.httpServer.Close()
			return fmt.Errorf("failed to shutdown HTTP server gracefully: %w", err)
		}

		// Close DB connection pool
		if s.db != nil {
			logger.Info("Closing database connection pool...")
			if err := s.db.Close(); err != nil {
				logger.Error("Error closing database connection", err)
			}
		}

		logger.Info("HTTP server stopped cleanly.")
	}

	return nil
}
