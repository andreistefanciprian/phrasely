package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const (
	magicLinkTTL = 15 * time.Minute
	jwtTTL       = 30 * 24 * time.Hour // 30 days
)

type Handler struct {
	store     db.Store
	baseURL   string // used to build the magic link; e.g. "http://localhost:8080"
	jwtSecret []byte // signs and verifies JWTs
}

func NewHandler(store db.Store, baseURL, jwtSecret string) *Handler {
	return &Handler{store: store, baseURL: baseURL, jwtSecret: []byte(jwtSecret)}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/auth/request", h.request).Methods(http.MethodPost)
	r.HandleFunc("/auth/verify", h.verify).Methods(http.MethodGet)
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
	// /auth-verify is the frontend page; it calls /auth/verify (API) internally.
	// Keeps the browser landing page separate from the API endpoint.
	link := h.baseURL + "/auth-verify?token=" + token.Token
	slog.Info("magic link", "email", body.Email, "link", link)

	respond(w, http.StatusOK, map[string]string{"message": "magic link sent"})
}

// verify handles GET /auth/verify?token=<uuid>.
// It validates the token (exists, not expired, not used), marks it used,
// and returns a signed JWT the client uses for all subsequent requests.
func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	rawToken := r.URL.Query().Get("token")
	if rawToken == "" {
		respond(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}
	// Validate UUID format before hitting the DB — a non-UUID value would
	// cause a Postgres cast error that would surface as a 500.
	if _, err := uuid.Parse(rawToken); err != nil {
		respond(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}

	record, err := h.store.GetMagicLinkToken(r.Context(), rawToken)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respond(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		slog.Error("get magic link token", "error", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify token"})
		return
	}

	// Token must be unused and not expired
	if record.UsedAt != nil {
		respond(w, http.StatusUnauthorized, map[string]string{"error": "token already used"})
		return
	}
	// !Before means >= — matches the DB's expires_at > NOW() so both agree on the boundary.
	if !time.Now().Before(record.ExpiresAt) {
		respond(w, http.StatusUnauthorized, map[string]string{"error": "token expired"})
		return
	}

	// Atomically consume the token — guards against concurrent requests
	// claiming the same token. ErrNotFound means it was already used or
	// expired between our SELECT and this UPDATE.
	if err := h.store.MarkTokenUsed(r.Context(), record.ID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respond(w, http.StatusUnauthorized, map[string]string{"error": "token already used or expired"})
			return
		}
		slog.Error("mark token used", "error", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify token"})
		return
	}

	// Issue a signed JWT containing the user ID
	signed, err := SignJWT(record.UserID, string(h.jwtSecret), jwtTTL)
	if err != nil {
		slog.Error("sign jwt", "error", err)
		respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to issue token"})
		return
	}

	respond(w, http.StatusOK, map[string]string{"token": signed})
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
