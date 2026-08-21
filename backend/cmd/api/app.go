package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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
	"github.com/gorilla/mux"
)

// run is the API's composition root. It owns startup, dependency cleanup, and
// the server lifecycle; main only translates process signals and exit status.
func run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// --- Migrations ---
	// goose uses the standard database/sql interface, so we open a short-lived
	// sql.DB just for this step, then close it before the app pool takes over.
	if err := runMigrations(ctx, cfg.databaseURL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// --- Database connection pool ---
	// pgxpool gives us a high-performance pool of reusable connections.
	// The 10s timeout ensures startup fails fast if the DB is unreachable.
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	store, err := db.NewPostgresStore(connectCtx, cfg.databaseURL)
	cancel()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer store.Close()
	slog.Info("connected to database")

	// --- Router ---
	router := buildRouter(cfg, store)

	// --- HTTP server ---
	server := newHTTPServer(cfg.port, router)
	slog.Info("server listening", "port", cfg.port)

	if err := serve(ctx, server); err != nil {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}

func buildRouter(cfg config, store db.Store) http.Handler {
	r := mux.NewRouter()

	// /health is used by Docker Compose and load balancers to check the service is alive.
	r.HandleFunc("/health", healthHandler).Methods(http.MethodGet)

	// Authentication middleware knows which routes are public and passes those through.
	r.Use(middleware.Authenticate(cfg.jwtSecret))

	mailer := newMailer(cfg)
	auth.NewHandler(
		store,
		cfg.baseURL,
		cfg.jwtSecret,
		mailer,
		cfg.magicLinkTTL,
		cfg.jwtTTL,
	).RegisterRoutes(r)

	// Internal OAuth 2.1 endpoints — only reachable by mcp and frontend over the
	// private network. Not exposed to the internet.
	oauth.NewHandler(store, cfg.jwtSecret, cfg.oauthAccessTokenTTL).RegisterRoutes(r)

	// Embeddings and curate are both optional — only enabled when OPENAI_API_KEY is set.
	embedder := registerOpenAIRoutes(r, cfg.openAIAPIKey)

	phrases.NewHandler(store, embedder, cfg.relatedMaxDistance).RegisterRoutes(r)
	settings.NewHandler(store).RegisterRoutes(r)

	return r
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func newMailer(cfg config) email.Sender {
	if cfg.resendAPIKey != "" && cfg.emailFrom != "" {
		slog.Info("email enabled", "from", cfg.emailFrom)
		return email.NewResendSender(cfg.resendAPIKey, cfg.emailFrom)
	}

	slog.Warn("RESEND_API_KEY or EMAIL_FROM not set — magic links logged to stdout")
	return &email.LogSender{}
}

func registerOpenAIRoutes(r *mux.Router, apiKey string) *embeddings.Service {
	if apiKey == "" {
		slog.Warn("OPENAI_API_KEY not set — embeddings and curate endpoint disabled")
		return nil
	}

	embedder := embeddings.New(apiKey)
	slog.Info("embeddings enabled")

	curator, err := curate.NewCurator(apiKey)
	if err != nil {
		slog.Warn("failed to initialize curator — curate endpoint disabled", "error", err)
		return embedder
	}

	curate.NewHandler(curator).RegisterRoutes(r)
	slog.Info("curate endpoint enabled")
	return embedder
}
