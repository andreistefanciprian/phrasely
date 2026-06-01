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
	"github.com/gorilla/mux"
)

// mockStore satisfies db.Store without a real database.
// Each test sets createPhrase to control what the store returns.
type mockStore struct {
	createPhrase func(ctx context.Context, req db.CreatePhraseRequest) (*db.Phrase, error)
}

func (m *mockStore) Close() {}
func (m *mockStore) CreatePhrase(ctx context.Context, req db.CreatePhraseRequest) (*db.Phrase, error) {
	return m.createPhrase(ctx, req)
}

// newTestServer wires a Handler with the given store and returns a test HTTP server.
func newTestServer(store db.Store) *httptest.Server {
	r := mux.NewRouter()
	NewHandler(store).RegisterRoutes(r)
	return httptest.NewServer(r)
}

func TestCreatePhrase_Success(t *testing.T) {
	store := &mockStore{
		createPhrase: func(_ context.Context, req db.CreatePhraseRequest) (*db.Phrase, error) {
			// Return a phrase that mirrors what Postgres would give back
			return &db.Phrase{
				ID:        "some-uuid",
				Phrase:    req.Phrase,
				Keyword:   req.Keyword,
				Note:      req.Note,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	body := `{"phrase":"It was serendipitous.","keyword":"serendipitous","note":"A happy accident."}`
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
	if got.Keyword != "serendipitous" {
		t.Errorf("expected keyword %q, got %q", "serendipitous", got.Keyword)
	}
}

func TestCreatePhrase_MissingFields(t *testing.T) {
	// Store should never be called when validation fails
	store := &mockStore{
		createPhrase: func(_ context.Context, _ db.CreatePhraseRequest) (*db.Phrase, error) {
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
		{"missing phrase", `{"keyword":"serendipitous"}`},
		{"missing keyword", `{"phrase":"It was serendipitous."}`},
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
		createPhrase: func(_ context.Context, _ db.CreatePhraseRequest) (*db.Phrase, error) {
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
