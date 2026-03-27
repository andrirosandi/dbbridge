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
	return &SQLParser{
		regex: regexp.MustCompile(`\{\s*(?:raw\|(\w+)|(\w+)(?::raw\|([^}]+))?|(\w+):([^}]+))?\s*\}`),
	}
}

// ParseResult contains the transformed SQL, the list of parameter names, and default values
type ParseResult struct {
	SQL         string
	ParamNames  []string
	Defaults    map[string]interface{}
	RawDefaults map[string]string
}

// Parse takes SQL text and optional values. If values are provided, it detects arrays/slices
// and expands placeholders (? -> ?, ?, ?) accordingly.
func (p *SQLParser) Parse(sqlText string, values map[string]interface{}) *ParseResult {
	paramNames := []string{}
	defaults := make(map[string]interface{})
	rawDefaults := make(map[string]string)

	// System variables to exclude from parameter parsing
	systemVars := map[string]bool{
		"select":     true,
		"endselect":  true,
		"order_by":   true,
		"pagination": true,
	}

	// Replace all occurrences of {var} or {var:default} with ?
	transformedSQL := p.regex.ReplaceAllStringFunc(sqlText, func(match string) string {
		// match is like "{id}", "{status:active}", "{raw|param}", or "{param:raw|default}"
		// Remove { and }
		content := match[1 : len(match)-1]

		// Check which pattern matched
		// Pattern 1: raw|param
		if strings.HasPrefix(content, "raw|") {
			paramName := strings.TrimSpace(strings.TrimPrefix(content, "raw|"))
			if systemVars[strings.ToLower(paramName)] {
				return match
			}
			// {raw|param} - replace langsung dengan value (tanpa placeholder)
			if values != nil {
				if val, ok := values[paramName]; ok {
					return fmt.Sprintf("%v", val)
				}
			}
			// Param tidak ada - return match untuk error handling di MapValues
			return match
		}

		// Pattern 2: param:raw|default
		if strings.Contains(content, ":raw|") {
			parts := strings.SplitN(content, ":raw|", 2)
			paramName := strings.TrimSpace(parts[0])
			rawDefault := strings.TrimSpace(parts[1])

			if systemVars[strings.ToLower(paramName)] {
				return match
			}

			// Simpan raw default
			rawDefaults[paramName] = rawDefault

			// Cek apakah param ada di values
			if values != nil {
				if _, ok := values[paramName]; ok {
					// Param ada - gunakan placeholder normal
					paramNames = append(paramNames, paramName)
					return "?"
				}
			}
			// Param tidak ada - replace dengan raw default
			return rawDefault
		}

		// Pattern 3: param:default (string default)
		if strings.Contains(content, ":") {
			parts := strings.SplitN(content, ":", 2)
			paramName := strings.TrimSpace(parts[0])
			defVal := strings.TrimSpace(parts[1])

			if systemVars[strings.ToLower(paramName)] {
				return match
			}

			defaults[paramName] = defVal

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
						strVal := strings.TrimSpace(v.String())
						if strings.HasPrefix(strVal, "[") && strings.HasSuffix(strVal, "]") {
							var parsed []interface{}
							if err := json.Unmarshal([]byte(strVal), &parsed); err == nil {
								arrayVal = reflect.ValueOf(parsed)
								isSlice = true
								values[paramName] = parsed
							}
						}
					}

					if isSlice {
						length := arrayVal.Len()
						if length == 0 {
							return "NULL"
						}

						placeholders := make([]string, length)
						for i := 0; i < length; i++ {
							placeholders[i] = "?"
							paramNames = append(paramNames, fmt.Sprintf("%s:%d", paramName, i))
						}
						return strings.Join(placeholders, ", ")
					}
				}
			}

			paramNames = append(paramNames, paramName)
			return "?"
		}

		// Pattern 4: {param} - wajib
		paramName := strings.TrimSpace(content)
		if systemVars[strings.ToLower(paramName)] {
			return match
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
					strVal := strings.TrimSpace(v.String())
					if strings.HasPrefix(strVal, "[") && strings.HasSuffix(strVal, "]") {
						var parsed []interface{}
						if err := json.Unmarshal([]byte(strVal), &parsed); err == nil {
							arrayVal = reflect.ValueOf(parsed)
							isSlice = true
							values[paramName] = parsed
						}
					}
				}

				if isSlice {
					length := arrayVal.Len()
					if length == 0 {
						return "NULL"
					}

					placeholders := make([]string, length)
					for i := 0; i < length; i++ {
						placeholders[i] = "?"
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
		SQL:         transformedSQL,
		ParamNames:  paramNames,
		Defaults:    defaults,
		RawDefaults: rawDefaults,
	}
}

// MapValues takes param names, values, defaults, and raw defaults to build argument list
func (p *SQLParser) MapValues(paramNames []string, values map[string]interface{}, defaults map[string]interface{}, rawDefaults map[string]string) ([]interface{}, error) {
	result := []interface{}{}
	missing := []string{}

	for _, name := range paramNames {
		// Check if param is in rawDefaults AND NOT provided by user
		// If user provided the param, use their value even if it has rawDefault
		if _, isRaw := rawDefaults[name]; isRaw {
			if val, userProvided := values[name]; userProvided {
				// User provided this param, use their value
				result = append(result, val)
				continue
			}
			// User did NOT provide, raw default was already embedded in SQL
			continue
		}

		// Check for indexed name "name:index" (used for array expansion)
		if strings.Contains(name, ":") {
			parts := strings.SplitN(name, ":", 2)
			realName := parts[0]
			idxStr := parts[1]

			if idx, err := strconv.Atoi(idxStr); err == nil {
				// It's an indexed param
				val, ok := values[realName]
				if !ok {
					missing = append(missing, realName)
					continue
				}
				v := reflect.ValueOf(val)
				if (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) && idx < v.Len() {
					result = append(result, v.Index(idx).Interface())
					continue
				}
			}
		}

		val, ok := values[name]
		if !ok {
			// Try default
			if def, hasDef := defaults[name]; hasDef {
				result = append(result, def)
				continue
			}
			missing = append(missing, name)
			continue
		}
		result = append(result, val)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing parameters: %s", strings.Join(missing, ", "))
	}

	return result, nil
}
