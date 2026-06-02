package phrases

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// Handler holds a reference to the store interface, not the concrete Postgres type.
// This allows tests to inject a mock store without a real database.
type Handler struct {
	store db.Store
}

func NewHandler(store db.Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes attaches phrase endpoints to the given router.
func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/api/v1/phrases", h.list).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/phrases", h.create).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/phrases/{id}", h.get).Methods(http.MethodGet)
}

// get handles GET /api/v1/phrases/{id}.
// Returns 404 if the phrase does not exist.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	phrase, err := h.store.GetPhrase(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusNotFound, "phrase not found")
			return
		}
		slog.Error("get phrase", "id", id, "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to get phrase")
		return
	}

	respond(w, http.StatusOK, phrase)
}

// list handles GET /api/v1/phrases.
// Accepts an optional ?keyword= query param for filtering by keyword.
// Always returns a JSON array — empty array when there are no results.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")

	phrases, err := h.store.ListPhrases(r.Context(), keyword)
	if err != nil {
		slog.Error("list phrases", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to list phrases")
		return
	}

	respond(w, http.StatusOK, phrases)
}

// maxBodyBytes is the maximum request body size we accept (1 KB).
// A phrase, keyword, and note easily fit within this; anything larger is rejected.
const maxBodyBytes = 1024

// create handles POST /api/v1/phrases.
// It decodes the request body, validates required fields, and inserts the phrase.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	// Cap the body size before reading to prevent oversized payloads.
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	// Reject requests that send fields not defined in CreatePhraseRequest.
	// Catches typos early and avoids silently ignoring unexpected input.
	dec.DisallowUnknownFields()

	var req db.CreatePhraseRequest
	if err := dec.Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// phrase and keyword are required; note is optional
	if req.Phrase == "" || req.Keyword == "" {
		respondErr(w, http.StatusBadRequest, "phrase and keyword are required")
		return
	}

	phrase, err := h.store.CreatePhrase(r.Context(), req)
	if err != nil {
		slog.Error("create phrase", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to create phrase")
		return
	}

	respond(w, http.StatusCreated, phrase)
}

// parseID extracts and validates the {id} path variable as a UUID.
// It writes a 400 response and returns false if the value is not a valid UUID,
// so callers can return immediately without hitting the database.
func parseID(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := mux.Vars(r)["id"]
	if _, err := uuid.Parse(raw); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid id: must be a UUID")
		return "", false
	}
	return raw, true
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func respondErr(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}
