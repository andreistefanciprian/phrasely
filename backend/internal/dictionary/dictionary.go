// Package dictionary verifies Merriam-Webster dictionary links using the
// Merriam-Webster Collegiate Dictionary API.
package dictionary

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const apiBaseURL = "https://www.dictionaryapi.com/api/v3/references/collegiate/json"

// Client looks up dictionary entries via the Merriam-Webster Collegiate
// Dictionary API and caches results in memory, since the same headwords
// recur across curate calls and dictionary entries don't change.
type Client struct {
	apiKey     string
	httpClient *http.Client

	mu    sync.RWMutex
	cache map[string]lookupResult
}

type lookupResult struct {
	canonical string
	found     bool
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{},
		cache:      make(map[string]lookupResult),
	}
}

// entry mirrors the subset of the Collegiate Dictionary API entry shape we need.
type entry struct {
	HWI struct {
		HW string `json:"hw"`
	} `json:"hwi"`
}

// Lookup returns the canonical Merriam-Webster headword form for term, e.g.
// "the brunt of" for "bearing the brunt". found is false if term (or a close
// variant) has no dictionary entry — the API returned only spelling
// suggestions or nothing at all.
func (c *Client) Lookup(ctx context.Context, term string) (canonical string, found bool, err error) {
	key := strings.ToLower(strings.TrimSpace(term))
	if key == "" {
		return "", false, nil
	}

	c.mu.RLock()
	if cached, ok := c.cache[key]; ok {
		c.mu.RUnlock()
		return cached.canonical, cached.found, nil
	}
	c.mu.RUnlock()

	reqURL := fmt.Sprintf("%s/%s?key=%s", apiBaseURL, url.PathEscape(key), url.QueryEscape(c.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", false, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("dictionary api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("dictionary api request: unexpected status %d", resp.StatusCode)
	}

	// The API returns either a list of entry objects (match found) or a list
	// of spelling-suggestion strings (no match) — decode generically and skip
	// elements that aren't entry objects.
	var raw []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", false, fmt.Errorf("decode dictionary api response: %w", err)
	}

	for _, item := range raw {
		var e entry
		if err := json.Unmarshal(item, &e); err != nil {
			continue
		}
		if e.HWI.HW == "" {
			continue
		}
		canonical = strings.ReplaceAll(e.HWI.HW, "*", "")
		found = true
		break
	}

	c.mu.Lock()
	c.cache[key] = lookupResult{canonical: canonical, found: found}
	c.mu.Unlock()

	return canonical, found, nil
}

// EntryURL builds a Merriam-Webster dictionary entry URL for a canonical headword.
func EntryURL(canonical string) string {
	return "https://www.merriam-webster.com/dictionary/" + url.PathEscape(canonical)
}
