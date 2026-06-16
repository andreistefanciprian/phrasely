package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
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

// requireBearer rejects requests without an Authorization: Bearer <token> header.
// ChatGPT sends the OAuth access token here after completing the flow.
// RFC 6750 §3: 401 responses must include WWW-Authenticate so clients know
// which auth scheme is expected.
func requireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="mcp"`)
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
