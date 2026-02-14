package data

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// InitDB initializes the SQLite database and runs migrations
func InitDB() (*sql.DB, error) {
	// Determine database path execution relative
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(filepath.Dir(exePath), "dbbridge.db")

	// If running with "go run", exe is in temp, so fallback to current dir for dev
	if filepath.Base(filepath.Dir(exePath)) != "dbbridge" && filepath.Base(filepath.Dir(exePath)) != "build" {
		wd, _ := os.Getwd()
		dbPath = filepath.Join(wd, "dbbridge.db")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	if err := runMigrations(db); err != nil {
		return nil, err
	}

	// Query Links
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS query_connections (
		query_id INTEGER NOT NULL,
		connection_id INTEGER NOT NULL,
		PRIMARY KEY (query_id, connection_id),
		FOREIGN KEY (query_id) REFERENCES queries(id) ON DELETE CASCADE,
		FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
	);`)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		is_active INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS api_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		key_prefix TEXT NOT NULL,
		key_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_used_at DATETIME,
		is_active INTEGER DEFAULT 1,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS connections (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		driver TEXT NOT NULL,
		connection_string_enc TEXT NOT NULL,
		is_active INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS queries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		slug TEXT NOT NULL UNIQUE,
		description TEXT,
		sql_text TEXT NOT NULL,
		params_config TEXT, -- JSON defining expected params
		is_active INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		user_id INTEGER,
		connection_id INTEGER,
		query_id INTEGER,
		duration_ms INTEGER,
		status TEXT,
		error_message TEXT
	);
	`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: Add description column if it doesn't exist
	if !columnExists(db, "api_keys", "description") {
		_, err := db.Exec(`ALTER TABLE api_keys ADD COLUMN description TEXT;`)
		if err != nil {
			return fmt.Errorf("failed to add description column: %w", err)
		}
	}

	// Migration: Add api_key_id to audit_logs
	if !columnExists(db, "audit_logs", "api_key_id") {
		_, err := db.Exec(`ALTER TABLE audit_logs ADD COLUMN api_key_id INTEGER;`)
		if err != nil {
			return fmt.Errorf("failed to add api_key_id column: %w", err)
		}
	}

	// Migration: Add params to audit_logs
	if !columnExists(db, "audit_logs", "params") {
		_, err := db.Exec(`ALTER TABLE audit_logs ADD COLUMN params TEXT;`)
		if err != nil {
			return fmt.Errorf("failed to add params column: %w", err)
		}
	}

	return nil
}

func columnExists(db *sql.DB, tableName, columnName string) bool {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := db.Query(query)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false
		}
		if name == columnName {
			return true
		}
	}
	return false
}
