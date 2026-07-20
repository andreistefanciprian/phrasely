package curate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

const defaultSystemPromptFile = "/etc/phrasely/system_prompt.txt"

// Curator calls the OpenAI API to curate a raw phrase input.
type Curator struct {
	client       *openai.Client
	systemPrompt string
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

func NewCurator(apiKey string) (*Curator, error) {
	promptBytes, err := os.ReadFile(defaultSystemPromptFile)
	if err != nil {
		return nil, fmt.Errorf("read system prompt file %q: %w", defaultSystemPromptFile, err)
	}

	prompt := strings.TrimSpace(string(promptBytes))
	if prompt == "" {
		return nil, fmt.Errorf("system prompt file %q is empty", defaultSystemPromptFile)
	}

	return &Curator{client: openai.NewClient(apiKey), systemPrompt: prompt}, nil
}

// Curate takes a raw phrase from the user and returns a structured, corrected phrase
// ready to be saved, with headwords, a usage note, and Merriam-Webster URLs.
func (c *Curator) Curate(ctx context.Context, input string) (*CuratedPhrase, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: "gpt-5-mini",
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
	return &result, nil
}
