package main

import (
	"log/slog"
	"net/http"
	"os"

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

	// Phase 1: a single static long-lived JWT, same format as auth.signJWT,
	// forwarded as-is to backend on every tool call. Per-caller OAuth tokens
	// come in Phase 2.
	authToken := os.Getenv("MCP_AUTH_TOKEN")

	api := newAPIClient(apiURL)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// OAuth 2.1 discovery and public proxy endpoints (Phase 2).
	registerOAuthDiscovery(mux, oauthConfig{
		mcpBaseURL:      mcpBaseURL,
		frontendBaseURL: frontendBaseURL,
	})
	registerOAuthProxy(mux, api)

	server := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: "0.1.0"}, nil)
	registerTools(server, api, authToken)

	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil))

	slog.Info("mcp server listening", "port", port, "api", apiURL, "mcp_url", mcpBaseURL)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
