package service

import (
	"context"
	"database/sql"
	"dbbridge/internal/core"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/alexbrainman/odbc"
)

type QueryExecutor struct {
	connRepo  core.ConnectionRepository
	queryRepo core.QueryRepository
	auditRepo core.AuditRepository
	cryptoSvc *EncryptionService
	parser    *core.SQLParser
}

func NewQueryExecutor(connRepo core.ConnectionRepository, queryRepo core.QueryRepository, auditRepo core.AuditRepository, cryptoSvc *EncryptionService) *QueryExecutor {
	return &QueryExecutor{
		connRepo:  connRepo,
		queryRepo: queryRepo,
		auditRepo: auditRepo,
		cryptoSvc: cryptoSvc,
		parser:    core.NewSQLParser(),
	}
}

type ExecutionResult struct {
	Columns []string                 `json:"columns"`
	Rows    []map[string]interface{} `json:"rows"`
}

func (e *QueryExecutor) Execute(ctx context.Context, connectionID int64, querySlug string, params map[string]interface{}) (result *ExecutionResult, err error) {
	// 3. Get Query Details
	queryDetails, err := e.queryRepo.GetBySlug(querySlug)
	if err != nil {
		return nil, fmt.Errorf("query not found: %w", err)
	}
	// queryID = queryDetails.ID // Capture for audit - Logic needs adjustment if we want to log QueryID.
	// For now, let's keep the audit log logic simple or refactor it too.
	// actually ExecuteSQL won't know about QueryID unless passed.
	// But Execute (by slug) knows it.

	// Let's defer audit inside Execute only, or pass QueryID to ExecuteSQL (optional).
	// To minimize changes, I'll keep the audit in Execute, and ExecuteSQL can have its own audit if called directly?
	// The request is for "Test Run", maybe we don't need strict auditing for test runs, or we do.
	// User didn't specify.

	return e.ExecuteSQL(ctx, connectionID, queryDetails.SQLText, params, queryDetails.ID)
}

func (e *QueryExecutor) ExecuteByName(ctx context.Context, connName string, querySlug string, params map[string]interface{}) (result *ExecutionResult, err error) {
	conn, err := e.connRepo.GetByName(connName)
	if err != nil {
		return nil, fmt.Errorf("connection not found: %w", err)
	}
	return e.Execute(ctx, conn.ID, querySlug, params)
}

// ExecuteSQL executes a raw SQL string against a connection
func (e *QueryExecutor) ExecuteSQL(ctx context.Context, connectionID int64, sqlText string, params map[string]interface{}, queryID int64) (result *ExecutionResult, err error) {
	startTime := time.Now()

	// Defer Audit Logging (Audit logs might be useful even for ad-hoc queries, usually QueryID=0)
	defer func() {
		duration := time.Since(startTime).Milliseconds()
		status := "SUCCESS"
		errMsg := ""
		if err != nil {
			status = "ERROR"
			errMsg = err.Error()
		}

		// TODO: UserID from context
		var userID int64 = 0
		var apiKeyID *int64 = nil

		if val := ctx.Value(core.ContextKeyApiKeyID); val != nil {
			if id, ok := val.(int64); ok {
				apiKeyID = &id
			}
		}

		// Serialize Params
		var paramsJSON string
		if len(params) > 0 {
			if b, err := json.Marshal(params); err == nil {
				paramsJSON = string(b)
			}
		}

		e.auditRepo.Create(&core.AuditLog{
			Timestamp:    startTime,
			UserID:       userID,
			ApiKeyID:     apiKeyID,
			ConnectionID: connectionID,
			QueryID:      queryID, // Use passed QueryID
			DurationMs:   duration,
			Status:       status,
			ErrorMessage: errMsg,
			Params:       paramsJSON,
		})
	}()

	// 1. Get Connection Details
	connDetails, err := e.connRepo.GetByID(connectionID)
	if err != nil {
		return nil, fmt.Errorf("connection not found: %w", err)
	}
	if !connDetails.IsActive {
		return nil, fmt.Errorf("connection is inactive")
	}

	// 2. Decrypt Password/Connection String
	decryptedConnStr, err := e.cryptoSvc.Decrypt(connDetails.ConnectionStringEnc)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt connection string: %w", err)
	}

	// 3. Parse SQL parameters
	// Check for system variables like {pagination}
	sqlText = e.processSystemVariables(sqlText, connDetails.Driver, params)

	// 3. Parse SQL parameters
	// Check for system variables like {pagination}
	sqlText = e.processSystemVariables(sqlText, connDetails.Driver, params)

	parseResult := e.parseSQL(sqlText, params)

	// 4. Build Parameter List
	args, err := e.parser.MapValues(parseResult.ParamNames, params, parseResult.Defaults)
	if err != nil {
		return nil, err
	}

	// 5. Connect to DB
	// TODO: Connection pooling
	db, err := sql.Open(connDetails.Driver, decryptedConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection (%s): %w", connDetails.Driver, err)
	}
	defer db.Close()

	// Check connection
	ctxTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctxTimeout); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 6. Execute Query
	rows, err := db.QueryContext(ctxTimeout, parseResult.SQL, args...)
	if err != nil {
		return nil, fmt.Errorf("execution error: %w", err)
	}
	defer rows.Close()

	// 7. Map Results
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	resultRows := []map[string]interface{}{}

	for rows.Next() {
		// Generic row scanning
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]

			// Handle []byte
			if b, ok := val.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = val
			}
		}
		resultRows = append(resultRows, rowMap)
	}

	return &ExecutionResult{
		Columns: columns,
		Rows:    resultRows,
	}, nil
}

