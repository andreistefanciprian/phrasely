package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
