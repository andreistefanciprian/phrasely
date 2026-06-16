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
	"github.com/gorilla/mux"
)

// mockStore satisfies db.Store for oauth handler tests.
type mockStore struct {
	createOAuthClient func(ctx context.Context, redirectURIs []string) (*db.OAuthClient, error)
}

func (m *mockStore) Close() {}
func (m *mockStore) CreateOAuthClient(ctx context.Context, redirectURIs []string) (*db.OAuthClient, error) {
	return m.createOAuthClient(ctx, redirectURIs)
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
func (m *mockStore) DeletePhrase(_ context.Context, _ string, _ string) error       { panic("not expected") }
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
func (m *mockStore) GetOAuthClient(_ context.Context, _ string) (*db.OAuthClient, error) {
	panic("not expected")
}
func (m *mockStore) CreateAuthorizationCode(_ context.Context, _ db.CreateAuthCodeRequest) (*db.OAuthAuthorizationCode, error) {
	panic("not expected")
}
func (m *mockStore) ConsumeAuthorizationCode(_ context.Context, _, _ string) (*db.OAuthAuthorizationCode, error) {
	panic("not expected")
}
func (m *mockStore) CreateRefreshToken(_ context.Context, _ db.CreateRefreshTokenRequest) (*db.OAuthRefreshToken, error) {
	panic("not expected")
}
func (m *mockStore) ConsumeRefreshToken(_ context.Context, _ string) (*db.OAuthRefreshToken, error) {
	panic("not expected")
}

func newTestServer(store db.Store) *httptest.Server {
	r := mux.NewRouter()
	NewHandler(store).RegisterRoutes(r)
	return httptest.NewServer(r)
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
