// Package oauth implements the internal OAuth 2.1 endpoints that back the public
// endpoints hosted by the mcp server. None of these routes are exposed directly
// to the internet — they are only reachable by mcp and frontend over the private
// internal network.
package oauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/auth"
	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/andreistefanciprian/phrasely/internal/middleware"
	"github.com/gorilla/mux"
)

type Handler struct {
	store     db.Store
	jwtSecret string
}

func NewHandler(store db.Store, jwtSecret string) *Handler {
	return &Handler{store: store, jwtSecret: jwtSecret}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/internal/oauth/register", h.register).Methods(http.MethodPost)
	r.HandleFunc("/internal/oauth/authorize", h.authorize).Methods(http.MethodPost)
	r.HandleFunc("/internal/oauth/clients/{id}", h.getClient).Methods(http.MethodGet)
	r.HandleFunc("/internal/oauth/token", h.token).Methods(http.MethodPost)
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

// clientResponse is the body returned by GET /internal/oauth/clients/{id}.
type clientResponse struct {
	ClientID     string   `json:"client_id"`
	RedirectURIs []string `json:"redirect_uris"`
}

// getClient handles GET /internal/oauth/clients/{id}.
// Called by the frontend on GET /authorize to validate client_id + redirect_uri
// before rendering the consent form. Per RFC 6749 §4.1.2.1 the frontend must
// NOT redirect on an invalid redirect_uri — it shows an error page instead.
func (h *Handler) getClient(w http.ResponseWriter, r *http.Request) {
	clientID := mux.Vars(r)["id"]
	client, err := h.store.GetOAuthClient(r.Context(), clientID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusNotFound, "client not found")
			return
		}
		slog.Error("get oauth client", "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	respond(w, http.StatusOK, clientResponse{
		ClientID:     client.ID,
		RedirectURIs: client.RedirectURIs,
	})
}

// authorizeRequest is the body of POST /internal/oauth/authorize.
// Called by the frontend after the user approves consent on the /authorize screen.
type authorizeRequest struct {
	// ClientID identifies the registered OAuth client (from /register).
	ClientID string `json:"client_id"`

	// RedirectURI is the URI the code will be appended to. Must exactly match
	// one of the URIs registered at /register — this prevents open redirect attacks.
	RedirectURI string `json:"redirect_uri"`

	// CodeChallenge is BASE64URL(SHA256(code_verifier)). Generated by the client
	// (ChatGPT) and stored here so we can verify the code_verifier at /token time.
	// The raw code_verifier never leaves the client — storing only the hash means
	// a DB leak can't be used to forge tokens.
	CodeChallenge string `json:"code_challenge"`
}

// authorizeResponse is returned on success.
type authorizeResponse struct {
	// Code is the short-lived (~60s) single-use authorization code.
	// The frontend appends it to the redirect_uri and sends the browser back to ChatGPT.
	Code string `json:"code"`
}

// authCodeTTL is how long an authorization code is valid for exchange at /token.
// RFC 6749 §4.1.2 recommends a maximum of 10 minutes; 60 seconds is tighter and
// common in production — the client exchanges the code immediately after redirect.
const authCodeTTL = 60 * time.Second

