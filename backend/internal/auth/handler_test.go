package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/email"
	"github.com/gorilla/mux"
)

const (
	testSecret   = "test-secret"
	testUserID   = "550e8400-e29b-41d4-a716-446655440001"
	testTokenID  = "550e8400-e29b-41d4-a716-446655440002"
	testTokenVal = "550e8400-e29b-41d4-a716-446655440003"
)

// mockStore satisfies db.Store for auth tests.
// Only the auth methods are wired; phrase methods panic if called.
type mockStore struct {
	getMagicLinkToken    func(ctx context.Context, token string) (*db.MagicLinkToken, error)
	markTokenUsed        func(ctx context.Context, tokenID string) error
	upsertUser           func(ctx context.Context, email string) (*db.User, error)
	createMagicLinkToken func(ctx context.Context, userID string, expiresAt time.Time) (*db.MagicLinkToken, error)
}

func (m *mockStore) Close() {}
func (m *mockStore) UpsertUser(ctx context.Context, email string) (*db.User, error) {
	return m.upsertUser(ctx, email)
}
func (m *mockStore) CreateMagicLinkToken(ctx context.Context, userID string, expiresAt time.Time) (*db.MagicLinkToken, error) {
	if m.createMagicLinkToken != nil {
		return m.createMagicLinkToken(ctx, userID, expiresAt)
	}
	panic("not expected in auth tests")
}
func (m *mockStore) GetMagicLinkToken(ctx context.Context, token string) (*db.MagicLinkToken, error) {
	return m.getMagicLinkToken(ctx, token)
}
func (m *mockStore) MarkTokenUsed(ctx context.Context, tokenID string) error {
	return m.markTokenUsed(ctx, tokenID)
}
func (m *mockStore) CreatePhrase(_ context.Context, _ string, _ db.CreatePhraseRequest) (*db.Phrase, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) ListPhrases(_ context.Context, _ string, _ string) ([]db.Phrase, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) GetPhrase(_ context.Context, _ string, _ string) (*db.Phrase, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) DeletePhrase(_ context.Context, _ string, _ string) error {
	panic("not expected in auth tests")
}
func (m *mockStore) UpdatePhrase(_ context.Context, _ string, _ string, _ db.UpdatePhraseRequest) (*db.Phrase, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) CreateOAuthClient(_ context.Context, _ []string) (*db.OAuthClient, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) GetOAuthClient(_ context.Context, _ string) (*db.OAuthClient, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) CreateAuthorizationCode(_ context.Context, _ db.CreateAuthCodeRequest) (*db.OAuthAuthorizationCode, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) ConsumeAuthorizationCode(_ context.Context, _, _ string) (*db.OAuthAuthorizationCode, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) CreateRefreshToken(_ context.Context, _ db.CreateRefreshTokenRequest) (*db.OAuthRefreshToken, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) ConsumeRefreshToken(_ context.Context, _, _ string) (*db.OAuthRefreshToken, error) {
	panic("not expected in auth tests")
}
func (m *mockStore) RevokeRefreshTokens(_ context.Context, _, _ string) error {
	panic("not expected in auth tests")
}
func (m *mockStore) SetPhraseEmbedding(_ context.Context, _ string, _ []float32) error {
	panic("not expected in auth tests")
}
func (m *mockStore) ListPhrasesWithoutEmbedding(_ context.Context) ([]db.Phrase, error) {
	panic("not expected in auth tests")
}

// spySender records calls to SendMagicLink for assertions.
type spySender struct {
	called bool
	to     string
	err    error // error to return
}

func (s *spySender) SendMagicLink(to, _ string) error {
	s.called = true
	s.to = to
	return s.err
}

func newTestServer(store db.Store) *httptest.Server {
	r := mux.NewRouter()
	NewHandler(store, "http://localhost:8080", testSecret, &email.LogSender{}, 15*time.Minute, 30*24*time.Hour).RegisterRoutes(r)
	return httptest.NewServer(r)
}

func validToken() *db.MagicLinkToken {
	return &db.MagicLinkToken{
		ID:        testTokenID,
		UserID:    testUserID,
		Token:     testTokenVal,
		ExpiresAt: time.Now().Add(15 * time.Minute),
		UsedAt:    nil,
	}
}

func TestVerify_Success(t *testing.T) {
	store := &mockStore{
		getMagicLinkToken: func(_ context.Context, _ string) (*db.MagicLinkToken, error) {
			return validToken(), nil
		},
		markTokenUsed: func(_ context.Context, _ string) error { return nil },
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/auth/verify?token=" + testTokenVal)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["token"] == "" {
		t.Error("expected a JWT token in response")
	}
}

