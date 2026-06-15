package curate

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/andreistefanciprian/phrasely/internal/dictionary"
)

const defaultSystemPromptFile = "/etc/phrasely/system_prompt.txt"

// linkVerificationTimeout bounds how long dictionary link verification may
// take across all headwords for a single curate call.
const linkVerificationTimeout = 5 * time.Second

const mwDictionaryURLPrefix = "https://www.merriam-webster.com/dictionary/"

// Curator calls the OpenAI API to curate a raw phrase input.
type Curator struct {
	client       *openai.Client
	systemPrompt string
	dict         *dictionary.Client
}

// CuratedPhrase is the structured payload returned by the curation model.
type CuratedPhrase struct {
	Phrase          string   `json:"phrase"`
	Headwords       []string `json:"headwords"`
	Note            string   `json:"note"`
	SourceURLs      []string `json:"source_urls"`
	ContentAdjusted bool     `json:"content_adjusted"`
	ValidInput      bool     `json:"valid_input"`
	InvalidReason   string   `json:"invalid_reason"`
}

// NewCurator creates a Curator. dict is optional — pass nil to skip
// Merriam-Webster dictionary link verification and use the AI-suggested
// source_urls as-is.
func NewCurator(apiKey string, dict *dictionary.Client) (*Curator, error) {
	promptBytes, err := os.ReadFile(defaultSystemPromptFile)
	if err != nil {
		return nil, fmt.Errorf("read system prompt file %q: %w", defaultSystemPromptFile, err)
	}

	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return nil, fmt.Errorf("system prompt file %q is empty", defaultSystemPromptFile)
	}

	return &Curator{client: openai.NewClient(apiKey), systemPrompt: prompt, dict: dict}, nil
}

// Curate takes a raw phrase from the user and returns a structured, corrected phrase
// ready to be saved, with headwords, a usage note, and Merriam-Webster URLs.
func (c *Curator) Curate(ctx context.Context, input string) (*CuratedPhrase, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: c.systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: input},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai response: empty choices")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("openai response: empty message content")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse response json object: %w", err)
	}

	var result CuratedPhrase
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if _, ok := raw["valid_input"]; !ok {
		result.ValidInput = true
	}
	if !result.ValidInput && strings.TrimSpace(result.InvalidReason) == "" {
		result.InvalidReason = "No valid expression or meaningful context was provided."
	}

	if result.ValidInput {
		c.verifyLinks(ctx, &result)
	}

	return &result, nil
}

// verifyLinks checks each entry in result.SourceURLs against the
// Merriam-Webster Collegiate Dictionary API and, when an exact entry is
// found for the AI-suggested term or the headword itself, replaces it with
// a verified URL built from the dictionary's canonical headword.
//
// If no exact entry is found, dictionary verification is unavailable (no
// API key configured), or a lookup errors (e.g. network issue), the
// AI-suggested URL is left as-is — even when it doesn't match an exact
// entry, Merriam-Webster's site shows related/"did you mean" results for
// the term, which is more useful than no link at all.
func (c *Curator) verifyLinks(ctx context.Context, result *CuratedPhrase) {
	if c.dict == nil || len(result.SourceURLs) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, linkVerificationTimeout)
	defer cancel()

	var wg sync.WaitGroup
	for i := range result.SourceURLs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			var candidates []string
			if term := termFromURL(result.SourceURLs[i]); term != "" {
				candidates = append(candidates, term)
			}
			if i < len(result.Headwords) && result.Headwords[i] != "" {
				candidates = append(candidates, result.Headwords[i])
			}

			for _, term := range candidates {
				canonical, found, err := c.dict.Lookup(ctx, term)
				if err != nil {
					slog.Warn("dictionary link verification failed", "term", term, "error", err)
					return
				}
				if found {
					result.SourceURLs[i] = dictionary.EntryURL(canonical)
					return
				}
			}
			// No exact entry found — leave the AI-suggested URL as-is.
		}(i)
	}
	wg.Wait()
}

// termFromURL extracts the dictionary lookup term from an
// AI-suggested Merriam-Webster URL, e.g.
// "https://www.merriam-webster.com/dictionary/the%20brunt%20of" -> "the brunt of".
func termFromURL(rawURL string) string {
	if !strings.HasPrefix(rawURL, mwDictionaryURLPrefix) {
		return ""
	}
	term, err := url.PathUnescape(strings.TrimPrefix(rawURL, mwDictionaryURLPrefix))
	if err != nil {
		return ""
	}
	return term
}
