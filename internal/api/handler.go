package api

import (
	"dbbridge/internal/service"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5" // Using Chi router for simplicity and pattern matching
)

type Handler struct {
	executor *service.QueryExecutor
}

func NewHandler(executor *service.QueryExecutor) *Handler {
	return &Handler{
		executor: executor,
	}
}

func (h *Handler) ExecuteQuery(w http.ResponseWriter, r *http.Request) {
	connIDStr := chi.URLParam(r, "connectionID")
	querySlug := chi.URLParam(r, "querySlug")

	connID, err := strconv.ParseInt(connIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid connection ID", http.StatusBadRequest)
		return
	}

	// Parse body params
	var params map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		// It's okay if body is empty, we just treat params as empty
		params = make(map[string]interface{})
	}

	result, err := h.executor.Execute(r.Context(), connID, querySlug, params)
	if err != nil {
		// Differentiate errors (404 vs 500) if possible, for now 500
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
	r.Use(AuthMiddleware)

	r.Post("/api/{connectionID}/{querySlug}", h.ExecuteQuery)

	return r
}
