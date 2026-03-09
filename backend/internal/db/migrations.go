package db

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// RunMigrations executes all unapplied migration files in filename order.
// Creates the schema_migrations tracking table if it does not exist.
// Idempotent: already-applied migrations are skipped.
func RunMigrations(db *sql.DB) error {
	// Ensure the tracking table exists before we attempt to read from it.
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    VARCHAR(50) PRIMARY KEY,
		applied_at TIMESTAMPTZ DEFAULT NOW()
	)`)
	if err != nil {
		return fmt.Errorf("migrations: creating tracking table: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("migrations: reading embedded directory: %w", err)
	}

	// Sort by filename so migrations run in numeric order (001, 002, …).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		version := entry.Name()

		// Check whether this migration has already been applied.
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = $1`, version,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("migrations: checking version %s: %w", version, err)
		}
		if count > 0 {
			continue // already applied
		}

		// Read the SQL file content.
		content, err := migrationFiles.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("migrations: reading file %s: %w", version, err)
		}

		// Execute the migration inside a transaction so partial failures leave
		// schema_migrations in a consistent state.
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("migrations: beginning transaction for %s: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrations: executing %s: %w", version, err)
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`,
			version,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrations: recording version %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrations: committing %s: %w", version, err)
		}
	}

	return nil
}
