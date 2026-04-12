package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type migrationFile struct {
	version string
	path    string
}

func RunMigrations(ctx context.Context, db *sql.DB, dir string) error {
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return err
	}

	applied, err := loadAppliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	migrations, err := collectMigrationFiles(dir)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if applied[migration.version] {
			log.Printf("migration skipped: %s already applied", migration.version)
			continue
		}

		if err := applyMigration(ctx, db, migration); err != nil {
			return err
		}

		log.Printf("migration applied: %s", migration.version)
	}

	return nil
}

func ensureMigrationsTable(ctx context.Context, db *sql.DB) error {
	query := `
		create table if not exists schema_migrations (
			version varchar(255) primary key,
			applied_at timestamptz not null default now()
		);
	`

	_, err := db.ExecContext(ctx, query)
	return err
}

func loadAppliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `select version from schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return applied, nil
}

func collectMigrationFiles(dir string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	migrations := make([]migrationFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".down.sql") {
			continue
		}

		migrations = append(migrations, migrationFile{
			version: name,
			path:    filepath.Join(dir, name),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

func applyMigration(ctx context.Context, db *sql.DB, migration migrationFile) error {
	script, err := os.ReadFile(migration.path)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, string(script)); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.version, err)
	}

	if _, err = tx.ExecContext(ctx, `insert into schema_migrations (version) values ($1)`, migration.version); err != nil {
		return fmt.Errorf("save migration %s: %w", migration.version, err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.version, err)
	}

	return nil
}
