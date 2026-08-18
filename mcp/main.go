package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverInstructions = `Phrasely supports this vocabulary-learning workflow: hear -> understand -> explore contexts -> choose -> save -> encounter again.

When the user clearly wants to save a finished phrase, call add_phrase. Treat "add it", "save it", "add this", "save this", and similar unambiguous requests as confirmation; do not ask again. Never save from a request that only asks for an explanation, definition, comparison, or rewrite. If the referenced phrase is ambiguous, ask which one. Construct the phrase, headwords, note, and source_urls locally; there is no requirement to call explore_phrase first.

When the user wants to understand a word or expression, refine the context they heard it in, or find memorable examples, call explore_phrase. It is pedagogical and conversational and never saves anything.

After applying explore_phrase, call render_phrase_choices once with exactly three complete save-ready choices: the refined original context, one memorable alternative, and one learning connection. The render tool only displays choices; the user decides whether to save one. Do not render choices when the user already gave a direct, unambiguous save instruction — call add_phrase instead. If UI is unavailable, keep the numbered conversational fallback.

Include exactly one concise, save-ready teaching connection in the render_phrase_choices call. It appears as the third card (card 3) after the first two choices. Title it with the selected category—not "One useful connection"—choosing the strongest useful link: a likely confusable word first, then a meaningful opposite or contrast, a nuanced near-synonym, a register alternative, a common collocation or construction, a word-family link, a common mistake or meaning boundary, or finally a memorable association. Construct its phrase, headwords, note, and source_urls using the add_phrase rules, with the note explaining the connection. Never force or invent a weak connection, and do not repeat the connection outside the UI.

Phrasely is a private, personal vocabulary companion for understanding, exploring, saving, retrieving, and practising English expressions from real life.

Tools:
- list_phrases — list the user's Phrasely phrases, newest first, optionally filtered by matching headword text.
- sample_phrases — randomly select phrases for review, quizzes, or speaking practice.
- explore_phrase — return Phrasely's learning instructions for understanding a word or expression and generating memorable contexts; it does not persist data.
- render_phrase_choices — display two final exploration candidates and one learning connection as three saveable cards; it does not persist data.
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

	protectedResourceMetadataURL := strings.TrimRight(mcpBaseURL, "/") + "/.well-known/oauth-protected-resource"

	mux.Handle("/mcp", newMCPHandler(api, protectedResourceMetadataURL))

	slog.Info("mcp server listening", "port", port, "api", apiURL, "mcp_url", mcpBaseURL)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// newMCPHandler leaves discovery and tools/list public so a fresh ChatGPT
// connection can discover each tool's OAuth security scheme. The transport is
// stateless because Phrasely does not use server-initiated calls or resumability,
// and MCP 2026-07-28 requires stateless Streamable HTTP. Tool calls return an
// mcp/www_authenticate challenge unless the request carries a Bearer token.
func newMCPHandler(api *apiClient, protectedResourceMetadataURL string) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: serverVersion}, &mcp.ServerOptions{
			Instructions: serverInstructions,
		})
		s.AddReceivingMiddleware(logMCPDiscovery)
		registerResources(s)
		registerTools(s, api, protectedResourceMetadataURL)
		return s
	}, &mcp.StreamableHTTPOptions{Stateless: true})
}

// logMCPDiscovery records the protocol milestones needed to diagnose a
// connector that authenticates successfully but does not expose any tools.
func logMCPDiscovery(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		result, err := next(ctx, method, req)

		switch method {
		case "server/discover":
			if err != nil {
				slog.Warn("mcp: server discovery failed", "error", err)
			} else {
				slog.Info("mcp: server discovery succeeded")
			}
		case "initialize":
			if err != nil {
				slog.Warn("mcp: initialization failed", "error", err)
			} else {
				slog.Info("mcp: initialization succeeded")
			}
		case "tools/list":
			if err != nil {
				slog.Warn("mcp: tool discovery failed", "error", err)
			} else if tools, ok := result.(*mcp.ListToolsResult); ok {
				slog.Info("mcp: tool discovery succeeded", "tool_count", len(tools.Tools))
			}
		}

		return result, err
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

// bearerToken extracts a syntactically valid Authorization: Bearer token.
// Token signature and expiry validation remain the backend's responsibility.
func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return ""
	}
	return parts[1]
}
