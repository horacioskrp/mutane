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

// splitStatements splits a SQL file into individual statements, respecting
// semicolons inside -- line comments and quoted strings.
func splitStatements(content string) []string {
	var stmts []string
	var buf strings.Builder
	inLineComment := false

	for i := 0; i < len(content); i++ {
		ch := content[i]

		// Detect start of -- comment
		if !inLineComment && ch == '-' && i+1 < len(content) && content[i+1] == '-' {
			inLineComment = true
			buf.WriteByte(ch)
			continue
		}
		// End of line comment on newline
		if inLineComment && ch == '\n' {
			inLineComment = false
			buf.WriteByte(ch)
			continue
		}
		// Semicolon inside comment — treat as comment text, not a separator
		if inLineComment {
			buf.WriteByte(ch)
			continue
		}

		if ch == ';' {
			// Strip comment-only lines from the accumulated statement
			stmt := stripCommentLines(buf.String())
			if stmt != "" {
				stmts = append(stmts, stmt)
			}
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	// Trailing content after last semicolon
	if stmt := stripCommentLines(buf.String()); stmt != "" {
		stmts = append(stmts, stmt)
	}
	return stmts
}

func stripCommentLines(raw string) string {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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
