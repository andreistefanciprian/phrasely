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
	store          db.Store
	jwtSecret      string
	accessTokenTTL time.Duration
}

func NewHandler(store db.Store, jwtSecret string, accessTokenTTL time.Duration) *Handler {
	return &Handler{store: store, jwtSecret: jwtSecret, accessTokenTTL: accessTokenTTL}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/internal/oauth/register", h.register).Methods(http.MethodPost)
	r.HandleFunc("/internal/oauth/authorize", h.authorize).Methods(http.MethodPost)
	r.HandleFunc("/internal/oauth/clients/{id}", h.getClient).Methods(http.MethodGet)
	r.HandleFunc("/internal/oauth/token", h.token).Methods(http.MethodPost)
	r.HandleFunc("/internal/oauth/tokens", h.revokeTokens).Methods(http.MethodDelete)
	r.HandleFunc("/internal/oauth/revoke", h.revoke).Methods(http.MethodPost)
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
	ClientID                string   `json:"client_id"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// register handles POST /internal/oauth/register.
// Called by mcp (which proxies the public /register endpoint).
// It validates the redirect URIs, stores the client, and returns a client_id.
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024) // 4 KB — generous for a list of URIs

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("oauth: client registration rejected", "reason", "invalid_request_body")
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// At least one redirect_uri is required. RFC 7591 doesn't mandate this
	// for all client types, but ChatGPT's connector flow always provides one,
	// and we need it to validate the /authorize redirect later.
	if len(req.RedirectURIs) == 0 {
		slog.Warn("oauth: client registration rejected", "reason", "missing_redirect_uri")
		respondErr(w, http.StatusBadRequest, "at least one redirect_uri is required")
		return
	}

	// Validate every URI before persisting anything.
	// We accept http:// to allow local dev (e.g. http://localhost:...).
	// In production, ChatGPT always uses https://.
	for _, raw := range req.RedirectURIs {
		if err := validateRedirectURI(raw); err != nil {
			slog.Warn("oauth: client registration rejected", "reason", "invalid_redirect_uri")
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

	slog.Info("oauth: client registered", "client_id", client.ID)
	respond(w, http.StatusCreated, registerResponse{
		ClientID:                client.ID,
		RedirectURIs:            client.RedirectURIs,
		TokenEndpointAuthMethod: "none",
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
			slog.Warn("oauth: client lookup rejected", "client_id", clientID, "reason", "unknown_client")
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

	// Resource is the canonical URI of the MCP server the access token will be
	// used with. It is carried through the code and refresh-token grants and
	// becomes the access token audience (RFC 8707).
	Resource string `json:"resource"`

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
		slog.Warn("oauth: authorization rejected", "reason", "authentication_required")
		respondErr(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req authorizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("oauth: authorization rejected", "reason", "invalid_request_body")
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ClientID == "" || req.Resource == "" || req.RedirectURI == "" || req.CodeChallenge == "" {
		slog.Warn("oauth: authorization rejected", "client_id", req.ClientID, "reason", "missing_required_field")
		respondErr(w, http.StatusBadRequest, "client_id, resource, redirect_uri, and code_challenge are required")
		return
	}

	if err := validateResourceURI(req.Resource); err != nil {
		slog.Warn("oauth: authorization rejected", "client_id", req.ClientID, "reason", "invalid_resource")
		respondErr(w, http.StatusBadRequest, "invalid resource: "+err.Error())
		return
	}

	// code_challenge must be BASE64URL(SHA256(verifier)) — always 43 chars, no padding.
	// Rejecting obviously wrong values early avoids storing garbage in the DB.
	if !base64urlRE.MatchString(req.CodeChallenge) {
		slog.Warn("oauth: authorization rejected", "client_id", req.ClientID, "reason", "invalid_code_challenge")
		respondErr(w, http.StatusBadRequest, "code_challenge must be base64url-encoded SHA256 (43 chars, no padding)")
		return
	}

	// Look up the registered client to validate client_id and retrieve redirect_uris.
	client, err := h.store.GetOAuthClient(r.Context(), req.ClientID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			slog.Warn("oauth: authorization rejected", "client_id", req.ClientID, "reason", "unknown_client")
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
		slog.Warn("oauth: authorization rejected", "client_id", req.ClientID, "reason", "redirect_uri_mismatch")
		respondErr(w, http.StatusBadRequest, "redirect_uri not registered for this client")
		return
	}

	code, err := h.store.CreateAuthorizationCode(r.Context(), db.CreateAuthCodeRequest{
		ClientID:      req.ClientID,
		UserID:        userID,
		Resource:      req.Resource,
		RedirectURI:   req.RedirectURI,
		CodeChallenge: req.CodeChallenge,
		ExpiresAt:     time.Now().Add(authCodeTTL),
	})
	if err != nil {
		slog.Error("create authorization code", "error", err)
		respondErr(w, http.StatusInternalServerError, "failed to issue authorization code")
		return
	}

	slog.Info("oauth: authorization code issued", "client_id", req.ClientID, "user_id", userID)
	respond(w, http.StatusOK, authorizeResponse{Code: code.Code})
}

// revokeTokens handles DELETE /internal/oauth/tokens?client_id=...
// Called by the frontend when the user explicitly denies an OAuth consent request.
// It revokes all active refresh tokens for the authenticated user + given client so
// the client cannot refresh access after a prior authorization. Any already-issued
// access token (JWT) remains valid until its 1-hour TTL expires.
func (h *Handler) revokeTokens(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		respondErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		respondErr(w, http.StatusBadRequest, "client_id is required")
		return
	}
	if err := h.store.RevokeRefreshTokens(r.Context(), userID, clientID); err != nil {
		slog.Error("revoke refresh tokens", "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	slog.Info("oauth: refresh tokens revoked on deny", "client_id", clientID, "user_id", userID)
	w.WriteHeader(http.StatusNoContent)
}

// revoke handles POST /internal/oauth/revoke.
//
// This is the server-side half of RFC 7009 token revocation. ChatGPT calls the
// public /revoke endpoint on the MCP server, which proxies here over the private
// network. It fires when the user clicks "Disconnect" in the ChatGPT connector UI.
//
// Key RFC 7009 §2.2 rule: the server MUST return 200 OK even when the token is
// unknown, already revoked, or belongs to a different client. The caller has no
// actionable response to a revocation failure — the token is either gone or useless.
//
// Access tokens are stateless JWTs; we cannot revoke them server-side. We skip
// them silently and let them expire naturally (1-hour TTL). Only refresh tokens
// are stored in the DB and can be revoked by setting revoked_at.
func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	if err := r.ParseForm(); err != nil {
		// Malformed body — still 200 per RFC 7009; nothing to revoke.
		slog.Warn("oauth: revoke: failed to parse form body", "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	token := r.PostForm.Get("token")
	clientID := r.PostForm.Get("client_id")
	hint := r.PostForm.Get("token_type_hint")

	slog.Debug("oauth: revoke request received", "client_id", clientID, "token_type_hint", hint)

	// Access tokens are self-contained JWTs with a 1-hour TTL — skip silently.
	if hint == "access_token" {
		slog.Debug("oauth: revoke: skipping access token (stateless JWT, cannot revoke server-side)", "client_id", clientID)
		w.WriteHeader(http.StatusOK)
		return
	}

	if token == "" || clientID == "" {
		// Nothing actionable — RFC 7009 still mandates 200.
		slog.Warn("oauth: revoke: missing token or client_id, nothing to revoke", "client_id", clientID)
		w.WriteHeader(http.StatusOK)
		return
	}

	// ConsumeRefreshToken atomically sets revoked_at on the matching row.
	// We reuse the rotation helper here — the difference is we don't issue a
	// new token. ErrNotFound covers: already revoked, unknown token, wrong client.
	if _, err := h.store.ConsumeRefreshToken(r.Context(), token, clientID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			// Token unknown or already revoked — harmless; client just wants it gone.
			slog.Debug("oauth: revoke: token not found or already revoked", "client_id", clientID)
		} else {
			// Unexpected DB error — log it, but still return 200 per RFC 7009.
			slog.Error("oauth: revoke: store error", "client_id", clientID, "error", err)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	slog.Info("oauth: refresh token revoked via disconnect", "client_id", clientID)
	w.WriteHeader(http.StatusOK)
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

// token handles POST /internal/oauth/token.
// Routes to the correct grant handler based on grant_type.
// Request body is application/x-www-form-urlencoded (OAuth 2.1 §3.2).
func (h *Handler) token(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	if err := r.ParseForm(); err != nil {
		slog.Warn("oauth: token request rejected", "reason", "invalid_request_body")
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	grantType := r.PostForm.Get("grant_type")
	switch grantType {
	case "authorization_code":
		h.authCodeGrant(w, r)
	case "refresh_token":
		h.refreshTokenGrant(w, r)
	default:
		slog.Warn("oauth: token request rejected", "grant_type", grantType, "reason", "unsupported_grant_type")
		respondErr(w, http.StatusBadRequest, "unsupported_grant_type")
	}
}

// authCodeGrant handles grant_type=authorization_code.
// This is where PKCE is verified: the client proves it holds the code_verifier
// that was used to generate the code_challenge stored when the code was issued.
func (h *Handler) authCodeGrant(w http.ResponseWriter, r *http.Request) {
	code := r.PostForm.Get("code")
	codeVerifier := r.PostForm.Get("code_verifier")
	clientID := r.PostForm.Get("client_id")
	redirectURI := r.PostForm.Get("redirect_uri")
	resource := r.PostForm.Get("resource")

	if code == "" || codeVerifier == "" || clientID == "" || redirectURI == "" || resource == "" {
		slog.Warn("oauth: authorization code exchange rejected", "client_id", clientID, "reason", "missing_required_field")
		respondErr(w, http.StatusBadRequest, "code, code_verifier, client_id, redirect_uri, and resource are required")
		return
	}

	// Atomically consume the code: marks it used and returns its record.
	// Returns ErrNotFound for any failure (expired, already used, wrong client)
	// — deliberately indistinct to avoid leaking state to attackers.
	authCode, err := h.store.ConsumeAuthorizationCode(r.Context(), code, clientID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			slog.Warn("oauth: authorization code exchange rejected", "client_id", clientID, "reason", "invalid_expired_or_used_code")
			respondErr(w, http.StatusBadRequest, "invalid_grant")
			return
		}
		slog.Error("consume authorization code", "client_id", clientID, "error", err)
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
		slog.Warn("oauth: authorization code exchange rejected", "client_id", clientID, "reason", "pkce_verification_failed")
		respondErr(w, http.StatusBadRequest, "invalid_grant")
		return
	}

	// redirect_uri must exactly match the value used when the code was issued
	// (RFC 6749 §4.1.3). Prevents a client from using a code with a different URI.
	if redirectURI != authCode.RedirectURI {
		slog.Warn("oauth: authorization code exchange rejected", "client_id", clientID, "reason", "redirect_uri_mismatch")
		respondErr(w, http.StatusBadRequest, "invalid_grant")
		return
	}

	// The token request must repeat the resource from the authorization request.
	// This prevents a code granted for one audience from being exchanged for a
	// token usable at another resource server (RFC 8707 §4.1).
	if resource != authCode.Resource {
		slog.Warn("oauth: authorization code exchange rejected", "client_id", clientID, "reason", "resource_mismatch")
		respondErr(w, http.StatusBadRequest, "invalid_target")
		return
	}

	refreshToken, err := h.store.CreateRefreshToken(r.Context(), db.CreateRefreshTokenRequest{
		ClientID: authCode.ClientID,
		UserID:   authCode.UserID,
		Resource: authCode.Resource,
	})
	if err != nil {
		slog.Error("create refresh token", "client_id", clientID, "grant_type", "authorization_code", "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.writeTokenResponse(w, authCode.UserID, refreshToken.Token, authCode.Resource, clientID, "authorization_code")
}

// refreshTokenGrant handles grant_type=refresh_token.
//
// Rotation: the old refresh token is atomically revoked and a new one is issued.
// If the old token arrives again (replay attack), ConsumeRefreshToken returns
// ErrNotFound — the client must re-authorise. In a hardened implementation, a
// replay should also revoke ALL tokens for the client (RFC 6819 §5.2.2.3).
func (h *Handler) refreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.PostForm.Get("refresh_token")
	clientID := r.PostForm.Get("client_id")
	resource := r.PostForm.Get("resource")
	if tokenStr == "" || clientID == "" || resource == "" {
		slog.Warn("oauth: refresh token exchange rejected", "client_id", clientID, "reason", "missing_required_field")
		respondErr(w, http.StatusBadRequest, "refresh_token, client_id, and resource are required")
		return
	}

	// Atomically revoke the old token and return its record.
	// clientID is enforced in the WHERE clause — a client cannot consume another's token.
	// Returns ErrNotFound if the token doesn't exist, is revoked, or clientID mismatches.
	old, err := h.store.ConsumeRefreshToken(r.Context(), tokenStr, clientID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			slog.Warn("oauth: refresh token exchange rejected", "client_id", clientID, "reason", "invalid_revoked_or_wrong_client_token")
			respondErr(w, http.StatusBadRequest, "invalid_grant")
			return
		}
		slog.Error("consume refresh token", "client_id", clientID, "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	if resource != old.Resource {
		slog.Warn("oauth: refresh token exchange rejected", "client_id", clientID, "reason", "resource_mismatch")
		respondErr(w, http.StatusBadRequest, "invalid_target")
		return
	}

	newRefreshToken, err := h.store.CreateRefreshToken(r.Context(), db.CreateRefreshTokenRequest{
		ClientID: old.ClientID,
		UserID:   old.UserID,
		Resource: old.Resource,
	})
	if err != nil {
		slog.Error("create refresh token", "client_id", clientID, "grant_type", "refresh_token", "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.writeTokenResponse(w, old.UserID, newRefreshToken.Token, old.Resource, clientID, "refresh_token")
}

// writeTokenResponse signs a JWT access token and writes the standard token
// response body with no-store cache headers (RFC 6749 §5.1).
func (h *Handler) writeTokenResponse(w http.ResponseWriter, userID, refreshToken, resource, clientID, grantType string) {
	accessToken, err := auth.SignOAuthJWT(userID, h.jwtSecret, resource, h.accessTokenTTL)
	if err != nil {
		slog.Error("sign access token", "client_id", clientID, "grant_type", grantType, "error", err)
		respondErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	slog.Info("oauth: token exchange succeeded", "client_id", clientID, "user_id", userID, "grant_type", grantType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	respond(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(h.accessTokenTTL.Seconds()),
		"refresh_token": refreshToken,
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

// validateResourceURI checks the RFC 8707 resource indicator before persisting
// it. Resource identifiers must be absolute HTTP(S) URIs and cannot include a
// fragment because fragments are not sent to resource servers.
func validateResourceURI(raw string) error {
	if strings.Contains(raw, "#") {
		return errors.New("fragments not allowed")
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("must be an absolute http or https URL")
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
