package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql, used by goose
	"github.com/pressly/goose/v3"
)

func main() {
	// Load .env when running locally. In production, env vars are injected by the platform.
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		slog.Error("DATABASE_URL is not set")
		os.Exit(1)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// --- Migrations ---
	// goose uses the standard database/sql interface, so we open a short-lived
	// sql.DB just for this step, then close it before the app pool takes over.
	if err := runMigrations(dsn); err != nil {
		slog.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	// --- Database connection pool ---
	// pgxpool gives us a high-performance pool of reusable connections.
	// The 10s timeout ensures startup fails fast if the DB is unreachable.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := db.NewPostgresStore(ctx, dsn)
	if err != nil {
		slog.Error("could not connect to database", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	slog.Info("connected to database")

	// --- Router ---
	r := mux.NewRouter()

	// /health is used by Docker Compose and load balancers to check the service is alive.
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}).Methods(http.MethodGet)

	// --- HTTP server ---
	// Explicit timeouts prevent slow clients from holding connections open indefinitely.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	slog.Info("server listening", "port", port)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// runMigrations applies any pending SQL migrations from the migrations/ directory.
// goose tracks which migrations have already run, so restarting the app is always safe.
func runMigrations(dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	goose.SetLogger(goose.NopLogger()) // silence goose's default output; we log ourselves
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return err
	}

	slog.Info("migrations applied")
	return nil
}
