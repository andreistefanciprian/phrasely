package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverInstructions = `Phrasely supports this vocabulary-learning workflow: hear -> understand -> explore contexts -> choose -> save -> encounter again.

When the user wants to understand a word or expression, refine the context they heard it in, or find memorable examples, call explore_phrase. It is pedagogical and conversational and never saves anything.

When the user has a finished phrase — either supplied directly or chosen from an explore_phrase conversation — construct the phrase, headwords, note, and source_urls locally, then call add_phrase. There is no requirement to call explore_phrase first. Treat "add it", "save it", "add this", "save this", and similar unambiguous requests as confirmation; do not ask again. Never save from a request that only asks for an explanation, definition, comparison, or rewrite. If the referenced phrase is ambiguous, ask which one.

Phrasely is a private, personal vocabulary companion for understanding, exploring, saving, retrieving, and practising English expressions from real life.

Tools:
- list_phrases — list the user's Phrasely phrases, newest first, optionally filtered by matching headword text.
- sample_phrases — randomly select phrases for review, quizzes, or speaking practice.
- explore_phrase — return Phrasely's learning instructions for understanding a word or expression and generating memorable contexts; it does not persist data.
- add_phrase — persist one finished phrase entry to Phrasely.

After retrieving phrases, carry out the requested review or practice conversationally.`

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
		s := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: serverVersion}, &mcp.ServerOptions{
			Instructions: serverInstructions,
		})
		registerTools(s, api, jwt)
		return s
	}, nil)

	protectedResourceMetadataURL := strings.TrimRight(mcpBaseURL, "/") + "/.well-known/oauth-protected-resource"
	mux.Handle("/mcp", requireBearer(mcpHandler, protectedResourceMetadataURL))

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
func requireBearer(next http.Handler, protectedResourceMetadataURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		// Require exactly two fields: scheme + non-empty token.
		// strings.Fields handles multiple spaces and normalises case-insensitive "bearer".
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			slog.Debug("requireBearer: rejected request", "remote_addr", r.RemoteAddr, "reason", "missing or invalid Bearer token")
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+protectedResourceMetadataURL+`"`)
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		// Normalise to canonical form so downstream (factory + tools) can rely on
		// a simple strings.TrimPrefix("Bearer ") without worrying about whitespace.
		r.Header.Set("Authorization", "Bearer "+parts[1])
		next.ServeHTTP(w, r)
	})
}
