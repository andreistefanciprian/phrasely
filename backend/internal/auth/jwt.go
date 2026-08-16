package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SignJWT creates a signed JWT for the given user ID.
// The token carries sub (user ID), iat (issued at), and exp (expiry).
func SignJWT(userID string, secret string, ttl time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(ttl).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

// SignOAuthJWT creates an audience-restricted OAuth access token. The resource
// identifier is also the issuer because Phrasely's MCP server hosts the public
// authorization-server and protected-resource metadata.
func SignOAuthJWT(userID, secret, resource string, ttl time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"iss": resource,
		"aud": resource,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(ttl).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign oauth jwt: %w", err)
	}
	return signed, nil
}

// ParseJWT validates a signed JWT and returns the user ID from the sub claim.
// Takes []byte so callers can precompute the secret once rather than allocating on every call.
// Returns an error if the token is invalid, expired, or tampered with.
func ParseJWT(tokenStr string, secret []byte) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		// Ensure the signing method is what we expect — reject tokens signed with other algorithms.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid token claims")
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", fmt.Errorf("missing sub claim")
	}
	return sub, nil
}
