package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andreistefanciprian/phrasely/internal/auth"
)

const testSecret = "test-secret"

// nextHandler is a simple handler that records whether it was called
// and writes the user ID from context into the response body.
func nextHandler(t *testing.T, called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		userID := UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(userID))
	})
}

func makeRequest(path, authHeader string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		r.Header.Set("Authorization", authHeader)
	}
	return r
}

func validJWT(t *testing.T) string {
	t.Helper()
	token, err := auth.SignJWT("user-123", testSecret, 30*time.Minute)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return token
}

func TestAuthenticate_UnprotectedRoutes(t *testing.T) {
	paths := []string{"/health", "/auth/request", "/auth/verify"}
	mw := Authenticate(testSecret)

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			called := false
			w := httptest.NewRecorder()
			mw(nextHandler(t, &called)).ServeHTTP(w, makeRequest(path, ""))

			if !called {
				t.Error("expected next handler to be called for unprotected route")
			}
			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", w.Code)
			}
		})
	}
}

func TestAuthenticate_MissingToken(t *testing.T) {
	called := false
	w := httptest.NewRecorder()
	Authenticate(testSecret)(nextHandler(t, &called)).ServeHTTP(w, makeRequest("/api/v1/phrases", ""))

	if called {
		t.Error("next handler should not be called without a token")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header on 401")
	}
}

func TestAuthenticate_MalformedHeader(t *testing.T) {
	called := false
	w := httptest.NewRecorder()
	// Missing "Bearer " prefix
	Authenticate(testSecret)(nextHandler(t, &called)).ServeHTTP(w, makeRequest("/api/v1/phrases", "Token abc123"))

	if called {
		t.Error("next handler should not be called with malformed header")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	called := false
	w := httptest.NewRecorder()
	Authenticate(testSecret)(nextHandler(t, &called)).ServeHTTP(w, makeRequest("/api/v1/phrases", "Bearer not-a-valid-jwt"))

	if called {
		t.Error("next handler should not be called with invalid token")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthenticate_ValidToken(t *testing.T) {
	called := false
	w := httptest.NewRecorder()
	token := validJWT(t)

	Authenticate(testSecret)(nextHandler(t, &called)).ServeHTTP(w, makeRequest("/api/v1/phrases", "Bearer "+token))

	if !called {
		t.Error("expected next handler to be called with valid token")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "user-123" {
		t.Errorf("expected user ID in context, got %q", w.Body.String())
	}
}

func TestAuthenticate_WrongSecret(t *testing.T) {
	called := false
	w := httptest.NewRecorder()
	token := validJWT(t) // signed with testSecret

	// Middleware uses a different secret — signature won't verify
	Authenticate("wrong-secret")(nextHandler(t, &called)).ServeHTTP(w, makeRequest("/api/v1/phrases", "Bearer "+token))

	if called {
		t.Error("next handler should not be called with wrong secret")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
