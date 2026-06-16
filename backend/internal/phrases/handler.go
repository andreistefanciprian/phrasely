package phrases

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/middleware"
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
	r.HandleFunc("/api/v1/phrases/{id}", h.update).Methods(http.MethodPatch)
	r.HandleFunc("/api/v1/phrases/{id}", h.delete).Methods(http.MethodDelete)
}

// list handles GET /api/v1/phrases.
// Accepts an optional ?headword= query param for filtering by headword.
// Always returns a JSON array — empty array when there are no results.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	headword := r.URL.Query().Get("headword")

	phrases, err := h.store.ListPhrases(r.Context(), userID, headword)
	if err != nil {
		slog.Error("list phrases", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to list phrases")
		return
	}

	slog.Debug("list phrases", "count", len(phrases), "headword", headword)
	respond(w, http.StatusOK, phrases)
}

// get handles GET /api/v1/phrases/{id}.
// Returns 404 if the phrase does not exist or belongs to another user.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	phrase, err := h.store.GetPhrase(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusNotFound, "phrase not found")
			return
		}
		slog.Error("get phrase", "id", id, "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to get phrase")
		return
	}

	slog.Debug("get phrase", "id", id)
	respond(w, http.StatusOK, phrase)
}

// maxBodyBytes is the maximum request body size we accept (1 KB).
// A phrase, headword, and note easily fit within this; anything larger is rejected.
// Note: blank-string headwords (e.g. [""]) are currently accepted; revisit if it
// causes issues with curation or display.
const maxBodyBytes = 1024

// create handles POST /api/v1/phrases.
// It decodes the request body, validates required fields, and inserts the phrase.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req db.CreatePhraseRequest
	if err := dec.Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// phrase and at least one headword are required; note is optional
	if req.Phrase == "" || len(req.Headwords) == 0 {
		respondErr(w, http.StatusBadRequest, "phrase and at least one headword are required")
		return
	}

	phrase, err := h.store.CreatePhrase(r.Context(), userID, req)
	if err != nil {
		slog.Error("create phrase", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to create phrase")
		return
	}

	slog.Debug("create phrase", "id", phrase.ID, "headwords", phrase.Headwords)
	respond(w, http.StatusCreated, phrase)
}

// update handles PATCH /api/v1/phrases/{id}.
// Only non-null fields in the request body are updated; omitted fields and
// explicit JSON null values both leave the current value unchanged.
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req db.UpdatePhraseRequest
	if err := dec.Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Phrase == nil && req.Headwords == nil && req.Note == nil && req.SourceURLs == nil {
		respondErr(w, http.StatusBadRequest, "at least one field must be provided")
		return
	}
	if req.Phrase != nil && *req.Phrase == "" {
		respondErr(w, http.StatusBadRequest, "phrase cannot be empty")
		return
	}
	if req.Headwords != nil && len(req.Headwords) == 0 {
		respondErr(w, http.StatusBadRequest, "headwords cannot be empty")
		return
	}

	phrase, err := h.store.UpdatePhrase(r.Context(), userID, id, req)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusNotFound, "phrase not found")
			return
		}
		slog.Error("update phrase", "id", id, "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to update phrase")
		return
	}

	slog.Debug("update phrase", "id", id)
	respond(w, http.StatusOK, phrase)
}

// delete handles DELETE /api/v1/phrases/{id}.
// Returns 204 on success, 404 if the phrase does not exist or belongs to another user.
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := h.store.DeletePhrase(r.Context(), userID, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusNotFound, "phrase not found")
			return
		}
		slog.Error("delete phrase", "id", id, "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to delete phrase")
		return
	}

	slog.Debug("delete phrase", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// parseID extracts and validates the {id} path variable as a UUID.
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
