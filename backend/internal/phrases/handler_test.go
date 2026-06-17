package phrases

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/middleware"
	"github.com/gorilla/mux"
)

// testUserID is injected into every test request context so handlers
// receive a non-empty user ID and the mocks can assert it is passed through.
const testUserID = "550e8400-e29b-41d4-a716-446655440099"

// mockStore satisfies db.Store without a real database.
// Each test sets only the functions it needs; unset functions will panic if called.
type mockStore struct {
	createPhrase func(ctx context.Context, userID string, req db.CreatePhraseRequest) (*db.Phrase, error)
	listPhrases  func(ctx context.Context, userID string, headword string) ([]db.Phrase, error)
	getPhrase    func(ctx context.Context, userID string, id string) (*db.Phrase, error)
	deletePhrase func(ctx context.Context, userID string, id string) error
	updatePhrase func(ctx context.Context, userID string, id string, req db.UpdatePhraseRequest) (*db.Phrase, error)
}

func (m *mockStore) Close() {}
func (m *mockStore) CreatePhrase(ctx context.Context, userID string, req db.CreatePhraseRequest) (*db.Phrase, error) {
	return m.createPhrase(ctx, userID, req)
}
func (m *mockStore) ListPhrases(ctx context.Context, userID string, headword string) ([]db.Phrase, error) {
	return m.listPhrases(ctx, userID, headword)
}
func (m *mockStore) GetPhrase(ctx context.Context, userID string, id string) (*db.Phrase, error) {
	return m.getPhrase(ctx, userID, id)
}
func (m *mockStore) DeletePhrase(ctx context.Context, userID string, id string) error {
	return m.deletePhrase(ctx, userID, id)
}
func (m *mockStore) UpdatePhrase(ctx context.Context, userID string, id string, req db.UpdatePhraseRequest) (*db.Phrase, error) {
	return m.updatePhrase(ctx, userID, id, req)
}

// Auth methods — not used in phrase tests; panic if called unexpectedly.
func (m *mockStore) UpsertUser(_ context.Context, _ string) (*db.User, error) {
	panic("UpsertUser not expected in phrase tests")
}
func (m *mockStore) CreateMagicLinkToken(_ context.Context, _ string, _ time.Time) (*db.MagicLinkToken, error) {
	panic("CreateMagicLinkToken not expected in phrase tests")
}
func (m *mockStore) GetMagicLinkToken(_ context.Context, _ string) (*db.MagicLinkToken, error) {
	panic("GetMagicLinkToken not expected in phrase tests")
}
func (m *mockStore) MarkTokenUsed(_ context.Context, _ string) error {
	panic("MarkTokenUsed not expected in phrase tests")
}
func (m *mockStore) CreateOAuthClient(_ context.Context, _ []string) (*db.OAuthClient, error) {
	panic("not expected in phrase tests")
}
func (m *mockStore) GetOAuthClient(_ context.Context, _ string) (*db.OAuthClient, error) {
	panic("not expected in phrase tests")
}
func (m *mockStore) CreateAuthorizationCode(_ context.Context, _ db.CreateAuthCodeRequest) (*db.OAuthAuthorizationCode, error) {
	panic("not expected in phrase tests")
}
func (m *mockStore) ConsumeAuthorizationCode(_ context.Context, _, _ string) (*db.OAuthAuthorizationCode, error) {
	panic("not expected in phrase tests")
}
func (m *mockStore) CreateRefreshToken(_ context.Context, _ db.CreateRefreshTokenRequest) (*db.OAuthRefreshToken, error) {
	panic("not expected in phrase tests")
}
func (m *mockStore) ConsumeRefreshToken(_ context.Context, _, _ string) (*db.OAuthRefreshToken, error) {
	panic("not expected in phrase tests")
}
func (m *mockStore) RevokeRefreshTokens(_ context.Context, _, _ string) error {
	panic("not expected in phrase tests")
}

// newTestServer wires a Handler with the given store and returns a test HTTP server.
// It injects testUserID into every request context so UserIDFromContext returns a
// non-empty value — exercising the actual scoping logic in each handler.
func newTestServer(store db.Store) *httptest.Server {
	r := mux.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), middleware.UserIDKey, testUserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	NewHandler(store).RegisterRoutes(r)
	return httptest.NewServer(r)
}

