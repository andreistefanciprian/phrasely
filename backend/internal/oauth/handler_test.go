package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/middleware"
	"github.com/gorilla/mux"
)

// mockStore satisfies db.Store for oauth handler tests.
// Only the fields used by the endpoint under test need to be set;
// all other methods panic to catch unexpected calls.
type mockStore struct {
	createOAuthClient        func(ctx context.Context, redirectURIs []string) (*db.OAuthClient, error)
	getOAuthClient           func(ctx context.Context, clientID string) (*db.OAuthClient, error)
	createAuthorizationCode  func(ctx context.Context, req db.CreateAuthCodeRequest) (*db.OAuthAuthorizationCode, error)
	consumeAuthorizationCode func(ctx context.Context, code, clientID string) (*db.OAuthAuthorizationCode, error)
	createRefreshToken       func(ctx context.Context, req db.CreateRefreshTokenRequest) (*db.OAuthRefreshToken, error)
	consumeRefreshToken      func(ctx context.Context, token string) (*db.OAuthRefreshToken, error)
}

func (m *mockStore) Close() {}
func (m *mockStore) CreateOAuthClient(ctx context.Context, redirectURIs []string) (*db.OAuthClient, error) {
	return m.createOAuthClient(ctx, redirectURIs)
}
func (m *mockStore) GetOAuthClient(ctx context.Context, clientID string) (*db.OAuthClient, error) {
	return m.getOAuthClient(ctx, clientID)
}
func (m *mockStore) CreateAuthorizationCode(ctx context.Context, req db.CreateAuthCodeRequest) (*db.OAuthAuthorizationCode, error) {
	return m.createAuthorizationCode(ctx, req)
}
func (m *mockStore) CreatePhrase(_ context.Context, _ string, _ db.CreatePhraseRequest) (*db.Phrase, error) {
	panic("not expected")
}
func (m *mockStore) ListPhrases(_ context.Context, _ string, _ string) ([]db.Phrase, error) {
	panic("not expected")
}
func (m *mockStore) GetPhrase(_ context.Context, _ string, _ string) (*db.Phrase, error) {
	panic("not expected")
}
func (m *mockStore) DeletePhrase(_ context.Context, _ string, _ string) error { panic("not expected") }
func (m *mockStore) UpdatePhrase(_ context.Context, _ string, _ string, _ db.UpdatePhraseRequest) (*db.Phrase, error) {
	panic("not expected")
}
func (m *mockStore) UpsertUser(_ context.Context, _ string) (*db.User, error) { panic("not expected") }
func (m *mockStore) CreateMagicLinkToken(_ context.Context, _ string, _ time.Time) (*db.MagicLinkToken, error) {
	panic("not expected")
}
func (m *mockStore) GetMagicLinkToken(_ context.Context, _ string) (*db.MagicLinkToken, error) {
	panic("not expected")
}
func (m *mockStore) MarkTokenUsed(_ context.Context, _ string) error { panic("not expected") }
func (m *mockStore) ConsumeAuthorizationCode(ctx context.Context, code, clientID string) (*db.OAuthAuthorizationCode, error) {
	if m.consumeAuthorizationCode != nil {
		return m.consumeAuthorizationCode(ctx, code, clientID)
	}
	panic("not expected")
}
func (m *mockStore) CreateRefreshToken(ctx context.Context, req db.CreateRefreshTokenRequest) (*db.OAuthRefreshToken, error) {
	if m.createRefreshToken != nil {
		return m.createRefreshToken(ctx, req)
	}
	panic("not expected")
}
func (m *mockStore) ConsumeRefreshToken(ctx context.Context, token string) (*db.OAuthRefreshToken, error) {
	if m.consumeRefreshToken != nil {
		return m.consumeRefreshToken(ctx, token)
	}
	panic("not expected")
}

const testJWTSecret = "test-jwt-secret"

func newTestServer(store db.Store) *httptest.Server {
	r := mux.NewRouter()
	NewHandler(store, testJWTSecret).RegisterRoutes(r)
	return httptest.NewServer(r)
}