// base64urlRE matches the base64url alphabet (no padding). A SHA256 hash encoded
// this way is always exactly 43 characters.
var base64urlRE = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// authorize handles POST /internal/oauth/authorize.
// Called by the frontend (over the private network) after the user approves consent.
// It validates the client + redirect URI, stores a PKCE authorization code, and
// returns it so the frontend can redirect the browser back to the OAuth client.
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)

	// user_id is injected by the JWT middleware — the user must be logged in to
	// the frontend before they can reach the consent screen.
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req authorizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ClientID == "" || req.RedirectURI == "" || req.CodeChallenge == "" {
		respondErr(w, http.StatusBadRequest, "client_id, redirect_uri, and code_challenge are required")
		return
	}

	// code_challenge must be BASE64URL(SHA256(verifier)) — always 43 chars, no padding.
	// Rejecting obviously wrong values early avoids storing garbage in the DB.
	if !base64urlRE.MatchString(req.CodeChallenge) {
		respondErr(w, http.StatusBadRequest, "code_challenge must be base64url-encoded SHA256 (43 chars, no padding)")
		return
	}

	// Look up the registered client to validate client_id and retrieve redirect_uris.
	client, err := h.store.GetOAuthClient(r.Context(), req.ClientID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusBadRequest, "unknown client_id")
			return
		}
		slog.Error("get oauth client", "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// redirect_uri must exactly match one of the registered URIs.
	// This is the open-redirect defence: an attacker cannot swap in their own URI
	// at /authorize because it won't match what was pinned at /register.
	if !redirectURIAllowed(req.RedirectURI, client.RedirectURIs) {
		respondErr(w, http.StatusBadRequest, "redirect_uri not registered for this client")
		return
	}

	code, err := h.store.CreateAuthorizationCode(r.Context(), db.CreateAuthCodeRequest{
		ClientID:      req.ClientID,
		UserID:        userID,
		RedirectURI:   req.RedirectURI,
		CodeChallenge: req.CodeChallenge,
		ExpiresAt:     time.Now().Add(authCodeTTL),
	})
	if err != nil {
		slog.Error("create authorization code", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to issue authorization code")
		return
	}

	respond(w, http.StatusOK, authorizeResponse{Code: code.Code})
}

// redirectURIAllowed reports whether uri exactly matches one of the allowed URIs.
// Exact match only — no prefix or suffix matching — to prevent redirect hijacking.
func redirectURIAllowed(uri string, allowed []string) bool {
	for _, a := range allowed {
		if uri == a {
			return true
		}
	}
	return false
}

// accessTokenTTL is how long an OAuth access token is valid.
// Shorter than the magic-link JWT (30 days) because the client can refresh silently.
const accessTokenTTL = time.Hour

// token handles POST /internal/oauth/token — the code→token exchange.
// Called by mcp (which proxies the public /token endpoint on itself).
// This is where PKCE is verified: the client proves it holds the code_verifier
// that was used to generate the code_challenge stored when the code was issued.
//
// Request body is application/x-www-form-urlencoded (OAuth 2.1 §3.2):
//
//	grant_type=authorization_code
//	&code=<auth-code>
//	&code_verifier=<pkce-verifier>
//	&client_id=<uuid>
//	&redirect_uri=<must-match-registered-value>
func (h *Handler) token(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	if err := r.ParseForm(); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if r.PostForm.Get("grant_type") != "authorization_code" {
		respondErr(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}

	code := r.PostForm.Get("code")
	codeVerifier := r.PostForm.Get("code_verifier")
	clientID := r.PostForm.Get("client_id")
	redirectURI := r.PostForm.Get("redirect_uri")

	if code == "" || codeVerifier == "" || clientID == "" || redirectURI == "" {
		respondErr(w, http.StatusBadRequest, "code, code_verifier, client_id, and redirect_uri are required")
		return
	}

	// Atomically consume the code: marks it used and returns its record.
	// Returns ErrNotFound for any failure (expired, already used, wrong client)
	// — deliberately indistinct to avoid leaking state to attackers.
	authCode, err := h.store.ConsumeAuthorizationCode(r.Context(), code, clientID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondErr(w, http.StatusBadRequest, "invalid_grant")
			return
		}
		slog.Error("consume authorization code", "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// PKCE S256 verification (RFC 7636 §4.6):
	// Compute BASE64URL(SHA256(code_verifier)) and compare to the stored challenge.
	//
	// Why this matters: the code travels in a browser redirect URL where it could
	// be logged or intercepted. The code_verifier never leaves the client, so even
	// with the code an attacker cannot complete the exchange without it.
	if !verifyPKCES256(codeVerifier, authCode.CodeChallenge) {
		respondErr(w, http.StatusBadRequest, "invalid_grant")
		return
	}

	// redirect_uri must exactly match the value used when the code was issued
	// (RFC 6749 §4.1.3). Prevents a client from using a code with a different URI.
	if redirectURI != authCode.RedirectURI {
		respondErr(w, http.StatusBadRequest, "invalid_grant")
		return
	}

	// Issue a short-lived JWT access token scoped to the authenticated user.
	// Same format as magic-link JWTs so the backend phrase endpoints accept it unchanged.
	accessToken, err := auth.SignJWT(authCode.UserID, h.jwtSecret, accessTokenTTL)
	if err != nil {
		slog.Error("sign access token", "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Issue a refresh token stored in the DB. The client uses it to get new access
	// tokens when this one expires, without re-prompting the user. Rotation in PR 29.
	refreshToken, err := h.store.CreateRefreshToken(r.Context(), db.CreateRefreshTokenRequest{
		ClientID: authCode.ClientID,
		UserID:   authCode.UserID,
	})
	if err != nil {
		slog.Error("create refresh token", "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// RFC 6749 §5.1: token responses must not be cached.
	// Pragma: no-cache is included for HTTP/1.0 proxy compatibility.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	respond(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(accessTokenTTL.Seconds()),
		"refresh_token": refreshToken.Token,
	})
}

// verifyPKCES256 returns true if BASE64URL(SHA256(verifier)) == challenge.
// Uses subtle.ConstantTimeCompare to avoid leaking information about the
// challenge via response-time differences (timing attack defence).
func verifyPKCES256(codeVerifier, codeChallenge string) bool {
	h := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(codeChallenge)) == 1
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
