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

	server := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: "0.1.0"}, nil)
	registerTools(server, api, authToken)

	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	slog.Info("mcp server listening", "port", port, "api", apiURL)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
