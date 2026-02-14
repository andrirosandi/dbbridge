package api

import (
	"context"
	"dbbridge/internal/core"
	"dbbridge/internal/service"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5" // Using Chi router for simplicity and pattern matching
)

type Handler struct {
	executor   *service.QueryExecutor
	docHandler *DocHandler
	authSvc    *service.AuthService
}

func NewHandler(executor *service.QueryExecutor, docHandler *DocHandler, authSvc *service.AuthService) *Handler {
	return &Handler{
		executor:   executor,
		docHandler: docHandler,
		authSvc:    authSvc,
	}
}

func (h *Handler) ExecuteQuery(w http.ResponseWriter, r *http.Request) {
	connName := chi.URLParam(r, "connectionName")
	querySlug := chi.URLParam(r, "querySlug")

	// Look up connection by Name (using Repo directly or via Executor if added)
	// Executor currently only has Execute with connID.
	// We need to resolve Name -> ID here or add ExecuteByName to Executor.
	// Let's resolve here to keep Executor simple, but Handler needs access to ConnRepo.
	// Handler struct only has Executor.
	// Best approach: Add ExecuteByName to Executor.

	// Parse body params
	var params map[string]interface{}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&params)
	}
	if params == nil {
		params = make(map[string]interface{})
	}

	result, err := h.executor.ExecuteByName(r.Context(), connName, querySlug, params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    result.Rows,
	})
}

// Router setup
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(LoggingMiddleware)
	r.Use(h.AuthMiddleware)

	// Old route (optional to keep or remove, let's keep for ID access if needed or just replace?)
	// User asked for /{connectionname}/{queryname}.
	// API Docs
	r.Get("/docs/openapi.json", h.docHandler.GetOpenAPISpec)
	r.Get("/docs", h.docHandler.ServeSwaggerUI)

	r.Post("/{connectionName}/{querySlug}", h.ExecuteQuery)

	return r
}

func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow public access to API Docs
		if strings.HasPrefix(r.URL.Path, "/api/docs") {
			next.ServeHTTP(w, r)
			return
		}

		apiKeyStr := r.Header.Get("X-API-Key")
		if apiKeyStr == "" {
			http.Error(w, "Missing X-API-Key header", http.StatusUnauthorized)
			return
		}

		// Verify Key
		apiKey, err := h.authSvc.VerifyApiKey(apiKeyStr)
		if err != nil {
			http.Error(w, "Invalid X-API-Key", http.StatusUnauthorized)
			return
		}

		// Store API Key ID in context
		ctx := context.WithValue(r.Context(), core.ContextKeyApiKeyID, apiKey.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
