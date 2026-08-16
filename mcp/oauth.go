package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
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
	//   - that token exchange uses a public client with no client secret
	//   - which PKCE method we require (S256 only — plain is insecure)
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("oauth: authorization server metadata served")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 cfg.mcpBaseURL,
			"authorization_endpoint": cfg.frontendBaseURL + "/authorize",
			"token_endpoint":         cfg.mcpBaseURL + "/token",
			"registration_endpoint":  cfg.mcpBaseURL + "/register",
			// revocation_endpoint advertises RFC 7009 support so ChatGPT (and other
			// clients) know where to POST when the user clicks Disconnect. Without
			// this field, clients that honour the spec won't attempt revocation at all.
			"revocation_endpoint": cfg.mcpBaseURL + "/revoke",
			// "code" is the only response type for the Authorization Code flow.
			"response_types_supported": []string{"code"},
			// refresh_token allows the client to get new access tokens without
			// re-prompting the user each time one expires.
			"grant_types_supported": []string{"authorization_code", "refresh_token"},
			// ChatGPT is a public OAuth client: it sends client_id + PKCE proof
			// during token exchange, but no client secret.
			"token_endpoint_auth_methods_supported": []string{"none"},
			// S256 is mandatory in OAuth 2.1. "plain" is deliberately omitted —
			// it provides no protection against code interception.
			"code_challenge_methods_supported": []string{"S256"},
		})
	})

	// /.well-known/oauth-protected-resource (RFC 9728)
	// Lets the client discover which authorization server protects this resource.
	// MCP Inspector and some clients check this before the server metadata above.
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("oauth: protected resource metadata served")
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

	// POST /token — authorization code → access token exchange (RFC 6749 §4.1.3).
	// ChatGPT posts the code + code_verifier (application/x-www-form-urlencoded);
	// mcp forwards to backend's /internal/oauth/token which does the PKCE check
	// and issues a JWT access token + refresh token.
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		proxyToBackend(w, r, api, "/internal/oauth/token")
	})

	// POST /revoke — RFC 7009 token revocation.
	// ChatGPT calls this when the user clicks Disconnect in the connector UI.
	// Body is application/x-www-form-urlencoded: token=<value>&client_id=<id>
	// The backend always returns 200 (RFC 7009 §2.2), so mcp just passes it through.
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		slog.Debug("oauth: proxying revocation request to backend")
		proxyToBackend(w, r, api, "/internal/oauth/revoke")
	})
}

const proxyMaxBodyBytes = 4 * 1024

// proxyToBackend forwards the incoming request body to the given backend path
// and writes the backend's status code and body back to the client unchanged.
// mcp never inspects or modifies the payload — it's a transparent proxy.
func proxyToBackend(w http.ResponseWriter, r *http.Request, api *apiClient, backendPath string) {
	// Read one byte beyond the limit. If we get that extra byte, the body is too
	// large and we reject with 413 rather than silently truncating and forwarding
	// a partial payload to the backend.
	body, err := io.ReadAll(io.LimitReader(r.Body, proxyMaxBodyBytes+1))
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > proxyMaxBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		api.baseURL+backendPath, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Forward the original Content-Type — /register sends application/json,
	// /token sends application/x-www-form-urlencoded (OAuth 2.1 §3.2).
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}

	resp, err := api.http.Do(req)
	if err != nil {
		slog.Error("oauth: proxy: backend unavailable", "path", backendPath, "error", err)
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		slog.Warn("oauth: proxy: backend rejected request", "path", backendPath, "status", resp.StatusCode)
	} else {
		slog.Debug("oauth: proxy: backend responded", "path", backendPath, "status", resp.StatusCode)
	}

	// Copy backend response headers (Content-Type, Cache-Control, Pragma, etc.)
	// while stripping hop-by-hop headers that proxies must not forward (RFC 7230 §6.1).
	for k, vals := range resp.Header {
		switch http.CanonicalHeaderKey(k) {
		case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
			"Te", "Trailer", "Transfer-Encoding", "Upgrade":
			continue
		}
		w.Header()[k] = append([]string(nil), vals...)
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
