package waitlist

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/gorilla/mux"
)

// mockStore satisfies db.Store for waitlist tests.
// Only AddWaitlistSignup is wired; every other method panics if called.
type mockStore struct {
	addWaitlistSignup func(ctx context.Context, email, source string) error
}

func (m *mockStore) Close() {}
func (m *mockStore) AddWaitlistSignup(ctx context.Context, email, source string) error {
	return m.addWaitlistSignup(ctx, email, source)
}
func (m *mockStore) CreatePhrase(_ context.Context, _ string, _ db.CreatePhraseRequest) (*db.Phrase, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) ListPhrases(_ context.Context, _ string, _ string) ([]db.Phrase, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) ListPhrasesSummary(_ context.Context, _ string, _ string) ([]db.PhraseSummary, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) GetRandomPhrases(_ context.Context, _ string, _ int) ([]db.PhraseSummary, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) GetPhrase(_ context.Context, _ string, _ string) (*db.Phrase, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) DeletePhrase(_ context.Context, _ string, _ string) error {
	panic("not expected in waitlist tests")
}
func (m *mockStore) UpdatePhrase(_ context.Context, _ string, _ string, _ db.UpdatePhraseRequest) (*db.Phrase, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) SetPhraseEmbedding(_ context.Context, _ string, _ []float32) error {
	panic("not expected in waitlist tests")
}
func (m *mockStore) ListPhrasesWithoutEmbedding(_ context.Context) ([]db.Phrase, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) SearchPhrasesBySimilarity(_ context.Context, _ string, _ []float32, _ int) ([]db.Phrase, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) GetRelatedPhrases(_ context.Context, _ string, _ string, _ float64, _ int) ([]db.Phrase, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) UpsertUser(_ context.Context, _ string) (*db.User, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) CreateMagicLinkToken(_ context.Context, _ string, _ time.Time) (*db.MagicLinkToken, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) GetMagicLinkToken(_ context.Context, _ string) (*db.MagicLinkToken, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) MarkTokenUsed(_ context.Context, _ string) error {
	panic("not expected in waitlist tests")
}
func (m *mockStore) CreateOAuthClient(_ context.Context, _ []string) (*db.OAuthClient, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) GetOAuthClient(_ context.Context, _ string) (*db.OAuthClient, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) CreateAuthorizationCode(_ context.Context, _ db.CreateAuthCodeRequest) (*db.OAuthAuthorizationCode, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) ConsumeAuthorizationCode(_ context.Context, _, _ string) (*db.OAuthAuthorizationCode, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) CreateRefreshToken(_ context.Context, _ db.CreateRefreshTokenRequest) (*db.OAuthRefreshToken, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) ConsumeRefreshToken(_ context.Context, _, _ string) (*db.OAuthRefreshToken, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) RevokeRefreshTokens(_ context.Context, _, _ string) error {
	panic("not expected in waitlist tests")
}
func (m *mockStore) GetDigestPreferences(_ context.Context, _ string) (*db.DigestPreferences, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) UpsertDigestPreferences(_ context.Context, _ string, _ string) (*db.DigestPreferences, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) ListDigestRecipients(_ context.Context) ([]db.DigestRecipient, error) {
	panic("not expected in waitlist tests")
}
func (m *mockStore) MarkDigestSent(_ context.Context, _ string, _ time.Time) error {
	panic("not expected in waitlist tests")
}

func doRequest(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := mux.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/waitlist", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestJoin_Success(t *testing.T) {
	var gotEmail, gotSource string
	store := &mockStore{
		addWaitlistSignup: func(_ context.Context, email, source string) error {
			gotEmail, gotSource = email, source
			return nil
		},
	}
	h := NewHandler(store)

	rec := doRequest(t, h, `{"email":" Jane@Example.com ","source":"hero"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotEmail != "jane@example.com" {
		t.Errorf("email = %q, want %q (should be trimmed + lowercased)", gotEmail, "jane@example.com")
	}
	if gotSource != "hero" {
		t.Errorf("source = %q, want %q", gotSource, "hero")
	}
}

func TestJoin_DefaultsSourceWhenMissing(t *testing.T) {
	var gotSource string
	store := &mockStore{
		addWaitlistSignup: func(_ context.Context, _, source string) error {
			gotSource = source
			return nil
		},
	}
	h := NewHandler(store)

	rec := doRequest(t, h, `{"email":"jane@example.com"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if gotSource != "unknown" {
		t.Errorf("source = %q, want %q", gotSource, "unknown")
	}
}

func TestJoin_InvalidEmail(t *testing.T) {
	store := &mockStore{
		addWaitlistSignup: func(_ context.Context, _, _ string) error {
			t.Fatal("AddWaitlistSignup should not be called for an invalid email")
			return nil
		},
	}
	h := NewHandler(store)

	rec := doRequest(t, h, `{"email":"not-an-email"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestJoin_RepeatSubmissionStillReturns200(t *testing.T) {
	// ON CONFLICT DO NOTHING makes this a no-op at the DB layer; the store
	// method returns nil either way, and the handler must not treat that
	// as an error.
	store := &mockStore{
		addWaitlistSignup: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	h := NewHandler(store)

	rec := doRequest(t, h, `{"email":"jane@example.com","source":"footer"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status field = %q, want %q", resp["status"], "ok")
	}
}
