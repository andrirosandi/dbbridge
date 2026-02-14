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
			// Use same regex as executor for consistency
			re := regexp.MustCompile(`(?i)\{\s*pagination(?::\s*(\d*)\s*:\s*(\d*)\s*)?\}`)
			if re.MatchString(q.SQLText) {
				hasPagination = true
			}

			for _, param := range parseRes.ParamNames {
				// Exclude system variables if any (though parser usually handles standard :param)
				// Our parser handles {param} too.
				// {pagination} should be ignored as it's a system variable handled via _page/_limit
				if strings.ToLower(param) == "pagination" {
					hasPagination = true
					continue
				}

				properties[param] = map[string]string{"type": "string"} // Default to string for simplicity
				exampleBody[param] = "value"
			}

			// Add Pagination params if {pagination} is present
			if hasPagination {
				properties["_page"] = map[string]interface{}{"type": "integer", "default": 1}
				properties["_limit"] = map[string]interface{}{"type": "integer", "default": 50}
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
										"rows": map[string]interface{}{
											"type": "array",
											"items": map[string]interface{}{
												"type": "object",
											},
										},
										"columns": map[string]interface{}{
											"type":  "array",
											"items": map[string]string{"type": "string"},
										},
									},
								},
							},
						},
					},
					"400": map[string]interface{}{
						"description": "Bad Request",
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
			"description": "Dynamic API generated from Saved Queries.",
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
