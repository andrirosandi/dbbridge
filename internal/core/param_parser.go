package core

import (
	"fmt"
	"regexp"
	"strings"
)

// SQLParser handles parsing of named parameters {var} to positional parameters ?
type SQLParser struct {
	regex *regexp.Regexp
}

func NewSQLParser() *SQLParser {
	// Matches {varname} where varname is alphanumeric
	return &SQLParser{
		regex: regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`),
	}
}

// ParseResult contains the transformed SQL and the list of parameter names in order
type ParseResult struct {
	SQL        string
	ParamNames []string
}

func (p *SQLParser) Parse(sqlText string) *ParseResult {
	paramNames := []string{}

	// Replace all occurrences of {var} with ? and track the var names
	transformedSQL := p.regex.ReplaceAllStringFunc(sqlText, func(match string) string {
		// match is like "{customer_id}"
		// extract "customer_id" (remove first and last char)
		paramName := match[1 : len(match)-1]
		paramNames = append(paramNames, paramName)
		return "?"
	})

	return &ParseResult{
		SQL:        transformedSQL,
		ParamNames: paramNames,
	}
}

// MapValues takes the list of param names and a map of values, returning the slice of values in order
func (p *SQLParser) MapValues(paramNames []string, values map[string]interface{}) ([]interface{}, error) {
	result := make([]interface{}, len(paramNames))
	missing := []string{}

	for i, name := range paramNames {
		val, ok := values[name]
		if !ok {
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
