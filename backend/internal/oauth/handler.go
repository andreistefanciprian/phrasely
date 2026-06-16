// Package oauth implements the internal OAuth 2.1 endpoints that back the public
// endpoints hosted by the mcp server. None of these routes are exposed directly
// to the internet — they are only reachable by mcp and frontend over the private
// internal network.
package oauth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
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
	r.HandleFunc("/internal/oauth/register", h.register).Methods(http.MethodPost)
}

// registerRequest is the body of POST /internal/oauth/register.
// It mirrors the RFC 7591 Dynamic Client Registration request, simplified to
// the fields we actually use.
type registerRequest struct {
	// RedirectURIs is the list of URIs the client is allowed to redirect to after
	// the user approves consent. We pin these at registration time and validate
	// them on every /authorize call — this is the key open-redirect defence.
	// An attacker who tries to swap in their own redirect_uri at /authorize will
	// be rejected because it won't match what was registered here.
	RedirectURIs []string `json:"redirect_uris"`
}

// registerResponse is the body returned on success.
// Per RFC 7591 §3.2.1 the server echoes back the registered metadata plus
// the assigned client_id.
type registerResponse struct {
	// ClientID is the opaque identifier assigned to this client. The client
	// includes it in every subsequent /authorize and /token request so the
	// server knows which registration to validate against.
	ClientID     string   `json:"client_id"`
	RedirectURIs []string `json:"redirect_uris"`
}

// register handles POST /internal/oauth/register.
// Called by mcp (which proxies the public /register endpoint).
// It validates the redirect URIs, stores the client, and returns a client_id.
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024) // 4 KB — generous for a list of URIs

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// At least one redirect_uri is required. RFC 7591 doesn't mandate this
	// for all client types, but ChatGPT's connector flow always provides one,
	// and we need it to validate the /authorize redirect later.
	if len(req.RedirectURIs) == 0 {
		respondErr(w, http.StatusBadRequest, "at least one redirect_uri is required")
		return
	}

	// Validate every URI before persisting anything.
	// We accept http:// to allow local dev (e.g. http://localhost:...).
	// In production, ChatGPT always uses https://.
	for _, raw := range req.RedirectURIs {
		if err := validateRedirectURI(raw); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid redirect_uri: "+err.Error())
			return
		}
	}

	client, err := h.store.CreateOAuthClient(r.Context(), req.RedirectURIs)
	if err != nil {
		slog.Error("create oauth client", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to register client")
		return
	}

	respond(w, http.StatusCreated, registerResponse{
		ClientID:     client.ID,
		RedirectURIs: client.RedirectURIs,
	})
}

// validateRedirectURI checks that a redirect_uri is a well-formed http/https URL.
// We deliberately do not allow:
//   - fragment components (#...) — RFC 6749 §3.1.2 forbids them. Note: we check
//     the raw string for '#' because url.ParseRequestURI silently strips fragments,
//     so u.Fragment would always appear empty even when a fragment is present.
//   - non-http(s) schemes — limits attack surface (no javascript:, data:, etc.)
func validateRedirectURI(raw string) error {
	if strings.Contains(raw, "#") {
		return errors.New("fragments not allowed in redirect_uri (RFC 6749 §3.1.2)")
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("scheme must be http or https")
	}
	return nil
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func respondErr(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}
