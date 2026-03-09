package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Connect opens a PostgreSQL connection pool and verifies connectivity via Ping.
// Returns the pool ready for use or an error with context.
func Connect(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: opening postgres connection: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("db: pinging postgres: %w", err)
	}

	return db, nil
}
