package phrases

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/andreistefanciprian/phrasely/internal/db"
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
	r.HandleFunc("/api/v1/phrases", h.create).Methods(http.MethodPost)
}

// create handles POST /api/v1/phrases.
// It decodes the request body, validates required fields, and inserts the phrase.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req db.CreatePhraseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func respondErr(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}
