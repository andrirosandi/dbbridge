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

	return e.ExecuteSQL(ctx, connectionID, queryDetails.SQLText, params)
}

// ExecuteSQL executes a raw SQL string against a connection
func (e *QueryExecutor) ExecuteSQL(ctx context.Context, connectionID int64, sqlText string, params map[string]interface{}) (result *ExecutionResult, err error) {
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

		e.auditRepo.Create(&core.AuditLog{
			Timestamp:    startTime,
			UserID:       userID,
			ConnectionID: connectionID,
			QueryID:      0, // Ad-hoc / Test run
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

	// 3. Parse SQL parameters
	parseResult := e.parseSQL(sqlText)

	// 4. Build Parameter List
	args, err := e.parser.MapValues(parseResult.ParamNames, params)
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
func (e *QueryExecutor) parseSQL(sqlText string) *core.ParseResult {
	// Re-using the logic from param_parser.go
	// In a real scenario we might want to attach the method to the struct directly or use the one we imported
	return e.parser.Parse(sqlText)
}
