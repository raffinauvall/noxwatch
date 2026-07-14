package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type migration struct {
	name string
	up   string
	down string
}

func Migrate(ctx context.Context, db *pgxpool.Pool, dir, mode string) error {
	migrations, err := readMigrations(dir)
	if err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	switch mode {
	case "up":
		return migrateUp(ctx, db, migrations)
	case "down":
		return migrateDown(ctx, db, migrations)
	default:
		return fmt.Errorf("unknown migration mode %q", mode)
	}
}

func migrateUp(ctx context.Context, db *pgxpool.Pool, migrations []migration) error {
	applied := map[string]bool{}
	rows, err := db.Query(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		applied[name] = true
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}

	for _, m := range migrations {
		if applied[m.name] {
			continue
		}
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, m.up); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("%s up: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, m.name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func migrateDown(ctx context.Context, db *pgxpool.Pool, migrations []migration) error {
	var name string
	if err := db.QueryRow(ctx, `SELECT name FROM schema_migrations ORDER BY name DESC LIMIT 1`).Scan(&name); err != nil {
		return err
	}
	for i := len(migrations) - 1; i >= 0; i-- {
		if migrations[i].name != name {
			continue
		}
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, migrations[i].down); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("%s down: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE name = $1`, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		return tx.Commit(ctx)
	}
	return fmt.Errorf("applied migration %q not found on disk", name)
}

func readMigrations(dir string) ([]migration, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	migrations := make([]migration, 0, len(files))
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		up, down, err := splitMigration(string(body))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		migrations = append(migrations, migration{name: filepath.Base(file), up: up, down: down})
	}
	return migrations, nil
}

func splitMigration(body string) (string, string, error) {
	upStart := strings.Index(body, "-- +noxwatch Up")
	downStart := strings.Index(body, "-- +noxwatch Down")
	if upStart == -1 || downStart == -1 || downStart <= upStart {
		return "", "", fmt.Errorf("missing noxwatch migration markers")
	}
	up := strings.TrimSpace(body[upStart+len("-- +noxwatch Up") : downStart])
	down := strings.TrimSpace(body[downStart+len("-- +noxwatch Down"):])
	if up == "" || down == "" {
		return "", "", fmt.Errorf("empty migration section")
	}
	return up, down, nil
}
