package http

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"staff_app/internal/platform/logger"
)

// responseWriter is a wrapper around http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// LoggerMiddleware logs each HTTP request and flags slow ones (>2s)
func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newResponseWriter(w)

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		msg := fmt.Sprintf("request: method=%s path=%s status=%d time=%.3fs",
			r.Method, r.URL.Path, rw.statusCode, duration.Seconds())

		if duration.Seconds() > 2.0 {
			logger.Warn("SLOW_REQUEST " + msg)
		} else {
			logger.Info(msg)
		}
	})
}

// RecoveryMiddleware handles panics and returns a generic 500 error
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Get stack trace
				stack := debug.Stack()
				logger.Error(fmt.Sprintf("UNHANDLED_EXCEPTION method=%s path=%s panic=%v", r.Method, r.URL.Path, err), nil)
				logger.Error("Traceback:\n"+string(stack), nil)

				// Respond generically
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"Internal Server Error"}`))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// CorsMiddleware injects CORS headers based on configured origins
func CorsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			isAllowed := false
			if origin != "" {
				for _, o := range allowedOrigins {
					if o == "*" || o == origin || (strings.HasSuffix(o, "*") && strings.HasPrefix(origin, strings.TrimSuffix(o, "*"))) {
						isAllowed = true
						break
					}
				}
			}

			if isAllowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
