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
