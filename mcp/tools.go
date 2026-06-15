package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools attaches the phrasely tools to the MCP server. Each tool
// forwards authToken to backend as a Bearer JWT (Phase 1: a single static
// token; Phase 2 will use per-caller OAuth tokens instead).
func registerTools(server *mcp.Server, api *apiClient, authToken string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_phrases",
		Description: "List the user's saved phrases, optionally filtered by headword.",
	}, listPhrasesHandler(api, authToken))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_phrase",
		Description: "Save a new phrase for the user. Requires the phrase text and at least one headword.",
	}, addPhraseHandler(api, authToken))
}

// ListPhrasesInput is the input schema for the list_phrases tool.
type ListPhrasesInput struct {
	Headword string `json:"headword,omitempty" jsonschema:"filter phrases to those containing this headword"`
}

// ListPhrasesOutput is the output schema for the list_phrases tool.
type ListPhrasesOutput struct {
	Phrases []Phrase `json:"phrases"`
}

func listPhrasesHandler(api *apiClient, authToken string) mcp.ToolHandlerFor[ListPhrasesInput, ListPhrasesOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ListPhrasesInput) (*mcp.CallToolResult, ListPhrasesOutput, error) {
		phrases, err := api.ListPhrases(authToken, in.Headword)
		if err != nil {
			return nil, ListPhrasesOutput{}, err
		}
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

func addPhraseHandler(api *apiClient, authToken string) mcp.ToolHandlerFor[AddPhraseInput, AddPhraseOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in AddPhraseInput) (*mcp.CallToolResult, AddPhraseOutput, error) {
		phrase, err := api.AddPhrase(authToken, AddPhraseRequest{
			Phrase:     in.Phrase,
			Headwords:  in.Headwords,
			Note:       in.Note,
			SourceURLs: in.SourceURLs,
		})
		if err != nil {
			return nil, AddPhraseOutput{}, err
		}
		return nil, AddPhraseOutput{Phrase: phrase}, nil
	}
}
