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
		Name: "add_phrase",
		Description: `Save a curated phrase to the user's phrase database. Before calling this tool, curate the input:

1. Polish the phrase — fix grammar/spelling, complete if too short, make it sound like something an articulate native speaker would say (usually one sentence).
2. Insert the meaning in parentheses immediately after each headword in the phrase text, e.g. "She was conspicuous (easy to notice) in the crowd."
3. Headwords — treat idioms and fixed expressions as ONE headword (e.g. "in the nick of time", not "nick" + "time"). Return multiple headwords only when the phrase teaches genuinely independent words. The headwords array must contain ONLY the raw expression — no definitions, no parentheses.
4. Note — 1–3 sentences on usage, tone, nuance, or register. If the word or expression has an interesting origin or etymology that would help remember it, include that.
5. source_urls — one Merriam-Webster URL per headword (https://www.merriam-webster.com/dictionary/<lookup-form>, spaces as %20). For verb idioms use the noun lookup form (e.g. "bearing the brunt" → the%20brunt%20of).`,
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
	Phrase     string   `json:"phrase" jsonschema:"curated phrase with each headword's meaning in parentheses immediately after it, e.g. 'She was conspicuous (easy to notice) in the crowd.'"`
	Headwords  []string `json:"headwords" jsonschema:"raw headword or expression only — no definitions or parentheses; treat idioms as a single entry (e.g. 'in the nick of time')"`
	Note       string   `json:"note,omitempty" jsonschema:"1–3 sentences on usage, tone, nuance, or register; include the word's origin or etymology if it has an interesting story that aids memorisation"`
	SourceURLs []string `json:"source_urls,omitempty" jsonschema:"one Merriam-Webster URL per headword aligned by index; format: https://www.merriam-webster.com/dictionary/<lookup-form> with spaces as %20"`
}

// AddPhraseOutput is the output schema for the add_phrase tool.
type AddPhraseOutput struct {
	Phrase Phrase `json:"phrase"`
}

func addPhraseHandler(api *apiClient, jwt string) mcp.ToolHandlerFor[AddPhraseInput, AddPhraseOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in AddPhraseInput) (*mcp.CallToolResult, AddPhraseOutput, error) {
		slog.Debug("tool: add_phrase", "headword_count", len(in.Headwords))
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
