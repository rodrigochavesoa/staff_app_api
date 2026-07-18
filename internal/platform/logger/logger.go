package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Log is the global logger instance
var Log *slog.Logger

// Setup initializes the global structured logger
func Setup(env string, writeToFile bool) func() {
	var handler slog.Handler
	var logFile *os.File

	// Determine output writer
	var writer io.Writer = os.Stdout

	if writeToFile {
		// Create logs directory if not exists
		logDir := "logs"
		// Restrict logs directory permission to 0750 (G301)
		if err := os.MkdirAll(logDir, 0o750); err == nil {
			logPath := filepath.Join(logDir, "api.log")
			var err error
			// #nosec G304 - logPath is trusted and fixed under logDir
			// Restrict log file permission to 0600 (G302)
			logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
			if err == nil {
				// Write to both stdout and log file
				writer = io.MultiWriter(os.Stdout, logFile)
			}
		}
	}

	// Choose format based on environment
	if env == "production" {
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	Log = slog.New(handler)
	slog.SetDefault(Log)

	// Return cleanup function to close log file
	return func() {
		if logFile != nil {
			// Ignored errors on Sync/Close are handled/discarded explicitly as we are exiting (G104)
			_ = logFile.Sync()
			_ = logFile.Close()
		}
	}
}

// Helper methods for easy logging
func Debug(msg string, args ...any) {
	Log.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	Log.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	Log.Warn(msg, args...)
}

func Error(msg string, err error, args ...any) {
	if err != nil {
		args = append(args, slog.Any("error", err))
	}
	Log.Error(msg, args...)
}

func ErrorCtx(ctx context.Context, msg string, err error, args ...any) {
	if err != nil {
		args = append(args, slog.Any("error", err))
	}
	Log.ErrorContext(ctx, msg, args...)
}
