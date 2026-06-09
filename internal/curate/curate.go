package curate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/andreistefanciprian/phrasely/internal/db"
	openai "github.com/sashabaranov/go-openai"
)

const defaultSystemPromptFile = "/etc/phrasely/system_prompt.txt"

// Curator calls the OpenAI API to curate a raw phrase input.
type Curator struct {
	client       *openai.Client
	systemPrompt string
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
func (c *Curator) Curate(ctx context.Context, input string) (*db.CreatePhraseRequest, error) {
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

	var result db.CreatePhraseRequest
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}
