package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/auth"
	"github.com/andreistefanciprian/phrasely/internal/curate"
	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/email"
	"github.com/andreistefanciprian/phrasely/internal/embeddings"
	"github.com/andreistefanciprian/phrasely/internal/middleware"
	"github.com/andreistefanciprian/phrasely/internal/oauth"
	"github.com/andreistefanciprian/phrasely/internal/phrases"
	"github.com/andreistefanciprian/phrasely/internal/settings"
	"github.com/andreistefanciprian/phrasely/migrations"
	"github.com/gorilla/mux"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql, used by goose
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"
)

func main() {
	// Load .env when running locally. In production, env vars are injected by the platform.
	_ = godotenv.Load()

	// Configure structured JSON logging first so every subsequent log line
	// is parseable by Railway's log viewer. Level defaults to INFO.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(os.Getenv("LOG_LEVEL")),
	})))

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

	// BASE_URL is the frontend origin — magic links point here so the browser
	// lands on the frontend /auth-verify page, not the API's /auth/verify endpoint.
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		slog.Error("JWT_SECRET is not set")
		os.Exit(1)
	}

	magicLinkTTL := durationEnv("MAGIC_LINK_TTL", 15*time.Minute)
	jwtTTL := durationEnv("JWT_TTL", 30*24*time.Hour)

	r.Use(middleware.Authenticate(jwtSecret))

	var mailer email.Sender
	resendKey := os.Getenv("RESEND_API_KEY")
	emailFrom := os.Getenv("EMAIL_FROM")
	if resendKey != "" && emailFrom != "" {
		mailer = email.NewResendSender(resendKey, emailFrom)
		slog.Info("email enabled", "from", emailFrom)
	} else {
		mailer = &email.LogSender{}
		slog.Warn("RESEND_API_KEY or EMAIL_FROM not set — magic links logged to stdout")
	}
	auth.NewHandler(store, baseURL, jwtSecret, mailer, magicLinkTTL, jwtTTL).RegisterRoutes(r)

	// Internal OAuth 2.1 endpoints — only reachable by mcp and frontend over the
	// private network. Not exposed to the internet.
	oauthAccessTokenTTL := durationEnv("OAUTH_ACCESS_TOKEN_TTL", time.Hour)
	oauth.NewHandler(store, jwtSecret, oauthAccessTokenTTL).RegisterRoutes(r)

	// Embeddings and curate are both optional — only enabled when OPENAI_API_KEY is set.
	var embedder *embeddings.Service
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		embedder = embeddings.New(apiKey)
		slog.Info("embeddings enabled")

		curator, err := curate.NewCurator(apiKey)
		if err != nil {
			slog.Warn("failed to initialize curator — curate endpoint disabled", "error", err)
		} else {
			curate.NewHandler(curator).RegisterRoutes(r)
			slog.Info("curate endpoint enabled")
		}
	} else {
		slog.Warn("OPENAI_API_KEY not set — embeddings and curate endpoint disabled")
	}

	relatedMaxDist := floatEnv("RELATED_MAX_DISTANCE", 0.45)
	phrases.NewHandler(store, embedder, relatedMaxDist).RegisterRoutes(r)
	settings.NewHandler(store).RegisterRoutes(r)

	// --- HTTP server ---
	// Explicit timeouts prevent slow clients from holding connections open indefinitely.
	// CORS is not needed — all browser traffic is proxied through nginx (same origin).
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

	// Verify connectivity before handing the connection to goose.
	// Without this, goose would hang indefinitely if the DB is unreachable.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
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

// parseLogLevel maps a LOG_LEVEL string to a slog.Level.
// Defaults to INFO for empty or unrecognised values.
func parseLogLevel(s string) slog.Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// floatEnv reads a float64 from the named env var, falling back to def if unset or invalid.
func floatEnv(key string, def float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		slog.Warn("invalid float env var, using default", "key", key, "value", val, "default", def)
		return def
	}
	return f
}

// durationEnv reads a duration from the named env var (e.g. "15m", "720h"),
// falling back to def if unset or invalid.
func durationEnv(key string, def time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		slog.Warn("invalid duration env var, using default", "key", key, "value", val, "default", def)
		return def
	}
	return d
}