// Helper to use the existing parser but returning the struct we need
func (e *QueryExecutor) parseSQL(sqlText string, params map[string]interface{}) *core.ParseResult {
	// Re-using the logic from param_parser.go
	return e.parser.Parse(sqlText, params)
}

func (e *QueryExecutor) processSystemVariables(sqlText string, driver string, params map[string]interface{}) string {
	// Regex to match {pagination}, {pagination:1:20}, {pagination::20}, {pagination:2:}
	// Case insensitive due to (?i)
	re := regexp.MustCompile(`(?i)\{\s*pagination(?::\s*(\d*)\s*:\s*(\d*)\s*)?\}`)

	// FindStringIndex returns the first match's indices
	loc := re.FindStringIndex(sqlText)
	if loc == nil {
		return sqlText
	}

	// Default pagination values (Global Default: 1:50)
	page := 1
	limit := 50

	// Check if the match contains custom defaults {pagination:P:L}
	// Note: FindStringSubmatch returns the text of the submatches
	match := re.FindStringSubmatch(sqlText)
	if len(match) == 3 {
		// match[1] is page (can be empty), match[2] is limit (can be empty)
		if match[1] != "" {
			if pVal, err := strconv.Atoi(match[1]); err == nil && pVal > 0 {
				page = pVal
			}
		}
		if match[2] != "" {
			if lVal, err := strconv.Atoi(match[2]); err == nil && lVal > 0 {
				limit = lVal
			}
		}
	}

	// Check params for _page and _limit (from UI or API) - These OVERRIDE everything
	if p, ok := params["_page"]; ok {
		if val, ok := p.(float64); ok { // JSON numbers are float64
			page = int(val)
		} else if val, ok := p.(int); ok {
			page = val
		} else if val, ok := p.(string); ok {
			if v, err := strconv.Atoi(val); err == nil {
				page = v
			}
		}
	}
	if l, ok := params["_limit"]; ok {
		if val, ok := l.(float64); ok {
			limit = int(val)
		} else if val, ok := l.(int); ok {
			limit = val
		} else if val, ok := l.(string); ok {
			if v, err := strconv.Atoi(val); err == nil {
				limit = v
			}
		}
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 1
	}

	offset := (page - 1) * limit
	replacement := ""

	switch driver {
	case "sqlite", "postgres":
		replacement = fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
	case "mysql":
		replacement = fmt.Sprintf("LIMIT %d, %d", offset, limit)
	case "odbc", "mssql":
		// Assuming SQL Anywhere / Sybase compatible syntax for ODBC
		replacement = fmt.Sprintf("TOP %d START AT %d", limit, offset+1)
	default:
		replacement = fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
	}

	// Replace only the first occurrence or all? User likely uses one pagination.
	// Provide full replacement of the matched tag.
	finalSQL := strings.Replace(sqlText, match[0], replacement, 1)
	return finalSQL
}
