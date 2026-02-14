package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	// 1. Connect to DB
	// Assume running from project root
	wd, _ := os.Getwd()
	dbPath := filepath.Join(wd, "dbbridge.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 2. Insert Connection (if not exists)
	connName := "test-conn"
	_, err = db.Exec(`INSERT OR IGNORE INTO connections (name, driver, connection_string_enc, is_active) 
		VALUES (?, ?, ?, ?)`, connName, "sqlite", "test.db", 1)
	if err != nil {
		log.Printf("Failed to insert conn: %v", err)
	}

	// Get Conn ID
	var connID int64
	err = db.QueryRow("SELECT id FROM connections WHERE name = ?", connName).Scan(&connID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Connection ID: %d\n", connID)

	// 3. Insert Query (if not exists)
	querySlug := "test-query"
	_, err = db.Exec(`INSERT OR IGNORE INTO queries (slug, description, sql_text, params_config, is_active) 
		VALUES (?, ?, ?, ?, ?)`, querySlug, "Test Query", "SELECT 'Hello API' as message", "{}", 1)
	if err != nil {
		log.Printf("Failed to insert query: %v", err)
	}

	// Get Query ID
	var queryID int64
	err = db.QueryRow("SELECT id FROM queries WHERE slug = ?", querySlug).Scan(&queryID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Query ID: %d\n", queryID)

	// 4. Link
	_, err = db.Exec(`INSERT OR IGNORE INTO query_connections (query_id, connection_id) VALUES (?, ?)`, queryID, connID)
	if err != nil {
		log.Printf("Failed to link: %v", err)
	}

	fmt.Println("Test data created successfully.")
}
