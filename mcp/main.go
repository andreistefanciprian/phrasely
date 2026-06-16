package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// Configure structured JSON logging first so every subsequent log line
	// is parseable by Railway's log viewer. Level defaults to INFO.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(os.Getenv("LOG_LEVEL")),
	})))

	apiURL := os.Getenv("API_HOST")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	// mcpBaseURL and frontendBaseURL are used to build the OAuth discovery documents.
	// In production these are the public HTTPS URLs of each service.
	// Locally they default to localhost so the full OAuth flow can be exercised
	// without a public domain.
	mcpBaseURL := os.Getenv("MCP_BASE_URL")
	if mcpBaseURL == "" {
		mcpBaseURL = "http://localhost:" + port
	}
	frontendBaseURL := os.Getenv("FRONTEND_BASE_URL")
	if frontendBaseURL == "" {
		frontendBaseURL = "http://localhost:3000"
	}

	api := newAPIClient(apiURL)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// OAuth 2.1 discovery and public proxy endpoints.
	registerOAuthDiscovery(mux, oauthConfig{
		mcpBaseURL:      mcpBaseURL,
		frontendBaseURL: frontendBaseURL,
	})
	registerOAuthProxy(mux, api)

	// /mcp requires a Bearer token. The factory creates a per-request server so
	// each caller's JWT is scoped to their own tool invocations — no shared state.
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		// requireBearer guarantees the header is present before we reach here.
		jwt := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		s := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: "0.1.0"}, nil)
		registerTools(s, api, jwt)
		return s
	}, nil)

	mux.Handle("/mcp", requireBearer(mcpHandler))

	slog.Info("mcp server listening", "port", port, "api", apiURL, "mcp_url", mcpBaseURL)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
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

// requireBearer rejects requests without a valid Authorization: Bearer <token> header.
// ChatGPT sends the OAuth access token here after completing the flow.
// RFC 6750 §3: 401 responses must include WWW-Authenticate so clients know
// which auth scheme is expected.
func requireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		// Require exactly two fields: scheme + non-empty token.
		// strings.Fields handles multiple spaces and normalises case-insensitive "bearer".
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			slog.Debug("requireBearer: rejected request", "remote_addr", r.RemoteAddr, "reason", "missing or invalid Bearer token")
			w.Header().Set("WWW-Authenticate", `Bearer realm="mcp"`)
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		// Normalise to canonical form so downstream (factory + tools) can rely on
		// a simple strings.TrimPrefix("Bearer ") without worrying about whitespace.
		r.Header.Set("Authorization", "Bearer "+parts[1])
		next.ServeHTTP(w, r)
	})
}
