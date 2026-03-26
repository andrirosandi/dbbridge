package api

import (
	"dbbridge/internal/core"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

type DocHandler struct {
	queryRepo core.QueryRepository
	connRepo  core.ConnectionRepository
	parser    *core.SQLParser
}

func NewDocHandler(queryRepo core.QueryRepository, connRepo core.ConnectionRepository) *DocHandler {
	return &DocHandler{
		queryRepo: queryRepo,
		connRepo:  connRepo,
		parser:    core.NewSQLParser(),
	}
}

func (h *DocHandler) ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	// Simple HTML to load Swagger UI
	html := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>DbBridge API Docs</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" />
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js" crossorigin></script>
<script>
    window.onload = () => {
        window.ui = SwaggerUIBundle({
            url: '/api/docs/openapi.json',
            dom_id: '#swagger-ui',
        });
    };
</script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func (h *DocHandler) GetOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	queries, err := h.queryRepo.GetAll()
	if err != nil {
		http.Error(w, "Failed to list queries", http.StatusInternalServerError)
		return
	}

	connections, err := h.connRepo.GetAll()
	if err != nil {
		http.Error(w, "Failed to list connections", http.StatusInternalServerError)
		return
	}

	// Build Paths
	paths := make(map[string]interface{})

	for _, conn := range connections {
		if !conn.IsActive {
			continue
		}

		// Group by Connection Name (Tag)
		connSlug := core.Slugify(conn.Name)

		for _, q := range queries {
			// Check if query is allowed for this connection
			allowed := false
			for _, id := range q.AllowedConnectionIDs {
				if id == conn.ID {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}

			pathKey := fmt.Sprintf("/api/%s/%s", connSlug, q.Slug)

			// Parse Params
			parseRes := h.parser.Parse(q.SQLText, nil)

			properties := make(map[string]interface{})
			exampleBody := make(map[string]interface{})

			hasPagination := false
			hasOrderBy := false

			// Use same regex as executor for consistency
			rePagination := regexp.MustCompile(`(?i)\{\s*pagination(?::\s*(\d*)\s*:\s*(\d*)\s*)?\}`)
			reOrderBy := regexp.MustCompile(`(?i)\{\s*order_by\s*:`)

			if rePagination.MatchString(q.SQLText) {
				hasPagination = true
			}
			if reOrderBy.MatchString(q.SQLText) {
				hasOrderBy = true
			}

			for _, param := range parseRes.ParamNames {
				// Exclude system variables: pagination, select, endselect, order_by
				lower := strings.ToLower(param)
				if lower == "pagination" || lower == "select" || lower == "endselect" || lower == "order_by" {
					if lower == "pagination" {
						hasPagination = true
					}
					continue
				}

				properties[param] = map[string]string{"type": "string"}
				exampleBody[param] = "value"
			}

			// Add Pagination params if {pagination} is present
			if hasPagination {
				properties["page"] = map[string]interface{}{"type": "integer", "default": 1}
				properties["per_page"] = map[string]interface{}{"type": "integer", "default": 50}
			}

			// Add Order By params if {order_by} is present
			if hasOrderBy {
				properties["order_by"] = map[string]string{"type": "string"}
				properties["order_direction"] = map[string]string{"type": "string"}
				exampleBody["order_by"] = "column_name"
				exampleBody["order_direction"] = "asc"
			}

			operation := map[string]interface{}{
				"summary":     q.Slug,
				"description": q.Description,
				"tags":        []string{conn.Name},
				"requestBody": map[string]interface{}{
					"required": true,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type":       "object",
								"properties": properties,
							},
							"example": exampleBody,
						},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Successful execution",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"data": map[string]interface{}{
											"type":        "array",
											"description": "Array of result rows",
											"items": map[string]interface{}{
												"type": "object",
											},
										},
										"meta": map[string]interface{}{
											"type":        "object",
											"description": "Metadata information (pagination, total count)",
											"properties": map[string]interface{}{
												"columns": map[string]interface{}{
													"type":        "array",
													"description": "Column names in the result",
													"items":       map[string]string{"type": "string"},
												},
												"total": map[string]interface{}{
													"type":        "integer",
													"description": "Total number of rows (requires {select}...{endselect} block in query)",
													"nullable":    true,
												},
												"page": map[string]interface{}{
													"type":        "integer",
													"description": "Current page number (requires {pagination} in query)",
													"nullable":    true,
												},
												"per_page": map[string]interface{}{
													"type":        "integer",
													"description": "Items per page (requires {pagination} in query)",
													"nullable":    true,
												},
												"total_pages": map[string]interface{}{
													"type":        "integer",
													"description": "Total number of pages (requires {pagination} in query)",
													"nullable":    true,
												},
												"has_next": map[string]interface{}{
													"type":        "boolean",
													"description": "Has next page (requires {pagination} in query)",
													"nullable":    true,
												},
												"has_prev": map[string]interface{}{
													"type":        "boolean",
													"description": "Has previous page (requires {pagination} in query)",
													"nullable":    true,
												},
												"next_page": map[string]interface{}{
													"type":        "integer",
													"description": "Next page number (requires {pagination} in query)",
													"nullable":    true,
												},
												"prev_page": map[string]interface{}{
													"type":        "integer",
													"description": "Previous page number (requires {pagination} in query)",
													"nullable":    true,
												},
											},
										},
										"error": map[string]interface{}{
											"type":        "string",
											"description": "Error message from query execution (COUNT errors are non-fatal)",
											"nullable":    true,
										},
										"debug_sql": map[string]interface{}{
											"type":        "string",
											"description": "The actual SQL query executed (only when DEBUG=true env is set)",
											"nullable":    true,
										},
										"debug_count_sql": map[string]interface{}{
											"type":        "string",
											"description": "The COUNT query for metadata (only when DEBUG=true env is set)",
											"nullable":    true,
										},
										"debug_args": map[string]interface{}{
											"type":        "array",
											"description": "Parameter values used in the query (only when DEBUG=true env is set)",
											"nullable":    true,
										},
									},
								},
							},
						},
					},
					"400": map[string]interface{}{
						"description": "Bad Request - Invalid parameters or missing required fields",
					},
					"500": map[string]interface{}{
						"description": "Internal Server Error",
					},
				},
			}

			paths[pathKey] = map[string]interface{}{
				"post": operation,
			}
		}
	}

	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "DbBridge API",
			"version":     "1.0.0",
			"description": "Dynamic API generated from Saved Queries.\n\n## Query Variables (in SQL)\n- `{param}` - Standard parameter\n- `{param:default}` - Parameter with default value\n- `{pagination}` or `{pagination:P:L}` - Pagination control\n- `{order_by:col(whitelist):dir}` - Dynamic sorting with whitelist validation\n- `{select}cols{endselect}` - Metadata block for total count\n- Arrays supported: `IN ({ids})` expands to `IN (?, ?, ?)`\n\n## API Parameters\n- `page` - Page number (requires {pagination} in query)\n- `per_page` - Items per page (requires {pagination} in query)\n- `order_by` - Column to sort by (requires {order_by} in query)\n- `order_direction` - Sort direction: `asc` or `desc` (requires {order_by} in query)\n\n## Response Fields\n- `data` - Array of result rows\n- `meta` - Pagination metadata (total, page, per_page, etc.)\n- `error` - Non-fatal error (e.g., COUNT query fails)\n- `debug_sql`, `debug_count_sql`, `debug_args` - Debug info (when DEBUG=true)\n\n## Reserved Parameter Names\nThe following names cannot be used as user-defined query parameters:\npage, per_page, order_by, order_direction",
		},
		"servers": []map[string]string{
			{"url": "http://localhost:8080"},
		},
		"paths": paths,
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"ApiKeyAuth": map[string]interface{}{
					"type": "apiKey",
					"in":   "header",
					"name": "X-API-Key",
				},
			},
		},
		"security": []map[string]interface{}{
			{
				"ApiKeyAuth": []string{},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(spec)
}
