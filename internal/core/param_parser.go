package core

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// SQLParser handles parsing of named parameters {var} to positional parameters ?
type SQLParser struct {
	regex *regexp.Regexp
}

func NewSQLParser() *SQLParser {
	// Matches {varname} or { varname } or {var:default}
	// Note: We use a broader regex to capture content inside {}, then split manually in ReplaceFunc logic for simplicity
	return &SQLParser{
		regex: regexp.MustCompile(`\{\s*([^}]+)\s*\}`),
	}
}

// ParseResult contains the transformed SQL, the list of parameter names, and default values
type ParseResult struct {
	SQL        string
	ParamNames []string
	Defaults   map[string]interface{}
}

// Parse takes SQL text and optional values. If values are provided, it detects arrays/slices
// and expands placeholders (? -> ?, ?, ?) accordingly.
func (p *SQLParser) Parse(sqlText string, values map[string]interface{}) *ParseResult {
	paramNames := []string{}
	defaults := make(map[string]interface{})

	// Replace all occurrences of {var} or {var:default} with ?
	transformedSQL := p.regex.ReplaceAllStringFunc(sqlText, func(match string) string {
		// match is like "{id}" or "{status:active}"
		// Remove { and }
		content := match[1 : len(match)-1]

		parts := strings.SplitN(content, ":", 2)
		paramName := strings.TrimSpace(parts[0])

		if len(parts) > 1 {
			defVal := strings.TrimSpace(parts[1])
			defaults[paramName] = defVal
		}

		// Array Expansion Logic
		if values != nil {
			if val, ok := values[paramName]; ok {
				v := reflect.ValueOf(val)
				var arrayVal reflect.Value
				isSlice := false

				if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
					arrayVal = v
					isSlice = true
				} else if v.Kind() == reflect.String {
					// Check if string looks like JSON array [ ... ]
					strVal := strings.TrimSpace(v.String())
					if strings.HasPrefix(strVal, "[") && strings.HasSuffix(strVal, "]") {
						var parsed []interface{}
						if err := json.Unmarshal([]byte(strVal), &parsed); err == nil {
							arrayVal = reflect.ValueOf(parsed)
							isSlice = true
							// Update values map so MapValues uses the parsed slice later!
							// Note: This modifies the map 'values' which is a reference type, good.
							values[paramName] = parsed
						}
					}
				}

				if isSlice {
					// It's an array/slice!
					length := arrayVal.Len()
					if length == 0 {
						// "IN (NULL)" is safe.
						return "NULL"
					}

					placeholders := make([]string, length)
					for i := 0; i < length; i++ {
						placeholders[i] = "?"
						// Encode index in paramName
						paramNames = append(paramNames, fmt.Sprintf("%s:%d", paramName, i))
					}
					return strings.Join(placeholders, ", ")
				}
			}
		}

		paramNames = append(paramNames, paramName)
		return "?"
	})

	return &ParseResult{
		SQL:        transformedSQL,
		ParamNames: paramNames,
		Defaults:   defaults,
	}
}

// MapValues takes param names, values, and defaults to build argument list
// MapValues takes param names, values, and defaults to build argument list
func (p *SQLParser) MapValues(paramNames []string, values map[string]interface{}, defaults map[string]interface{}) ([]interface{}, error) {
	result := make([]interface{}, len(paramNames))
	missing := []string{}

	for i, name := range paramNames {
		// Check for indexed name "name:index" (used for array expansion)
		if strings.Contains(name, ":") {
			parts := strings.SplitN(name, ":", 2)
			realName := parts[0]
			idxStr := parts[1]

			if idx, err := strconv.Atoi(idxStr); err == nil {
				// It's an indexed param
				val, ok := values[realName]
				if !ok {
					missing = append(missing, realName) // Should not happen if Parse saw it, but safer
					continue
				}
				v := reflect.ValueOf(val)
				if (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) && idx < v.Len() {
					result[i] = v.Index(idx).Interface()
					continue
				}
			}
		}

		val, ok := values[name]
		if !ok {
			// Try default
			if def, hasDef := defaults[name]; hasDef {
				result[i] = def
				continue
			}
			missing = append(missing, name)
			continue
		}
		result[i] = val
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing parameters: %s", strings.Join(missing, ", "))
	}

	return result, nil
}