func TestVerify_TokenNotFound(t *testing.T) {
	store := &mockStore{
		getMagicLinkToken: func(_ context.Context, _ string) (*db.MagicLinkToken, error) {
			return nil, db.ErrNotFound
		},
		markTokenUsed: func(_ context.Context, _ string) error { panic("should not be called") },
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/auth/verify?token=" + testTokenVal)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestVerify_TokenAlreadyUsed(t *testing.T) {
	usedAt := time.Now().Add(-1 * time.Minute)
	store := &mockStore{
		getMagicLinkToken: func(_ context.Context, _ string) (*db.MagicLinkToken, error) {
			tok := validToken()
			tok.UsedAt = &usedAt
			return tok, nil
		},
		markTokenUsed: func(_ context.Context, _ string) error { panic("should not be called") },
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/auth/verify?token=" + testTokenVal)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestVerify_TokenExpired(t *testing.T) {
	store := &mockStore{
		getMagicLinkToken: func(_ context.Context, _ string) (*db.MagicLinkToken, error) {
			tok := validToken()
			tok.ExpiresAt = time.Now().Add(-1 * time.Minute)
			return tok, nil
		},
		markTokenUsed: func(_ context.Context, _ string) error { panic("should not be called") },
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/auth/verify?token=" + testTokenVal)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestVerify_InvalidUUID(t *testing.T) {
	store := &mockStore{
		getMagicLinkToken: func(_ context.Context, _ string) (*db.MagicLinkToken, error) {
			panic("should not be called for non-UUID token")
		},
		markTokenUsed: func(_ context.Context, _ string) error { panic("should not be called") },
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/auth/verify?token=not-a-uuid")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestVerify_MissingToken(t *testing.T) {
	srv := newTestServer(&mockStore{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/auth/verify")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestVerify_RaceConditionAlreadyConsumed(t *testing.T) {
	// MarkTokenUsed returns ErrNotFound — simulates a concurrent request
	// consuming the token between our SELECT and UPDATE.
	store := &mockStore{
		getMagicLinkToken: func(_ context.Context, _ string) (*db.MagicLinkToken, error) {
			return validToken(), nil
		},
		markTokenUsed: func(_ context.Context, _ string) error {
			return db.ErrNotFound
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/auth/verify?token=" + testTokenVal)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func newTestServerWithMailer(store db.Store, mailer email.Sender) *httptest.Server {
	r := mux.NewRouter()
	NewHandler(store, "http://localhost:8080", testSecret, mailer, 15*time.Minute, 30*24*time.Hour).RegisterRoutes(r)
	return httptest.NewServer(r)
}

func requestStore() *mockStore {
	return &mockStore{
		upsertUser: func(_ context.Context, _ string) (*db.User, error) {
			return &db.User{ID: testUserID, Email: "user@example.com"}, nil
		},
		createMagicLinkToken: func(_ context.Context, _ string, _ time.Time) (*db.MagicLinkToken, error) {
			return validToken(), nil
		},
		getMagicLinkToken: func(_ context.Context, _ string) (*db.MagicLinkToken, error) {
			return validToken(), nil
		},
		markTokenUsed: func(_ context.Context, _ string) error { return nil },
	}
}

func TestRequest_MailerCalled(t *testing.T) {
	spy := &spySender{}
	srv := newTestServerWithMailer(requestStore(), spy)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/auth/request", "application/json",
		strings.NewReader(`{"email":"user@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !spy.called {
		t.Error("expected mailer.SendMagicLink to be called")
	}
	if spy.to != "user@example.com" {
		t.Errorf("expected to=user@example.com, got %q", spy.to)
	}
}

func TestRequest_MailerErrorStillReturns200(t *testing.T) {
	// A mailer error should not block the user — handler still responds 200
	spy := &spySender{err: fmt.Errorf("smtp timeout")}
	srv := newTestServerWithMailer(requestStore(), spy)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/auth/request", "application/json",
		strings.NewReader(`{"email":"user@example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 even on mailer error, got %d", resp.StatusCode)
	}
}

func TestRequest_MissingEmail(t *testing.T) {
	srv := newTestServer(&mockStore{upsertUser: func(_ context.Context, _ string) (*db.User, error) {
		panic("should not be called")
	}})
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/auth/request", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}
