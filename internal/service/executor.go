package service

import (
	"context"
	"database/sql"
	"dbbridge/internal/core"
	"fmt"
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
	startTime := time.Now()
	var queryID int64

	// Defer Audit Logging
	defer func() {
		duration := time.Since(startTime).Milliseconds()
		status := "SUCCESS"
		errMsg := ""
		if err != nil {
			status = "ERROR"
			errMsg = err.Error()
		}

		// TODO: Extract UserID from context if available
		// userID := ctx.Value("user_id").(int64) or similar
		var userID int64 = 0 // System/Anonymous for now

		e.auditRepo.Create(&core.AuditLog{
			Timestamp:    startTime,
			UserID:       userID,
			ConnectionID: connectionID,
			QueryID:      queryID,
			DurationMs:   duration,
			Status:       status,
			ErrorMessage: errMsg,
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

	// 3. Get Query Details
	queryDetails, err := e.queryRepo.GetBySlug(querySlug)
	if err != nil {
		return nil, fmt.Errorf("query not found: %w", err)
	}
	queryID = queryDetails.ID // Capture for audit
	if !queryDetails.IsActive {
		return nil, fmt.Errorf("query is inactive")
	}

	// 4. Parse SQL parameters
	// Re-parse every time to ensure dynamic behavior (could be cached in future)
	parseResult := e.parseSQL(queryDetails.SQLText)

	// 5. Build Parameter List
	args, err := e.parser.MapValues(parseResult.ParamNames, params)
	if err != nil {
		return nil, err
	}

	// 6. Connect to ODBC
	// TODO: Connection pooling could be implemented here by caching *sql.DB objects
	db, err := sql.Open("odbc", decryptedConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open odbc connection: %w", err)
	}
	defer db.Close()

	// Check connection
	ctxTimeout, cancel := context.WithTimeout(ctx, 30*time.Second) // 30s timeout
	defer cancel()

	if err := db.PingContext(ctxTimeout); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 7. Execute Query
	rows, err := db.QueryContext(ctxTimeout, parseResult.SQL, args...)
	if err != nil {
		return nil, fmt.Errorf("execution error: %w", err)
	}
	defer rows.Close()

	// 8. Map Results to JSON-friendly format
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

			// Handle []byte (common in drivers for strings/blobs)
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
func (e *QueryExecutor) parseSQL(sqlText string) *core.ParseResult {
	// Re-using the logic from param_parser.go
	// In a real scenario we might want to attach the method to the struct directly or use the one we imported
	return e.parser.Parse(sqlText)
}
