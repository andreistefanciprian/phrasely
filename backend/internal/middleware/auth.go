package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/andreistefanciprian/phrasely/internal/auth"
)

// contextKey is an unexported type for context keys in this package,
// preventing collisions with keys from other packages.
type contextKey string

const UserIDKey contextKey = "user_id"

// Authenticate validates the JWT on protected routes and injects the user ID
// into the request context. Unprotected routes pass through unchanged.
func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	// Precompute once per middleware init — avoids allocating []byte on every request.
	secretBytes := []byte(jwtSecret)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isUnprotected(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				respondUnauthorized(w)
				return
			}
			// TrimSpace guards against a trailing space in the header value
			// which would cause an otherwise-valid token to be rejected.
			tokenStr := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))

			userID, err := auth.ParseJWT(tokenStr, secretBytes)
			if err != nil {
				respondUnauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext retrieves the authenticated user ID from the request context.
// Returns empty string if not present (i.e. on unprotected routes).
func UserIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(UserIDKey).(string)
	return id
}

// isUnprotected returns true for routes that do not require authentication.
func isUnprotected(path string) bool {
	return path == "/health" ||
		strings.HasPrefix(path, "/auth/") ||
		// /internal/ routes are called service-to-service (mcp → backend, frontend → backend)
		// over the private network. The API is never directly exposed to the internet, so
		// network isolation is the access control — no user JWT is possible or needed here.
		strings.HasPrefix(path, "/internal/")
}

func respondUnauthorized(w http.ResponseWriter) {
	// RFC 6750 §3: 401 responses for Bearer-protected resources must include
	// WWW-Authenticate so clients know which auth scheme is required.
	w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
