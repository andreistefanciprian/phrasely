package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestParseLogLevel checks that LOG_LEVEL strings map to the correct slog levels.
func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"", slog.LevelInfo},        // unset → INFO
		{"VERBOSE", slog.LevelInfo}, // unrecognised → INFO
	}
	for _, tt := range tests {
		if got := parseLogLevel(tt.input); got != tt.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "missing", header: "", want: ""},
		{name: "wrong scheme", header: "Basic dXNlcjpwYXNz", want: ""},
		{name: "empty token", header: "Bearer ", want: ""},
		{name: "valid", header: "Bearer some-jwt-here", want: "some-jwt-here"},
		{name: "lowercase scheme", header: "bearer some-jwt-here", want: "some-jwt-here"},
		{name: "extra whitespace", header: "Bearer  some-jwt-here", want: "some-jwt-here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bearerToken(tt.header); got != tt.want {
				t.Errorf("bearerToken(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestAnonymousMCPDiscoveryAndAuthChallenge(t *testing.T) {
	const protectedResourceMetadataURL = "https://mcp.example.com/.well-known/oauth-protected-resource"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/phrases/summary" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[{"phrase":"A test phrase","headwords":["test"]}]`)
	}))
	defer backend.Close()

	server := httptest.NewServer(newMCPHandler(newAPIClient(backend.URL), protectedResourceMetadataURL))
	defer server.Close()

	post := func(body, sessionID, token string) (*http.Response, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp, string(responseBody)
	}

	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	resp, body := post(initialize, "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize status = %d, want 200; body = %s", resp.StatusCode, body)
	}
	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("initialize response has no Mcp-Session-Id")
	}

	_, body = post(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, sessionID, "")
	for _, want := range []string{`"name":"list_phrases"`, `"name":"sample_phrases"`, `"name":"explore_phrase"`, `"name":"add_phrase"`, `"securitySchemes"`} {
		if !strings.Contains(body, want) {
			t.Errorf("anonymous tools/list response does not contain %s: %s", want, body)
		}
	}

	_, body = post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"explore_phrase","arguments":{"phrase":"test"}}}`, sessionID, "")
	if strings.Contains(body, `"isError":true`) || !strings.Contains(body, `"instructions"`) {
		t.Errorf("public explore_phrase call failed: %s", body)
	}

	assertAuthChallenge := func(body string) {
		t.Helper()
		for _, want := range []string{`"isError":true`, `"mcp/www_authenticate"`, protectedResourceMetadataURL, `error=\"invalid_token\"`} {
			if !strings.Contains(body, want) {
				t.Errorf("authentication error response does not contain %q: %s", want, body)
			}
		}
	}

	_, body = post(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"list_phrases","arguments":{}}}`, sessionID, "")
	assertAuthChallenge(body)

	_, body = post(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_phrases","arguments":{}}}`, sessionID, "expired-token")
	assertAuthChallenge(body)

	_, body = post(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"list_phrases","arguments":{}}}`, sessionID, "test-token")
	if strings.Contains(body, `"isError":true`) || !strings.Contains(body, `"total":1`) || !strings.Contains(body, `"A test phrase"`) {
		t.Errorf("authenticated tool call did not succeed on the anonymously initialized session: %s", body)
	}
}

// newTestMux builds a plain http.ServeMux wired with the OAuth routes only,
// matching the structure in main.go. We don't spin up the MCP server here —
// just the OAuth endpoints under test.
func newTestMux(cfg oauthConfig, backendURL string) *http.ServeMux {
	mux := http.NewServeMux()
	registerOAuthDiscovery(mux, cfg)
	api := &apiClient{baseURL: backendURL, http: &http.Client{}}
	registerOAuthProxy(mux, api)
	return mux
}

// TestOAuthDiscovery checks that the two well-known endpoints return the right fields.
func TestOAuthDiscovery(t *testing.T) {
	cfg := oauthConfig{
		mcpBaseURL:      "https://mcp.example.com",
		frontendBaseURL: "https://example.com",
	}
	mux := newTestMux(cfg, "http://unused")

	t.Run("authorization_server metadata", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var doc map[string]any
		if err := json.NewDecoder(w.Body).Decode(&doc); err != nil {
			t.Fatalf("decode: %v", err)
		}

		// Issuer must match the mcp base URL.
		if got := doc["issuer"]; got != cfg.mcpBaseURL {
			t.Errorf("issuer = %q, want %q", got, cfg.mcpBaseURL)
		}
		// /authorize lives on the frontend, not mcp.
		wantAuth := cfg.frontendBaseURL + "/authorize"
		if got := doc["authorization_endpoint"]; got != wantAuth {
			t.Errorf("authorization_endpoint = %q, want %q", got, wantAuth)
		}
		// /token and /register are on mcp itself.
		if got := doc["token_endpoint"]; got != cfg.mcpBaseURL+"/token" {
			t.Errorf("token_endpoint = %q, want %q", got, cfg.mcpBaseURL+"/token")
		}
		if got := doc["registration_endpoint"]; got != cfg.mcpBaseURL+"/register" {
			t.Errorf("registration_endpoint = %q, want %q", got, cfg.mcpBaseURL+"/register")
		}
		// ChatGPT exchanges its PKCE proof as a public client, without a secret.
		tokenAuthMethods, _ := doc["token_endpoint_auth_methods_supported"].([]any)
		if len(tokenAuthMethods) != 1 || tokenAuthMethods[0] != "none" {
			t.Errorf("token_endpoint_auth_methods_supported = %v, want [none]", tokenAuthMethods)
		}
		// OAuth 2.1 mandates S256 only — "plain" must be absent.
		methods, _ := doc["code_challenge_methods_supported"].([]any)
		if len(methods) != 1 || methods[0] != "S256" {
			t.Errorf("code_challenge_methods_supported = %v, want [S256]", methods)
		}
	})

	t.Run("protected_resource metadata", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var doc map[string]any
		if err := json.NewDecoder(w.Body).Decode(&doc); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if got := doc["resource"]; got != cfg.mcpBaseURL {
			t.Errorf("resource = %q, want %q", got, cfg.mcpBaseURL)
		}
	})
}

// TestRegisterProxy checks that /register forwards the request body to the backend
// and relays the backend's response unchanged.
func TestRegisterProxy(t *testing.T) {
	t.Run("proxies success response from backend", func(t *testing.T) {
		// Stand up a fake backend that mimics /internal/oauth/register returning 201.
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/internal/oauth/register" {
				t.Errorf("backend received wrong path: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"client_id":"abc123","redirect_uris":["https://chatgpt.com/cb"]}`))
		}))
		defer backend.Close()

		mux := newTestMux(oauthConfig{}, backend.URL)
		w := httptest.NewRecorder()
		body := `{"redirect_uris":["https://chatgpt.com/cb"]}`
		r := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", w.Code)
		}

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["client_id"] != "abc123" {
			t.Errorf("client_id = %v, want abc123", resp["client_id"])
		}
	})

	t.Run("proxies error response from backend", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid redirect_uri"}`))
		}))
		defer backend.Close()

		mux := newTestMux(oauthConfig{}, backend.URL)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"redirect_uris":[]}`))
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("non-POST rejected", func(t *testing.T) {
		mux := newTestMux(oauthConfig{}, "http://unused")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/register", nil)
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", w.Code)
		}
	})

	t.Run("oversized body returns 413", func(t *testing.T) {
		mux := newTestMux(oauthConfig{}, "http://unused")
		w := httptest.NewRecorder()
		// One byte over the 4KB limit.
		r := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(strings.Repeat("x", proxyMaxBodyBytes+1)))
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want 413", w.Code)
		}
	})

	t.Run("backend unavailable returns 502", func(t *testing.T) {
		// Point at a port with nothing listening.
		mux := newTestMux(oauthConfig{}, "http://127.0.0.1:1")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"redirect_uris":["https://example.com"]}`))
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusBadGateway {
			t.Errorf("status = %d, want 502", w.Code)
		}
	})
}

func TestTokenProxy(t *testing.T) {
	t.Run("proxies form body and forwards cache headers", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify the backend received form-encoded content type, not JSON.
			if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
				t.Errorf("backend Content-Type = %q, want application/x-www-form-urlencoded", ct)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"access_token":"tok","token_type":"Bearer","expires_in":3600}`))
		}))
		defer backend.Close()

		mux := newTestMux(oauthConfig{}, backend.URL)
		w := httptest.NewRecorder()
		body := "grant_type=authorization_code&code=abc&code_verifier=xyz&client_id=cid&redirect_uri=https://example.com"
		r := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		// Cache-Control: no-store must reach the client (RFC 6749 §5.1).
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", cc)
		}
	})

	t.Run("non-POST rejected", func(t *testing.T) {
		mux := newTestMux(oauthConfig{}, "http://unused")
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/token", nil)
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", w.Code)
		}
	})
}
