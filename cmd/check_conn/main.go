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
	wd, _ := os.Getwd()
	dbPath := filepath.Join(wd, "dbbridge.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	connName := "ittest"
	var id int64
	err = db.QueryRow("SELECT id FROM connections WHERE name = ?", connName).Scan(&id)
	if err == sql.ErrNoRows {
		fmt.Println("Connection 'ittest' not found. Creating...")
		res, err := db.Exec(`INSERT INTO connections (name, driver, connection_string_enc, is_active) 
			VALUES (?, ?, ?, ?)`, connName, "sqlite", "test.db", 1)
		if err != nil {
			log.Fatal(err)
		}
		id, _ = res.LastInsertId()
		fmt.Printf("Created connection 'ittest' with ID: %d\n", id)
	} else if err != nil {
		log.Fatal(err)
	} else {
		fmt.Printf("Connection 'ittest' exists with ID: %d\n", id)
	}
}
