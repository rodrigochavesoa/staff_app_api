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
)

type Server struct {
	httpServer *http.Server
	shutdown   func() error
}

func NewServer(cfg *config.Config, deps Deps) *Server {
	router := NewRouter(cfg, deps)

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{
		httpServer: httpServer,
		shutdown:   deps.Shutdown,
	}
}

func (s *Server) Start() error {
	serverErrors := make(chan error, 1)

	go func() {
		logger.Info(fmt.Sprintf("HTTP Server listening on %s", s.httpServer.Addr))
		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server startup failed: %w", err)

	case sig := <-shutdownSignal:
		logger.Info("Shutdown signal received, stopping HTTP server...", "signal", sig.String())

		// Graceful shutdown com timeout de 15s.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := s.httpServer.Shutdown(ctx); err != nil {
			_ = s.httpServer.Close()
			return fmt.Errorf("failed to shutdown HTTP server gracefully: %w", err)
		}

		if s.shutdown != nil {
			logger.Info("Closing database connection pool...")
			if err := s.shutdown(); err != nil {
				logger.Error("Error closing database connection", err)
			}
		}

		logger.Info("HTTP server stopped cleanly.")
	}

	return nil
}
