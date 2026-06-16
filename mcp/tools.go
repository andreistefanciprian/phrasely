package main

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools attaches the phrasely tools to the MCP server.
// jwt is the per-request OAuth access token forwarded to the backend.
func registerTools(server *mcp.Server, api *apiClient, jwt string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_phrases",
		Description: "List the user's saved phrases, optionally filtered by headword.",
	}, listPhrasesHandler(api, jwt))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_phrase",
		Description: "Save a new phrase for the user. Requires the phrase text and at least one headword.",
	}, addPhraseHandler(api, jwt))
}

// ListPhrasesInput is the input schema for the list_phrases tool.
type ListPhrasesInput struct {
	Headword string `json:"headword,omitempty" jsonschema:"filter phrases to those containing this headword"`
}

// ListPhrasesOutput is the output schema for the list_phrases tool.
type ListPhrasesOutput struct {
	Phrases []Phrase `json:"phrases"`
}

func listPhrasesHandler(api *apiClient, jwt string) mcp.ToolHandlerFor[ListPhrasesInput, ListPhrasesOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ListPhrasesInput) (*mcp.CallToolResult, ListPhrasesOutput, error) {
		slog.Debug("tool: list_phrases", "headword", in.Headword)
		phrases, err := api.ListPhrases(jwt, in.Headword)
		if err != nil {
			slog.Error("tool: list_phrases failed", "error", err)
			return nil, ListPhrasesOutput{}, err
		}
		slog.Debug("tool: list_phrases returned", "count", len(phrases))
		return nil, ListPhrasesOutput{Phrases: phrases}, nil
	}
}

// AddPhraseInput is the input schema for the add_phrase tool.
type AddPhraseInput struct {
	Phrase     string   `json:"phrase" jsonschema:"the phrase or sentence to save"`
	Headwords  []string `json:"headwords" jsonschema:"at least one dictionary headword or fixed expression this phrase illustrates"`
	Note       string   `json:"note,omitempty" jsonschema:"an explanation of the phrase's meaning"`
	SourceURLs []string `json:"source_urls,omitempty" jsonschema:"reference URLs, one per headword, aligned by index"`
}

// AddPhraseOutput is the output schema for the add_phrase tool.
type AddPhraseOutput struct {
	Phrase Phrase `json:"phrase"`
}

func addPhraseHandler(api *apiClient, jwt string) mcp.ToolHandlerFor[AddPhraseInput, AddPhraseOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in AddPhraseInput) (*mcp.CallToolResult, AddPhraseOutput, error) {
		slog.Debug("tool: add_phrase", "headwords", in.Headwords)
		phrase, err := api.AddPhrase(jwt, AddPhraseRequest{
			Phrase:     in.Phrase,
			Headwords:  in.Headwords,
			Note:       in.Note,
			SourceURLs: in.SourceURLs,
		})
		if err != nil {
			slog.Error("tool: add_phrase failed", "error", err)
			return nil, AddPhraseOutput{}, err
		}
		slog.Debug("tool: add_phrase saved", "id", phrase.ID)
		return nil, AddPhraseOutput{Phrase: phrase}, nil
	}
}
