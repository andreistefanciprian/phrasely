package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// apiClient calls the private API on the internal network.
// The browser never talks to the API directly.
type apiClient struct {
	baseURL string
	http    *http.Client
}

func newAPIClient(baseURL string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Proxy forwards a request to the API with the user's JWT and returns the raw response body + status.
func (c *apiClient) Proxy(method, path, jwt string, body io.Reader, contentType string) (int, []byte, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("proxy request: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

// RequestMagicLink asks the API to send a magic link to the given email.
func (c *apiClient) RequestMagicLink(email string) error {
	body := strings.NewReader(fmt.Sprintf(`{"email":%q}`, email))
	resp, err := c.http.Post(c.baseURL+"/auth/request", "application/json", body)
	if err != nil {
		return fmt.Errorf("request magic link: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned %d", resp.StatusCode)
	}
	return nil
}

// ListPhrases fetches all phrases for the authenticated user.
func (c *apiClient) ListPhrases(jwt string) ([]map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v1/phrases", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list phrases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}
	var phrases []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&phrases); err != nil {
		return nil, fmt.Errorf("decode phrases: %w", err)
	}
	return phrases, nil
}

// ValidateOAuthClient checks with the backend that redirect_uri is registered
// for the given client_id. Returns an error if either is unknown or mismatched.
// Called on GET /authorize so we can show an error page (not a redirect) for
// bad params — per RFC 6749 §4.1.2.1 we must never redirect to an invalid URI.
func (c *apiClient) ValidateOAuthClient(clientID, redirectURI string) error {
	resp, err := c.http.Get(c.baseURL + "/internal/oauth/clients/" + url.PathEscape(clientID))
	if err != nil {
		return fmt.Errorf("validate client: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("unknown client_id")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("backend returned %d", resp.StatusCode)
	}
	var client struct {
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&client); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	for _, u := range client.RedirectURIs {
		if u == redirectURI {
			return nil
		}
	}
	return fmt.Errorf("redirect_uri not registered for this client")
}

// IssueAuthCode calls the internal backend endpoint to create an OAuth 2.1
// authorization code. The backend validates client_id, resource, redirect_uri,
// and code_challenge, then stores a short-lived code and returns it.
// The user's JWT must be valid — the backend uses it to bind the code to the user.
func (c *apiClient) IssueAuthCode(jwt, clientID, resource, redirectURI, codeChallenge string) (string, error) {
	body := fmt.Sprintf(`{"client_id":%q,"resource":%q,"redirect_uri":%q,"code_challenge":%q}`,
		clientID, resource, redirectURI, codeChallenge)
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/internal/oauth/authorize",
		strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("issue auth code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("backend returned %d", resp.StatusCode)
	}
	var result struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.Code == "" {
		return "", fmt.Errorf("empty code in response")
	}
	return result.Code, nil
}

// RevokeOAuthTokens revokes all active refresh tokens for the given client on behalf
// of the authenticated user. Called when the user denies an OAuth consent request so
// the client loses access even if it already holds tokens from a prior authorization.
// Errors are non-fatal — the caller should log and still redirect with error=access_denied.
func (c *apiClient) RevokeOAuthTokens(jwt, clientID string) error {
	slog.Debug("oauth: revoking tokens", "client_id", clientID)
	req, err := http.NewRequest(http.MethodDelete,
		c.baseURL+"/internal/oauth/tokens?client_id="+url.QueryEscape(clientID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := c.http.Do(req)
	if err != nil {
		slog.Error("oauth: revoke: request failed", "client_id", clientID, "error", err)
		return fmt.Errorf("revoke oauth tokens: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		slog.Error("oauth: revoke: unexpected status", "client_id", clientID, "status", resp.StatusCode)
		return fmt.Errorf("backend returned %d", resp.StatusCode)
	}
	slog.Info("oauth: tokens revoked", "client_id", clientID)
	return nil
}

// VerifyToken exchanges a magic link token for a JWT.
func (c *apiClient) VerifyToken(token string) (string, error) {
	resp, err := c.http.Get(c.baseURL + "/auth/verify?token=" + url.QueryEscape(token))
	if err != nil {
		return "", fmt.Errorf("verify token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("invalid or expired link")
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("empty token in response")
	}
	return result.Token, nil
}
