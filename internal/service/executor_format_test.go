package service

import (
	"dbbridge/internal/core"
	"testing"
)

func TestFormatSQL(t *testing.T) {
	executor := &QueryExecutor{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "replace param with default",
			input:    "SELECT * FROM users WHERE id = {id}",
			expected: "SELECT * FROM users WHERE id = ?",
		},
		{
			name:     "keep pagination as-is",
			input:    "SELECT * FROM users {pagination::20}",
			expected: "SELECT * FROM users {pagination::20}",
		},
		{
			name:     "keep order_by as-is",
			input:    "SELECT * FROM users {order_by:created_at()}",
			expected: "SELECT * FROM users {order_by:created_at()}",
		},
		{
			name:     "keep select/endselect as-is",
			input:    "{select}* FROM users{endselect}",
			expected: "{select}* FROM users{endselect}",
		},
		{
			name:     "replace multiple params",
			input:    "SELECT {select}id,name{endselect} FROM users WHERE id = {id} AND status = {status}",
			expected: "SELECT {select}id,name{endselect} FROM users WHERE id = ? AND status = ?",
		},
		{
			name:     "realistic query",
			input:    "SELECT {pagination::20} {select}itemid,description{endselect} FROM initem WHERE itemid LIKE {itemid:%}",
			expected: "SELECT {pagination::20} {select}itemid,description{endselect} FROM initem WHERE itemid LIKE ?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.formatSQL(tt.input)
			if result != tt.expected {
				t.Errorf("formatSQL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTwoStepFlow_Defaults(t *testing.T) {
	parser := core.NewSQLParser()

	tests := []struct {
		name             string
		sql              string
		params           map[string]interface{}
		expectedParams   []string
		expectedDefaults map[string]interface{}
		expectedArgs     []interface{}
	}{
		{
			name:             "percent default in LIKE",
			sql:              "SELECT * FROM initem WHERE itemid LIKE {itemid:%}",
			params:           map[string]interface{}{},
			expectedParams:   []string{"itemid"},
			expectedDefaults: map[string]interface{}{"itemid": "%"},
			expectedArgs:     []interface{}{"%"},
		},
		{
			name:             "percent default overridden by param",
			sql:              "SELECT * FROM initem WHERE itemid LIKE {itemid:%}",
			params:           map[string]interface{}{"itemid": "ABC%"},
			expectedParams:   []string{"itemid"},
			expectedDefaults: map[string]interface{}{"itemid": "%"},
			expectedArgs:     []interface{}{"ABC%"},
		},
		{
			name:             "mixed params with defaults",
			sql:              "SELECT * FROM users WHERE status = {status:active} AND name LIKE {name:%}",
			params:           map[string]interface{}{},
			expectedParams:   []string{"status", "name"},
			expectedDefaults: map[string]interface{}{"status": "active", "name": "%"},
			expectedArgs:     []interface{}{"active", "%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parseResult := parser.Parse(tt.sql, tt.params)

			if len(parseResult.ParamNames) != len(tt.expectedParams) {
				t.Errorf("ParamNames = %v, want %v", parseResult.ParamNames, tt.expectedParams)
			}

			for k, v := range tt.expectedDefaults {
				if parseResult.Defaults[k] != v {
					t.Errorf("Defaults[%s] = %v, want %v", k, parseResult.Defaults[k], v)
				}
			}

			args, err := parser.MapValues(parseResult.ParamNames, tt.params, parseResult.Defaults, parseResult.RawDefaults)
			if err != nil {
				t.Errorf("MapValues error: %v", err)
				return
			}

			if len(args) != len(tt.expectedArgs) {
				t.Errorf("args = %v, want %v", args, tt.expectedArgs)
				return
			}

			for i, expected := range tt.expectedArgs {
				if args[i] != expected {
					t.Errorf("args[%d] = %v, want %v", i, args[i], expected)
				}
			}
		})
	}
}
