package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func RunMigrations(db *sql.DB) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}

	entries, err := os.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, f := range files {
		applied, err := isMigrationApplied(db, f)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		content, err := os.ReadFile(filepath.Join("migrations", f))
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}

		// Split on ";" and execute each statement individually.
		// lib/pq's db.Exec() supports multi-statement strings via the simple-query
		// protocol, but splitting is more portable and gives clearer error messages.
		stmts := splitStatements(string(content))
		for i, stmt := range stmts {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("apply %s statement %d (%q): %w", f, i+1, truncate(stmt, 60), err)
			}
		}

		if _, err := db.Exec(`INSERT INTO schema_migrations (filename) VALUES ($1)`, f); err != nil {
			return fmt.Errorf("record migration %s: %w", f, err)
		}

		log.Printf("Migration applied: %s (%d statements)", f, len(stmts))
	}

	return nil
}

// splitStatements splits a SQL file into individual statements on ";",
// skipping blank lines and SQL-style comments.
func splitStatements(content string) []string {
	var stmts []string
	for _, raw := range strings.Split(content, ";") {
		stmt := strings.TrimSpace(raw)
		if stmt == "" {
			continue
		}
		// Strip leading comment-only lines so empty results after comment removal are skipped.
		var lines []string
		for _, line := range strings.Split(stmt, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "--") {
				continue
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			continue
		}
		stmts = append(stmts, strings.Join(lines, "\n"))
	}
	return stmts
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func ensureMigrationsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id        SERIAL PRIMARY KEY,
			filename  TEXT NOT NULL UNIQUE,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func isMigrationApplied(db *sql.DB, filename string) (bool, error) {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, filename).Scan(&exists)
	return exists, err
}
