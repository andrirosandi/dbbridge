package data

import (
	"database/sql"
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
	return err
}
