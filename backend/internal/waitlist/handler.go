// Package waitlist handles the "join the ChatGPT list" capture on the
// landing page. POST /waitlist is unauthenticated (called before the visitor
// has an account) and deliberately ships without rate limiting for now — the
// ChatGPT integration this waitlist gates is expected to launch soon, which
// shortens how long the endpoint needs to stay exposed.
package waitlist

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/gorilla/mux"
)

type Handler struct {
	store db.Store
}

func NewHandler(store db.Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/waitlist", h.join).Methods(http.MethodPost)
}

// emailRe mirrors the client-side check in the landing page form — good
// enough to catch typos, not a full RFC 5322 validator.
var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]{2,}$`)

// validSources are the landing page's three capture placements. Anything
// else — a bug, a stale client, or someone poking the endpoint directly —
// falls back to "unknown" rather than polluting attribution reporting with
// arbitrary strings.
var validSources = map[string]bool{"hero": true, "integration": true, "closing": true}

type joinRequest struct {
	Email  string `json:"email"`
	Source string `json:"source"`
}

// join handles POST /waitlist. It's idempotent per email: a repeat
// submission returns 200 rather than an error.
func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req joinRequest
	if err := dec.Decode(&req); err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !emailRe.MatchString(req.Email) {
		respond(w, http.StatusBadRequest, map[string]string{"error": "a valid email is required"})
		return
	}

	req.Source = strings.TrimSpace(req.Source)
	if !validSources[req.Source] {
		req.Source = "unknown"
	}

	if err := h.store.AddWaitlistSignup(r.Context(), req.Email, req.Source); err != nil {
		slog.Error("add waitlist signup", "error", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to join waitlist"})
		return
	}

	respond(w, http.StatusOK, map[string]string{"status": "ok"})
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
