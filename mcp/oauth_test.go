package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestMCPCurrentProtocolDiscoveryListsTools(t *testing.T) {
	server := httptest.NewServer(newMCPHandler(newAPIClient("http://unused"), "https://mcp.example.com/.well-known/oauth-protected-resource"))
	defer server.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL}, nil)
	if err != nil {
		t.Fatalf("connect with current protocol: %v", err)
	}
	defer session.Close()

	initializeResult := session.InitializeResult()
	if initializeResult == nil {
		t.Fatal("current protocol discovery returned no initialization result")
	}
	if initializeResult.ProtocolVersion != "2026-07-28" {
		t.Fatalf("negotiated protocol = %q, want 2026-07-28", initializeResult.ProtocolVersion)
	}
	if session.ID() != "" {
		t.Fatalf("stateless connection returned unexpected session ID %q", session.ID())
	}

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools with current protocol: %v", err)
	}
	got := make(map[string]bool, len(tools.Tools))
	for _, tool := range tools.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"list_phrases", "sample_phrases", "explore_phrase", "add_phrase"} {
		if !got[want] {
			t.Errorf("current protocol tools/list is missing %q: %v", want, got)
		}
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
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse backend form: %v", err)
			}
			if got := r.PostForm.Get("resource"); got != "https://mcp.example.com" {
				t.Errorf("backend resource = %q, want https://mcp.example.com", got)
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
		body := "grant_type=authorization_code&code=abc&code_verifier=xyz&client_id=cid&redirect_uri=https://example.com&resource=https://mcp.example.com"
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
