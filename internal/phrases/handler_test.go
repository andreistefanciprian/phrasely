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
// Each test sets only the functions it needs; unset functions will panic if called.
type mockStore struct {
	createPhrase func(ctx context.Context, req db.CreatePhraseRequest) (*db.Phrase, error)
	listPhrases  func(ctx context.Context, keyword string) ([]db.Phrase, error)
	getPhrase    func(ctx context.Context, id string) (*db.Phrase, error)
	deletePhrase func(ctx context.Context, id string) error
	updatePhrase func(ctx context.Context, id string, req db.UpdatePhraseRequest) (*db.Phrase, error)
}

func (m *mockStore) Close() {}
func (m *mockStore) CreatePhrase(ctx context.Context, req db.CreatePhraseRequest) (*db.Phrase, error) {
	return m.createPhrase(ctx, req)
}
func (m *mockStore) ListPhrases(ctx context.Context, keyword string) ([]db.Phrase, error) {
	return m.listPhrases(ctx, keyword)
}
func (m *mockStore) GetPhrase(ctx context.Context, id string) (*db.Phrase, error) {
	return m.getPhrase(ctx, id)
}
func (m *mockStore) DeletePhrase(ctx context.Context, id string) error {
	return m.deletePhrase(ctx, id)
}
func (m *mockStore) UpdatePhrase(ctx context.Context, id string, req db.UpdatePhraseRequest) (*db.Phrase, error) {
	return m.updatePhrase(ctx, id, req)
}

// newTestServer wires a Handler with the given store and returns a test HTTP server.
func newTestServer(store db.Store) *httptest.Server {
	r := mux.NewRouter()
	NewHandler(store).RegisterRoutes(r)
	return httptest.NewServer(r)
}

func TestListPhrases_ReturnsAll(t *testing.T) {
	store := &mockStore{
		listPhrases: func(_ context.Context, keyword string) ([]db.Phrase, error) {
			return []db.Phrase{
				{ID: "1", Phrase: "It was serendipitous.", Keyword: "serendipitous"},
				{ID: "2", Phrase: "A fortuitous meeting.", Keyword: "fortuitous"},
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
		listPhrases: func(_ context.Context, _ string) ([]db.Phrase, error) {
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

func TestListPhrases_KeywordFilter(t *testing.T) {
	store := &mockStore{
		listPhrases: func(_ context.Context, keyword string) ([]db.Phrase, error) {
			if keyword != "serendipitous" {
				t.Errorf("expected keyword %q, got %q", "serendipitous", keyword)
			}
			return []db.Phrase{
				{ID: "1", Phrase: "It was serendipitous.", Keyword: "serendipitous"},
			}, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/phrases?keyword=serendipitous")
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
		getPhrase: func(_ context.Context, id string) (*db.Phrase, error) {
			return &db.Phrase{ID: id, Phrase: "It was serendipitous.", Keyword: "serendipitous"}, nil
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
		getPhrase: func(_ context.Context, _ string) (*db.Phrase, error) {
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
		getPhrase: func(_ context.Context, _ string) (*db.Phrase, error) {
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

func TestUpdatePhrase_Success(t *testing.T) {
	updated := "updated note"
	store := &mockStore{
		updatePhrase: func(_ context.Context, _ string, req db.UpdatePhraseRequest) (*db.Phrase, error) {
			return &db.Phrase{ID: validUUID, Phrase: "It was serendipitous.", Keyword: "serendipitous", Note: *req.Note}, nil
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
		updatePhrase: func(_ context.Context, _ string, _ db.UpdatePhraseRequest) (*db.Phrase, error) {
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

func TestUpdatePhrase_NotFound(t *testing.T) {
	store := &mockStore{
		updatePhrase: func(_ context.Context, _ string, _ db.UpdatePhraseRequest) (*db.Phrase, error) {
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
		deletePhrase: func(_ context.Context, _ string) error { return nil },
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
		deletePhrase: func(_ context.Context, _ string) error { return db.ErrNotFound },
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
		deletePhrase: func(_ context.Context, _ string) error {
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

func TestCreatePhrase_UnknownFields(t *testing.T) {
	store := &mockStore{
		createPhrase: func(_ context.Context, _ db.CreatePhraseRequest) (*db.Phrase, error) {
			t.Error("store should not be called when unknown fields are present")
			return nil, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	body := `{"phrase":"It was serendipitous.","keyword":"serendipitous","unknown_field":"oops"}`
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
		createPhrase: func(_ context.Context, _ db.CreatePhraseRequest) (*db.Phrase, error) {
			t.Error("store should not be called when body exceeds size limit")
			return nil, nil
		},
	}

	srv := newTestServer(store)
	defer srv.Close()

	// Build a payload larger than maxBodyBytes (1 KB)
	oversized := `{"phrase":"` + string(make([]byte, 2048)) + `","keyword":"test"}`
	resp, err := http.Post(srv.URL+"/api/v1/phrases", "application/json", bytes.NewBufferString(oversized))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}
