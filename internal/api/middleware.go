package api

import (
	"context"
	"dbbridge/internal/logger"
	"net/http"
	"time"
)

type Middleware struct {
	// Add dependencies if needed (e.g. Auth Service)
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		rw := &responseWriter{w, http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		logger.Info.Printf("%s %s %d %v", r.Method, r.URL.Path, rw.status, duration)
	})
}

// Custom response writer to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Context keys
type key int

const (
	UserKey key = iota
)

// AuthMiddleware - Placeholder for now until we implement full Auth Service
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			http.Error(w, "Missing X-API-Key header", http.StatusUnauthorized)
			return
		}

		// TODO: Validate API Key against DB/Service
		// For MVP/Scaffolding, checks against a dev SUPERKEY or mock logic
		// In production, this must query the database.

		// Pass basic context
		ctx := context.WithValue(r.Context(), UserKey, "admin")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
