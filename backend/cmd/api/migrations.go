package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/andreistefanciprian/phrasely/migrations"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql, used by goose
	"github.com/pressly/goose/v3"
)

// runMigrations applies any pending SQL migrations from the migrations/ directory.
// goose tracks which migrations have already run, so restarting the app is always safe.
func runMigrations(ctx context.Context, dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	// Verify connectivity before handing the connection to goose.
	// Without this, goose would hang indefinitely if the DB is unreachable.
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return fmt.Errorf("ping before migrations: %w", err)
	}

	goose.SetLogger(goose.NopLogger()) // silence goose's default output; we log ourselves
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	// Tell goose to read migration files from the embedded FS instead of disk.
	// The path "." matches the root of migrations.FS where the .sql files live.
	goose.SetBaseFS(migrations.FS)
	if err := goose.Up(sqlDB, "."); err != nil {
		return err
	}

	slog.Info("migrations applied")
	return nil
}
