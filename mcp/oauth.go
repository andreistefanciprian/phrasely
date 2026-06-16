package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// oauthConfig holds the public URLs needed to populate discovery documents
// and proxy OAuth requests. Both are injected via env vars so the same binary
// works locally (http://localhost:...) and in production (https://...).
type oauthConfig struct {
	// mcpBaseURL is the public URL of this mcp server, e.g. https://mcp.getphrasely.com.
	// Used as the OAuth issuer and to build token/register endpoint URLs.
	mcpBaseURL string

	// frontendBaseURL is the public URL of the frontend, e.g. https://getphrasely.com.
	// The /authorize consent screen lives there, not on mcp.
	frontendBaseURL string
}

// registerOAuthDiscovery mounts the two well-known discovery endpoints on mux.
// ChatGPT (and any OAuth 2.1 client) fetches these first to learn where all
// the OAuth endpoints are before starting the flow.
func registerOAuthDiscovery(mux *http.ServeMux, cfg oauthConfig) {
	// /.well-known/oauth-authorization-server (RFC 8414)
	// This is the primary discovery document. It tells the client:
	//   - who the authorization server is (issuer)
	//   - where to send the user to log in and consent (/authorize → frontend)
	//   - where to exchange a code for tokens (/token → mcp)
	//   - where to register as a client (/register → mcp)
	//   - which PKCE method we require (S256 only — plain is insecure)
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 cfg.mcpBaseURL,
			"authorization_endpoint": cfg.frontendBaseURL + "/authorize",
			"token_endpoint":         cfg.mcpBaseURL + "/token",
			"registration_endpoint":  cfg.mcpBaseURL + "/register",
			// "code" is the only response type for the Authorization Code flow.
			"response_types_supported": []string{"code"},
			// refresh_token allows the client to get new access tokens without
			// re-prompting the user each time one expires.
			"grant_types_supported": []string{"authorization_code", "refresh_token"},
			// S256 is mandatory in OAuth 2.1. "plain" is deliberately omitted —
			// it provides no protection against code interception.
			"code_challenge_methods_supported": []string{"S256"},
		})
	})

	// /.well-known/oauth-protected-resource (RFC 9728)
	// Lets the client discover which authorization server protects this resource.
	// MCP Inspector and some clients check this before the server metadata above.
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"resource":              cfg.mcpBaseURL,
			"authorization_servers": []string{cfg.mcpBaseURL},
		})
	})
}

// registerOAuthProxy mounts the public OAuth proxy endpoints on mux.
// These are thin pass-throughs: mcp is the public face, backend does the real work.
func registerOAuthProxy(mux *http.ServeMux, api *apiClient) {
	// POST /register — Dynamic Client Registration (RFC 7591).
	// ChatGPT posts {"redirect_uris": [...]} here; mcp forwards it to backend's
	// internal /internal/oauth/register and returns the response (client_id).
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		proxyToBackend(w, r, api, "/internal/oauth/register")
	})
}

// proxyToBackend forwards the incoming request body to the given backend path
// and writes the backend's status code and body back to the client unchanged.
// mcp never inspects or modifies the payload — it's a transparent proxy.
func proxyToBackend(w http.ResponseWriter, r *http.Request, api *apiClient, backendPath string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024))
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		api.baseURL+backendPath, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := api.http.Do(req)
	if err != nil {
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
