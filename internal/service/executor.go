package service

import (
	"context"
	"database/sql"
	"dbbridge/internal/core"
	"dbbridge/internal/logger"
	"encoding/json"
	"fmt"
	"os"
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

type MetaInfo struct {
	Columns    []string `json:"columns,omitempty"`
	Total      *int64   `json:"total,omitempty"`
	Page       *int     `json:"page,omitempty"`
	PerPage    *int     `json:"per_page,omitempty"`
	TotalPages *int     `json:"total_pages,omitempty"`
	HasNext    *bool    `json:"has_next,omitempty"`
	HasPrev    *bool    `json:"has_prev,omitempty"`
	NextPage   *int     `json:"next_page,omitempty"`
	PrevPage   *int     `json:"prev_page,omitempty"`
}

type ExecutionResult struct {
	Data       []map[string]interface{} `json:"data"`
	Meta       MetaInfo                 `json:"meta,omitempty"`
	Error      string                   `json:"error,omitempty"`
	DebugSQL   string                   `json:"debug_sql,omitempty"`
	DebugCount string                   `json:"debug_count_sql,omitempty"`
	DebugArgs  interface{}              `json:"debug_args,omitempty"`
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

	// Defer Audit Logging

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

	// STEP 1: Parse original SQL to extract paramNames and defaults
	// (This must happen BEFORE formatSQL removes the {param} patterns)
	parseResult := e.parseSQL(sqlText, params)

	// STEP 2: Generate formatted query using parseResult.SQL (which has raw values already replaced)
	formattedSQL := e.formatSQL(parseResult.SQL)

	// STEP 3: Generate COUNT query BEFORE pagination is applied
	// This is important because COUNT should NOT include pagination limits
	// COUNT query: remove {pagination} entirely, remove {order_by}, keep everything else
	countQueryBase := formattedSQL
	countQueryBase = regexp.MustCompile(`(?i)\{\s*pagination(?::\s*\d*\s*:\s*\d*\s*)?\}`).ReplaceAllString(countQueryBase, "")
	countQueryBase = regexp.MustCompile(`(?i)\{\s*order_by:[^}]+\}`).ReplaceAllString(countQueryBase, "")

	// Generate Main & Count from this base (before pagination)
	countSelectBlock := e.processSelectBlock(countQueryBase)
	countSQL := countSelectBlock.CountSQL

	// STEP 4: Process pagination & order_by on formatted query for MAIN query
	formattedSQL, page, limit := e.processSystemVariables(formattedSQL, connDetails.Driver, params, decryptedConnStr)
	formattedSQL = e.processOrderBy(formattedSQL, params)

	// Generate Main SQL from the paginated version
	selectBlock := e.processSelectBlock(formattedSQL)
	// Use the COUNT SQL generated before pagination (this is the correct COUNT without limits)
	selectBlock.CountSQL = countSQL

	// STEP 5: Generate exec SQL - replace remaining {param} with ? in the final SQL
	// Use selectBlock.SQLWithout which has actual column names, not {select}...{endselect}
	execSQL := e.formatSQL(selectBlock.SQLWithout)

	// STEP 6: Build Parameter List using the paramNames and defaults from STEP 1
	var args []interface{}
	args, err = e.parser.MapValues(parseResult.ParamNames, params, parseResult.Defaults, parseResult.RawDefaults)
	if err != nil {
		return nil, err
	}

	// 7. Connect to DB
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

	// 8. Execute Query
	// Special handling for Sybase/SQL Anywhere: batch with params not supported
	isSybaseBatch := strings.Contains(strings.ToLower(connDetails.Driver), "sql anywhere") ||
		strings.Contains(strings.ToLower(connDetails.Driver), "sybase")
	hasParams := len(args) > 0
	isBatch := strings.Contains(strings.ToLower(execSQL), "begin")

	// For Sybase batch with params, try to execute differently
	var rows *sql.Rows
	if isSybaseBatch && hasParams && isBatch {
		// Try removing BEGIN-END for execution
		singleSQL := execSQL
		singleSQL = regexp.MustCompile(`(?i)^\s*BEGIN\s*`).ReplaceAllString(singleSQL, "")
		singleSQL = regexp.MustCompile(`(?i)\s*END\s*$`).ReplaceAllString(singleSQL, "")
		singleSQL = strings.TrimSpace(singleSQL)

		logger.Info.Printf("[DEBUG] Sybase batch with params - trying without batch wrapper")
		logger.Info.Printf("[DEBUG] Single SQL: %s", singleSQL)

		rows, err = db.QueryContext(ctxTimeout, singleSQL, args...)
	} else {
		rows, err = db.QueryContext(ctxTimeout, execSQL, args...)
	}

	if err != nil {
		errMsg := fmt.Sprintf("execution error: %v\nDEBUG params: %v", err, params)
		if os.Getenv("DEBUG") == "true" {
			errMsg = fmt.Sprintf("%s\n\nSQL: %s\nArgs: %v", errMsg, execSQL, args)
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	defer rows.Close()

	// 9. Map Results
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

	// 10. Build metadata (only columns if no select block)
	meta := MetaInfo{
		Columns: columns,
	}

	// 12. Execute COUNT query if {select}{endselect} block exists
	var execError string

	if selectBlock.HasBlock && selectBlock.Error == "" {
		countSQL := selectBlock.CountSQL

		var total int64 = 0
		var countErr error = nil

		if selectBlock.SelectContent != "" {
			var countArgs []interface{}
			if strings.Contains(countSQL, "?") {
				countArgs = args
			}

			countRows, err := db.QueryContext(ctxTimeout, countSQL, countArgs...)
			if err != nil {
				countErr = err
			} else {
				defer countRows.Close()
				if countRows.Next() {
					countRows.Scan(&total)
				}
			}
		}

		if countErr != nil {
			execError = countErr.Error()
		} else {
			pagePtr := &page
			limitPtr := &limit
			totalPages := int(total) / limit
			if int(total)%limit > 0 {
				totalPages++
			}
			totalPagesPtr := &totalPages
			hasNext := page < totalPages
			hasPrev := page > 1

			meta.Page = pagePtr
			meta.PerPage = limitPtr
			meta.Total = &total
			meta.TotalPages = totalPagesPtr
			meta.HasNext = &hasNext
			meta.HasPrev = &hasPrev

			if hasNext {
				np := page + 1
				meta.NextPage = &np
			}
			if hasPrev {
				pp := page - 1
				meta.PrevPage = &pp
			}
		}
	}

	var execResult *ExecutionResult
	if os.Getenv("DEBUG") == "true" {
		// Replace special chars for JSON safety
		escapeJSON := func(s string) string {
			s = strings.ReplaceAll(s, "\\", "\\\\")
			s = strings.ReplaceAll(s, "\"", "\\\"")
			s = strings.ReplaceAll(s, "\n", "\\n")
			s = strings.ReplaceAll(s, "\r", "\\r")
			s = strings.ReplaceAll(s, "\t", "\\t")
			return s
		}

		execResult = &ExecutionResult{
			Data:       resultRows,
			Meta:       meta,
			Error:      execError,
			DebugSQL:   escapeJSON(execSQL),
			DebugCount: escapeJSON(selectBlock.CountSQL),
			DebugArgs:  args,
		}
	} else {
		execResult = &ExecutionResult{
			Data:  resultRows,
			Meta:  meta,
			Error: execError,
		}
	}

	return execResult, nil
}

// Helper to use the existing parser but returning the struct we need
func (e *QueryExecutor) parseSQL(sqlText string, params map[string]interface{}) *core.ParseResult {
	// Re-using the logic from param_parser.go
	return e.parser.Parse(sqlText, params)
}

type SelectBlockResult struct {
	HasBlock      bool
	SQLWithout    string
	CountSQL      string
	SelectContent string
	Error         string
}

func (e *QueryExecutor) formatSQL(sqlText string) string {
	re := regexp.MustCompile(`\{\s*([a-zA-Z_][a-zA-Z0-9_]*)(:[^}]*)?\}`)

	formatted := re.ReplaceAllStringFunc(sqlText, func(match string) string {
		content := match[1 : len(match)-1]
		parts := strings.SplitN(content, ":", 2)
		paramName := strings.TrimSpace(parts[0])

		lower := strings.ToLower(paramName)
		if lower == "pagination" || lower == "select" || lower == "endselect" || lower == "order_by" {
			return match
		}

		return "?"
	})

	return formatted
}

func (e *QueryExecutor) processSelectBlock(sqlText string) *SelectBlockResult {
	// (?s) flag makes . match newlines (dotall mode)
	reBlock := regexp.MustCompile(`(?si)\{select\}(.*?)\{endselect\}`)

	match := reBlock.FindStringIndex(sqlText)
	if match == nil {
		return &SelectBlockResult{
			HasBlock:   false,
			SQLWithout: sqlText,
		}
	}

	matchContent := reBlock.FindStringSubmatch(sqlText)
	if matchContent == nil || len(matchContent) < 2 {
		return &SelectBlockResult{
			HasBlock:   true,
			SQLWithout: sqlText,
			CountSQL:   sqlText,
			Error:      "Invalid {select}{endselect} block",
		}
	}

	selectContent := strings.TrimSpace(matchContent[1])

	// Main query: replace {select}...{endselect} with actual column names
	mainSQL := reBlock.ReplaceAllString(sqlText, selectContent)

	// COUNT query: replace {select}...{endselect} with COUNT(*)
	countSQL := reBlock.ReplaceAllString(sqlText, "COUNT(*)")

	// Remove {order_by:...} pattern from COUNT query
	countSQL = regexp.MustCompile(`(?i)\s*\{order_by:[^}]+\}`).ReplaceAllString(countSQL, " ")

	// Remove {pagination} pattern from COUNT query
	countSQL = regexp.MustCompile(`(?i)\s*\{pagination(?::\s*\d*\s*:\s*\d*\s*)?\}`).ReplaceAllString(countSQL, "")

	return &SelectBlockResult{
		HasBlock:      true,
		SQLWithout:    mainSQL,
		CountSQL:      countSQL,
		SelectContent: selectContent,
	}
}

func (e *QueryExecutor) buildMetadata(page, limit int, total int64) MetaInfo {
	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}

	hasNext := page < totalPages
	hasPrev := page > 1

	pagePtr := &page
	limitPtr := &limit
	totalPtr := &total
	totalPagesPtr := &totalPages
	hasNextPtr := &hasNext
	hasPrevPtr := &hasPrev

	var nextPage, prevPage *int
	if hasNext {
		np := page + 1
		nextPage = &np
	}
	if hasPrev {
		pp := page - 1
		prevPage = &pp
	}

	return MetaInfo{
		Page:       pagePtr,
		PerPage:    limitPtr,
		Total:      totalPtr,
		TotalPages: totalPagesPtr,
		HasNext:    hasNextPtr,
		HasPrev:    hasPrevPtr,
		NextPage:   nextPage,
		PrevPage:   prevPage,
	}
}

func (e *QueryExecutor) processOrderBy(sqlText string, params map[string]interface{}) string {
	// Regex to match:
	// Simple: {order_by:columnname}
	// Full: {order_by:defaultColumn(whitelist1,whitelist2,...):defaultDirection}
	re := regexp.MustCompile(`(?i)\{\s*order_by\s*:\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*(\([^)]*\))?\s*(:\s*(asc|desc))?\s*\}`)

	loc := re.FindStringIndex(sqlText)
	if loc == nil {
		return sqlText
	}

	match := re.FindStringSubmatch(sqlText)
	if len(match) < 2 {
		return sqlText
	}

	defaultColumn := match[1]
	whitelistStr := ""
	defaultDirection := "ASC"

	if len(match) >= 3 && match[2] != "" {
		whitelistStr = match[2]
	}
	if len(match) >= 5 && match[4] != "" {
		defaultDirection = strings.ToUpper(match[4])
	}

	whitelist := make(map[string]bool)
	if whitelistStr != "" {
		for _, col := range strings.Split(whitelistStr[1:len(whitelistStr)-1], ",") {
			col = strings.TrimSpace(col)
			if col != "" {
				whitelist[strings.ToLower(col)] = true
			}
		}
	}

	if len(whitelist) == 0 && defaultColumn != "" {
		whitelist[strings.ToLower(defaultColumn)] = true
	}

	if len(whitelist) == 0 {
		return sqlText
	}

	column := defaultColumn
	direction := defaultDirection

	if orderBy, ok := params["order_by"]; ok {
		if col, ok := orderBy.(string); ok {
			col = strings.TrimSpace(col)
			if col != "" && whitelist[strings.ToLower(col)] {
				column = col
			}
		}
	}

	if orderDir, ok := params["order_direction"]; ok {
		if dir, ok := orderDir.(string); ok {
			dir = strings.ToLower(strings.TrimSpace(dir))
			if dir == "asc" || dir == "desc" {
				direction = strings.ToUpper(dir)
			}
		}
	}

	replacement := fmt.Sprintf("ORDER BY %s %s", column, direction)
	finalSQL := strings.Replace(sqlText, match[0], replacement, 1)
	return finalSQL
}

func (e *QueryExecutor) processSystemVariables(sqlText string, driver string, params map[string]interface{}, connStr string) (string, int, int) {
	// Regex to match {pagination}, {pagination:1:20}, {pagination::20}, {pagination:2:}
	// Case insensitive due to (?i)
	re := regexp.MustCompile(`(?i)\{\s*pagination(?::\s*(\d*)\s*:\s*(\d*)\s*)?\}`)

	// FindStringIndex returns the first match's indices
	loc := re.FindStringIndex(sqlText)
	if loc == nil {
		return sqlText, 1, 50
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

	// Check params for page and per_page (from UI or API) - These OVERRIDE everything
	if p, ok := params["page"]; ok {
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
	if l, ok := params["per_page"]; ok {
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
		// Detect if it is Sybase / SQL Anywhere based on Connection String
		// SQL Anywhere drivers usually contain "SQL Anywhere" or "ASA"
		isSybase := strings.Contains(strings.ToLower(connStr), "sql anywhere") ||
			strings.Contains(strings.ToLower(connStr), "asa")

		if isSybase {
			replacement = fmt.Sprintf("TOP %d START AT %d", limit, offset+1)
		} else {
			// Default to LIMIT OFFSET for other ODBC (Postgres, MySQL, etc.)
			// Assuming most modern SQL/ODBC drivers support LIMIT/OFFSET or similar enough
			// If not, we might need more specific detection
			replacement = fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
		}
	default:
		replacement = fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
	}

	// Replace only the first occurrence or all? User likely uses one pagination.
	// Provide full replacement of the matched tag.
	finalSQL := strings.Replace(sqlText, match[0], replacement, 1)
	return finalSQL, page, limit
}