func TestListPhrases_ReturnsAll(t *testing.T) {
	store := &mockStore{
		listPhrases: func(_ context.Context, userID string, headword string) ([]db.Phrase, error) {
			if userID != testUserID {
				t.Errorf("expected userID %q, got %q", testUserID, userID)
			}
			return []db.Phrase{
				{ID: "1", Phrase: "It was serendipitous.", Headwords: []string{"serendipitous"}},
				{ID: "2", Phrase: "A fortuitous meeting.", Headwords: []string{"fortuitous"}},
			}, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/phrases")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got []db.Phrase
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 phrases, got %d", len(got))
	}
}

func TestListPhrases_EmptyReturnsArray(t *testing.T) {
	store := &mockStore{
		listPhrases: func(_ context.Context, userID string, _ string) ([]db.Phrase, error) {
			return []db.Phrase{}, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/phrases")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Must return [] not null so the frontend can always iterate the response
	var got []db.Phrase
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got == nil {
		t.Error("expected empty array, got null")
	}
}

func TestListPhrases_HeadwordFilter(t *testing.T) {
	store := &mockStore{
		listPhrases: func(_ context.Context, userID string, headword string) ([]db.Phrase, error) {
			if userID != testUserID {
				t.Errorf("expected userID %q, got %q", testUserID, userID)
			}
			if headword != "serendipitous" {
				t.Errorf("expected headword %q, got %q", "serendipitous", headword)
			}
			return []db.Phrase{
				{ID: "1", Phrase: "It was serendipitous.", Headwords: []string{"serendipitous"}},
			}, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/phrases?headword=serendipitous")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

const validUUID = "550e8400-e29b-41d4-a716-446655440000"

func TestGetPhrase_Success(t *testing.T) {
	store := &mockStore{
		getPhrase: func(_ context.Context, userID string, id string) (*db.Phrase, error) {
			if userID != testUserID {
				t.Errorf("expected userID %q, got %q", testUserID, userID)
			}
			return &db.Phrase{ID: id, Phrase: "It was serendipitous.", Headwords: []string{"serendipitous"}}, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/phrases/" + validUUID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got db.Phrase
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != validUUID {
		t.Errorf("expected id %q, got %q", validUUID, got.ID)
	}
}

func TestGetPhrase_NotFound(t *testing.T) {
	store := &mockStore{
		getPhrase: func(_ context.Context, userID string, _ string) (*db.Phrase, error) {
			return nil, db.ErrNotFound
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/phrases/" + validUUID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetPhrase_InvalidID(t *testing.T) {
	store := &mockStore{
		getPhrase: func(_ context.Context, userID string, _ string) (*db.Phrase, error) {
			t.Error("store should not be called for an invalid UUID")
			return nil, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/phrases/not-a-uuid")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestUpdatePhrase_SourceURLsOnly(t *testing.T) {
	store := &mockStore{
		updatePhrase: func(_ context.Context, userID string, _ string, req db.UpdatePhraseRequest) (*db.Phrase, error) {
			return &db.Phrase{ID: validUUID, SourceURLs: req.SourceURLs}, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	// Sending only source_urls should be accepted — not rejected as "no fields provided"
	body := `{"source_urls":["https://www.merriam-webster.com/dictionary/test"]}`
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/phrases/"+validUUID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestUpdatePhrase_Success(t *testing.T) {
	updated := "updated note"
	store := &mockStore{
		updatePhrase: func(_ context.Context, userID string, _ string, req db.UpdatePhraseRequest) (*db.Phrase, error) {
			return &db.Phrase{ID: validUUID, Phrase: "It was serendipitous.", Headwords: []string{"serendipitous"}, Note: *req.Note}, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	body := `{"note":"` + updated + `"}`
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/phrases/"+validUUID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got db.Phrase
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Note != updated {
		t.Errorf("expected note %q, got %q", updated, got.Note)
	}
}

func TestUpdatePhrase_EmptyBody(t *testing.T) {
	store := &mockStore{
		updatePhrase: func(_ context.Context, userID string, _ string, _ db.UpdatePhraseRequest) (*db.Phrase, error) {
			t.Error("store should not be called when no fields provided")
			return nil, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/phrases/"+validUUID, bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestUpdatePhrase_EmptyFieldValues(t *testing.T) {
	store := &mockStore{
		updatePhrase: func(_ context.Context, userID string, _ string, _ db.UpdatePhraseRequest) (*db.Phrase, error) {
			t.Error("store should not be called when provided fields are empty strings")
			return nil, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	tests := []struct {
		name string
		body string
	}{
		{"empty phrase", `{"phrase":""}`},
		{"empty headwords", `{"headwords":[]}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/phrases/"+validUUID, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestUpdatePhrase_NotFound(t *testing.T) {
	store := &mockStore{
		updatePhrase: func(_ context.Context, userID string, _ string, _ db.UpdatePhraseRequest) (*db.Phrase, error) {
			return nil, db.ErrNotFound
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/phrases/"+validUUID, bytes.NewBufferString(`{"note":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeletePhrase_Success(t *testing.T) {
	store := &mockStore{
		deletePhrase: func(_ context.Context, userID string, _ string) error { return nil },
	}

	srv := newTestServer(store)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/phrases/"+validUUID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestDeletePhrase_NotFound(t *testing.T) {
	store := &mockStore{
		deletePhrase: func(_ context.Context, userID string, _ string) error { return db.ErrNotFound },
	}

	srv := newTestServer(store)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/phrases/"+validUUID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeletePhrase_InvalidID(t *testing.T) {
	store := &mockStore{
		deletePhrase: func(_ context.Context, userID string, _ string) error {
			t.Error("store should not be called for an invalid UUID")
			return nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/phrases/not-a-uuid", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreatePhrase_Success(t *testing.T) {
	store := &mockStore{
		createPhrase: func(_ context.Context, userID string, req db.CreatePhraseRequest) (*db.Phrase, error) {
			if userID != testUserID {
				t.Errorf("expected userID %q, got %q", testUserID, userID)
			}
			// Return a phrase that mirrors what Postgres would give back
			return &db.Phrase{
				ID:        "some-uuid",
				Phrase:    req.Phrase,
				Headwords: req.Headwords,
				Note:      req.Note,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	body := `{"phrase":"It was serendipitous.","headwords":["serendipitous"],"note":"A happy accident."}`
	resp, err := http.Post(srv.URL+"/api/v1/phrases", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var got db.Phrase
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Headwords[0] != "serendipitous" {
		t.Errorf("expected headword %q, got %q", "serendipitous", got.Headwords[0])
	}
}

func TestCreatePhrase_MissingFields(t *testing.T) {
	// Store should never be called when validation fails
	store := &mockStore{
		createPhrase: func(_ context.Context, userID string, _ db.CreatePhraseRequest) (*db.Phrase, error) {
			t.Error("store should not be called on invalid input")
			return nil, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	tests := []struct {
		name string
		body string
	}{
		{"missing phrase", `{"headwords":["serendipitous"]}`},
		{"missing headwords", `{"phrase":"It was serendipitous."}`},
		{"empty body", `{}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/api/v1/phrases", "application/json", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestCreatePhrase_InvalidJSON(t *testing.T) {
	store := &mockStore{
		createPhrase: func(_ context.Context, userID string, _ db.CreatePhraseRequest) (*db.Phrase, error) {
			t.Error("store should not be called on invalid JSON")
			return nil, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/phrases", "application/json", bytes.NewBufferString(`not json`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreatePhrase_UnknownFields(t *testing.T) {
	store := &mockStore{
		createPhrase: func(_ context.Context, userID string, _ db.CreatePhraseRequest) (*db.Phrase, error) {
			t.Error("store should not be called when unknown fields are present")
			return nil, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	body := `{"phrase":"It was serendipitous.","headwords":["serendipitous"],"unknown_field":"oops"}`
	resp, err := http.Post(srv.URL+"/api/v1/phrases", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreatePhrase_BodyTooLarge(t *testing.T) {
	store := &mockStore{
		createPhrase: func(_ context.Context, userID string, _ db.CreatePhraseRequest) (*db.Phrase, error) {
			t.Error("store should not be called when body exceeds size limit")
			return nil, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	// Build a payload larger than maxBodyBytes (1 KB)
	oversized := `{"phrase":"` + string(make([]byte, 2048)) + `","headwords":["test"]}`
	resp, err := http.Post(srv.URL+"/api/v1/phrases", "application/json", bytes.NewBufferString(oversized))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}