// callAuthorize sends a POST /internal/oauth/authorize request directly through
// the router (no HTTP round-trip) with userID pre-injected into context —
// the same value the JWT middleware would set for a logged-in user.
func callAuthorize(store db.Store, userID, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/internal/oauth/authorize", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if userID != "" {
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, userID))
	}
	router := mux.NewRouter()
	NewHandler(store, testJWTSecret).RegisterRoutes(router)
	router.ServeHTTP(w, r)
	return w
}

func TestRegister(t *testing.T) {
	okStore := &mockStore{
		createOAuthClient: func(_ context.Context, uris []string) (*db.OAuthClient, error) {
			return &db.OAuthClient{ID: "client-123", RedirectURIs: uris}, nil
		},
	}

	tests := []struct {
		name       string
		body       string
		store      *mockStore
		wantStatus int
	}{
		{
			name:       "valid redirect_uri returns 201 with client_id",
			body:       `{"redirect_uris":["https://chatgpt.com/callback"]}`,
			store:      okStore,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "empty redirect_uris rejected",
			body:       `{"redirect_uris":[]}`,
			store:      okStore,
			wantStatus: http.StatusBadRequest,
		},
		{
			// Fragments are forbidden by RFC 6749 §3.1.2 — the browser strips them
			// before the redirect, so the server can never validate them.
			name:       "fragment in redirect_uri rejected",
			body:       `{"redirect_uris":["https://example.com/cb#frag"]}`,
			store:      okStore,
			wantStatus: http.StatusBadRequest,
		},
		{
			// Non-http(s) schemes (ftp://, javascript:, etc.) are not valid redirect targets.
			name:       "non-http scheme rejected",
			body:       `{"redirect_uris":["ftp://example.com/cb"]}`,
			store:      okStore,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "store error returns 500",
			body: `{"redirect_uris":["https://example.com/cb"]}`,
			store: &mockStore{
				createOAuthClient: func(_ context.Context, _ []string) (*db.OAuthClient, error) {
					return nil, errors.New("db unavailable")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(tt.store)
			defer srv.Close()

			resp, err := http.Post(srv.URL+"/internal/oauth/register", "application/json", strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			// On success, confirm client_id is present in the response.
			if tt.wantStatus == http.StatusCreated {
				var body map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if body["client_id"] == "" {
					t.Error("expected client_id in response")
				}
			}
		})
	}
}

func TestGetClient(t *testing.T) {
	tests := []struct {
		name       string
		clientID   string
		store      *mockStore
		wantStatus int
	}{
		{
			name:     "known client returns 200 with redirect_uris",
			clientID: "client-abc",
			store: &mockStore{
				getOAuthClient: func(_ context.Context, id string) (*db.OAuthClient, error) {
					return &db.OAuthClient{ID: id, RedirectURIs: []string{"https://chatgpt.com/cb"}}, nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "unknown client returns 404",
			clientID: "no-such-client",
			store: &mockStore{
				getOAuthClient: func(_ context.Context, _ string) (*db.OAuthClient, error) {
					return nil, db.ErrNotFound
				},
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(tt.store)
			defer srv.Close()

			resp, err := http.Get(srv.URL + "/internal/oauth/clients/" + tt.clientID)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var body clientResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if len(body.RedirectURIs) == 0 {
					t.Error("expected redirect_uris in response")
				}
			}
		})
	}
}

func TestAuthorize(t *testing.T) {
	// A valid base64url-encoded SHA256 hash is always exactly 43 chars.
	const validChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	const validClientID = "client-abc"
	const validRedirectURI = "https://chatgpt.com/callback"

	// okStore returns a registered client and successfully creates an auth code.
	okStore := &mockStore{
		getOAuthClient: func(_ context.Context, _ string) (*db.OAuthClient, error) {
			return &db.OAuthClient{
				ID:           validClientID,
				RedirectURIs: []string{validRedirectURI},
			}, nil
		},
		createAuthorizationCode: func(_ context.Context, _ db.CreateAuthCodeRequest) (*db.OAuthAuthorizationCode, error) {
			return &db.OAuthAuthorizationCode{Code: "code-xyz"}, nil
		},
	}

	validBody := `{"client_id":"` + validClientID + `","redirect_uri":"` + validRedirectURI + `","code_challenge":"` + validChallenge + `"}`

	tests := []struct {
		name       string
		userID     string
		body       string
		store      *mockStore
		wantStatus int
	}{
		{
			name:       "valid request returns 200 with code",
			userID:     "user-123",
			body:       validBody,
			store:      okStore,
			wantStatus: http.StatusOK,
		},
		{
			// /authorize requires a logged-in user — no JWT context means no user_id.
			name:       "missing user returns 401",
			userID:     "",
			body:       validBody,
			store:      okStore,
			wantStatus: http.StatusUnauthorized,
		},
		{
			// client_id not in DB — return 400 (not 404) to avoid leaking client existence.
			name:   "unknown client_id returns 400",
			userID: "user-123",
			body:   validBody,
			store: &mockStore{
				getOAuthClient: func(_ context.Context, _ string) (*db.OAuthClient, error) {
					return nil, db.ErrNotFound
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// redirect_uri must exactly match a registered URI — prevents open redirect.
			name:   "unregistered redirect_uri returns 400",
			userID: "user-123",
			body:   `{"client_id":"` + validClientID + `","redirect_uri":"https://evil.com/steal","code_challenge":"` + validChallenge + `"}`,
			store: &mockStore{
				getOAuthClient: func(_ context.Context, _ string) (*db.OAuthClient, error) {
					return &db.OAuthClient{
						ID:           validClientID,
						RedirectURIs: []string{validRedirectURI},
					}, nil
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// code_challenge must be base64url SHA256 — 43 chars, no padding.
			name:       "invalid code_challenge returns 400",
			userID:     "user-123",
			body:       `{"client_id":"` + validClientID + `","redirect_uri":"` + validRedirectURI + `","code_challenge":"not-a-valid-challenge"}`,
			store:      okStore,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := callAuthorize(tt.store, tt.userID, tt.body)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if resp["code"] == "" {
					t.Error("expected code in response")
				}
			}
		})
	}
}

func TestToken(t *testing.T) {
	// A real PKCE pair: verifier → SHA256 → base64url → challenge.
	// verifyPKCES256(verifier, challenge) == true
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	const clientID = "client-abc"
	const redirectURI = "https://chatgpt.com/callback"
	const userID = "user-123"

	okStore := &mockStore{
		consumeAuthorizationCode: func(_ context.Context, _, _ string) (*db.OAuthAuthorizationCode, error) {
			return &db.OAuthAuthorizationCode{
				Code:          "the-code",
				ClientID:      clientID,
				UserID:        userID,
				RedirectURI:   redirectURI,
				CodeChallenge: challenge,
			}, nil
		},
		createRefreshToken: func(_ context.Context, _ db.CreateRefreshTokenRequest) (*db.OAuthRefreshToken, error) {
			return &db.OAuthRefreshToken{Token: "refresh-xyz"}, nil
		},
	}

	validForm := "grant_type=authorization_code&code=the-code&code_verifier=" + verifier +
		"&client_id=" + clientID + "&redirect_uri=" + redirectURI

	callToken := func(store *mockStore, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/internal/oauth/token", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router := mux.NewRouter()
		NewHandler(store, testJWTSecret).RegisterRoutes(router)
		router.ServeHTTP(w, r)
		return w
	}

	tests := []struct {
		name       string
		body       string
		store      *mockStore
		wantStatus int
	}{
		{
			name:       "valid exchange returns access_token and refresh_token",
			body:       validForm,
			store:      okStore,
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong grant_type returns 400",
			body:       "grant_type=client_credentials&code=x&code_verifier=x&client_id=x&redirect_uri=x",
			store:      okStore,
			wantStatus: http.StatusBadRequest,
		},
		{
			// ConsumeAuthorizationCode returns ErrNotFound for expired/used/wrong-client codes.
			name: "invalid or expired code returns 400",
			body: validForm,
			store: &mockStore{
				consumeAuthorizationCode: func(_ context.Context, _, _ string) (*db.OAuthAuthorizationCode, error) {
					return nil, db.ErrNotFound
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// Wrong verifier → SHA256 hash doesn't match stored challenge.
			name: "wrong code_verifier returns 400",
			body: "grant_type=authorization_code&code=the-code&code_verifier=wrongverifier1234567890123456789012345678" +
				"&client_id=" + clientID + "&redirect_uri=" + redirectURI,
			store: &mockStore{
				consumeAuthorizationCode: func(_ context.Context, _, _ string) (*db.OAuthAuthorizationCode, error) {
					return &db.OAuthAuthorizationCode{
						ClientID: clientID, UserID: userID,
						RedirectURI: redirectURI, CodeChallenge: challenge,
					}, nil
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// redirect_uri in the request must match the one pinned on the code.
			name: "redirect_uri mismatch returns 400",
			body: "grant_type=authorization_code&code=the-code&code_verifier=" + verifier +
				"&client_id=" + clientID + "&redirect_uri=https://other.com/cb",
			store: &mockStore{
				consumeAuthorizationCode: func(_ context.Context, _, _ string) (*db.OAuthAuthorizationCode, error) {
					return &db.OAuthAuthorizationCode{
						ClientID: clientID, UserID: userID,
						RedirectURI: redirectURI, CodeChallenge: challenge,
					}, nil
				},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := callToken(tt.store, tt.body)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if tok, ok := resp["access_token"].(string); !ok || tok == "" {
					t.Error("expected non-empty access_token string in response")
				}
				if tok, ok := resp["refresh_token"].(string); !ok || tok == "" {
					t.Error("expected non-empty refresh_token string in response")
				}
				if typ, ok := resp["token_type"].(string); !ok || typ != "Bearer" {
					t.Errorf("token_type = %v, want Bearer", resp["token_type"])
				}
			}
		})
	}
}

func TestRefreshToken(t *testing.T) {
	const clientID = "client-abc"
	const userID = "user-123"

	okStore := &mockStore{
		// ConsumeRefreshToken atomically revokes the old token and returns its record.
		consumeRefreshToken: func(_ context.Context, token string) (*db.OAuthRefreshToken, error) {
			return &db.OAuthRefreshToken{Token: token, ClientID: clientID, UserID: userID}, nil
		},
		// CreateRefreshToken issues the new rotated token.
		createRefreshToken: func(_ context.Context, _ db.CreateRefreshTokenRequest) (*db.OAuthRefreshToken, error) {
			return &db.OAuthRefreshToken{Token: "new-refresh-token"}, nil
		},
	}

	callRefresh := func(store *mockStore, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/internal/oauth/token", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router := mux.NewRouter()
		NewHandler(store, testJWTSecret).RegisterRoutes(router)
		router.ServeHTTP(w, r)
		return w
	}

	tests := []struct {
		name       string
		body       string
		store      *mockStore
		wantStatus int
	}{
		{
			name:       "valid refresh returns new access_token and refresh_token",
			body:       "grant_type=refresh_token&refresh_token=old-refresh-token",
			store:      okStore,
			wantStatus: http.StatusOK,
		},
		{
			// A replayed or unknown token — ConsumeRefreshToken returns ErrNotFound.
			// This also fires when an attacker replays a previously rotated token.
			name: "revoked or unknown token returns 400",
			body: "grant_type=refresh_token&refresh_token=stolen-token",
			store: &mockStore{
				consumeRefreshToken: func(_ context.Context, _ string) (*db.OAuthRefreshToken, error) {
					return nil, db.ErrNotFound
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing refresh_token field returns 400",
			body:       "grant_type=refresh_token",
			store:      okStore,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "store error on consume returns 500",
			body: "grant_type=refresh_token&refresh_token=some-token",
			store: &mockStore{
				consumeRefreshToken: func(_ context.Context, _ string) (*db.OAuthRefreshToken, error) {
					return nil, errors.New("db unavailable")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := callRefresh(tt.store, tt.body)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if tok, ok := resp["access_token"].(string); !ok || tok == "" {
					t.Error("expected non-empty access_token in response")
				}
				// Rotation: the new refresh token must differ from the one we sent.
				if tok, ok := resp["refresh_token"].(string); !ok || tok == "" {
					t.Error("expected non-empty refresh_token in response")
				} else if tok == "old-refresh-token" {
					t.Error("refresh_token was not rotated — same token returned")
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Error("expected Cache-Control: no-store on token response")
				}
			}
		})
	}
}
