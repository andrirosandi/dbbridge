package service

import (
	"testing"
)

func TestProcessOrderBy_DefaultValues(t *testing.T) {
	executor := &QueryExecutor{}

	tests := []struct {
		name     string
		sql      string
		params   map[string]interface{}
		expected string
	}{
		{
			name:     "no order_by pattern",
			sql:      "SELECT * FROM users WHERE id = {id}",
			params:   map[string]interface{}{"id": 1},
			expected: "SELECT * FROM users WHERE id = {id}",
		},
		{
			name:     "use default values when no params - use default direction from query",
			sql:      "SELECT * FROM orders {order_by:trdate(id,trdate,amount):desc}",
			params:   map[string]interface{}{},
			expected: "SELECT * FROM orders ORDER BY trdate DESC",
		},
		{
			name:     "override column - keep default direction from query",
			sql:      "SELECT * FROM orders {order_by:trdate(id,trdate,amount):desc}",
			params:   map[string]interface{}{"order_by": "amount"},
			expected: "SELECT * FROM orders ORDER BY amount DESC",
		},
		{
			name:     "override direction with valid param",
			sql:      "SELECT * FROM orders {order_by:trdate(id,trdate,amount):asc}",
			params:   map[string]interface{}{"order_direction": "desc"},
			expected: "SELECT * FROM orders ORDER BY trdate DESC",
		},
		{
			name:     "override column - keep default direction from query",
			sql:      "SELECT * FROM orders {order_by:trdate(id,trdate,amount):desc}",
			params:   map[string]interface{}{"order_by": "amount"},
			expected: "SELECT * FROM orders ORDER BY amount DESC",
		},
		{
			name:     "override direction with valid param",
			sql:      "SELECT * FROM orders {order_by:trdate(id,trdate,amount):desc}",
			params:   map[string]interface{}{"order_direction": "desc"},
			expected: "SELECT * FROM orders ORDER BY trdate DESC",
		},
		{
			name:     "override both column and direction",
			sql:      "SELECT * FROM orders {order_by:trdate(id,trdate,amount):desc}",
			params:   map[string]interface{}{"order_by": "id", "order_direction": "asc"},
			expected: "SELECT * FROM orders ORDER BY id ASC",
		},
		{
			name:     "invalid column falls back to default",
			sql:      "SELECT * FROM orders {order_by:trdate(id,trdate,amount):desc}",
			params:   map[string]interface{}{"order_by": "invalid_col"},
			expected: "SELECT * FROM orders ORDER BY trdate DESC",
		},
		{
			name:     "invalid direction falls back to default from query",
			sql:      "SELECT * FROM orders {order_by:trdate(id,trdate,amount):desc}",
			params:   map[string]interface{}{"order_direction": "invalid"},
			expected: "SELECT * FROM orders ORDER BY trdate DESC",
		},
		{
			name:     "case insensitive whitelist match",
			sql:      "SELECT * FROM orders {order_by:trdate(id,TRDATE,amount):desc}",
			params:   map[string]interface{}{"order_by": "trdate"},
			expected: "SELECT * FROM orders ORDER BY trdate DESC",
		},
		{
			name:     "with other patterns present",
			sql:      "SELECT * FROM orders WHERE status = {status} {order_by:trdate(id,trdate):desc} {pagination:1:20}",
			params:   map[string]interface{}{"order_by": "id"},
			expected: "SELECT * FROM orders WHERE status = {status} ORDER BY id DESC {pagination:1:20}",
		},
		{
			name:     "empty whitelist - auto add default column",
			sql:      "SELECT * FROM orders {order_by:created_at():asc}",
			params:   map[string]interface{}{},
			expected: "SELECT * FROM orders ORDER BY created_at ASC",
		},
		{
			name:     "empty whitelist - use default direction",
			sql:      "SELECT * FROM orders {order_by:updated_at():desc}",
			params:   map[string]interface{}{},
			expected: "SELECT * FROM orders ORDER BY updated_at DESC",
		},
		{
			name:     "empty whitelist - override default column",
			sql:      "SELECT * FROM orders {order_by:created_at():asc}",
			params:   map[string]interface{}{"order_by": "created_at", "order_direction": "desc"},
			expected: "SELECT * FROM orders ORDER BY created_at DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.processOrderBy(tt.sql, tt.params)
			if result != tt.expected {
				t.Errorf("processOrderBy() = %q, want %q", result, tt.expected)
			}
		})
	}
}
