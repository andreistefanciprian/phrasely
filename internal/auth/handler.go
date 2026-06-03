package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/gorilla/mux"
)

const magicLinkTTL = 15 * time.Minute

type Handler struct {
	store   db.Store
	baseURL string // used to build the magic link; e.g. "http://localhost:8080"
}

func NewHandler(store db.Store, baseURL string) *Handler {
	return &Handler{store: store, baseURL: baseURL}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/auth/request", h.request).Methods(http.MethodPost)
}

type requestBody struct {
	Email string `json:"email"`
}

// request handles POST /auth/request.
// It upserts the user by email, generates a magic link token,
// and logs the link to stdout (in production this would send an email).
func (h *Handler) request(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var body requestBody
	if err := dec.Decode(&body); err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	body.Email = strings.TrimSpace(body.Email)
	if body.Email == "" {
		respond(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}

	user, err := h.store.UpsertUser(r.Context(), body.Email)
	if err != nil {
		slog.Error("upsert user", "error", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to process request"})
		return
	}

	token, err := h.store.CreateMagicLinkToken(r.Context(), user.ID, time.Now().Add(magicLinkTTL))
	if err != nil {
		slog.Error("create magic link token", "error", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to process request"})
		return
	}

	// In production: send this link by email.
	// Locally: log it to stdout so you can click it directly from the terminal.
	link := h.baseURL + "/auth/verify?token=" + token.Token
	slog.Info("magic link", "email", body.Email, "link", link)

	respond(w, http.StatusOK, map[string]string{"message": "magic link sent"})
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
