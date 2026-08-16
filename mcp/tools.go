package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const explorePhraseInstructions = `You are an English vocabulary guide helping the user understand and remember a word, phrase, or expression they encountered in real life.

The user gives you a rough phrase, sentence, or fragment they heard in a podcast, conversation, book, movie, article, social media post, or everyday speech.

Do this locally. Do not persist anything — saving is a separate step handled by add_phrase.

1. Understand.
	- Identify the likely target headword or fixed expression.
	- Explain its meaning in the supplied context in simple English.
	- Explain useful nuance where relevant: tone, register, connotation, or difference from a similar word.
	- Treat idioms and fixed expressions as one headword.

2. Preserve the real-world context.
	- Refine the phrase or context the user actually heard: correct grammar and awkward wording.
	- Preserve the user's original meaning, subject matter, tone, and provenance. Never invent provenance.
	- If the source expresses an opinion, controversial claim, political argument, or religious argument, preserve it as the speaker's claim rather than silently converting it into an objective fact.

3. Explore memorable contexts.
	- Generate 1-3 additional contexts using the same headword or expression.
	- Make them easy to understand, vivid, memorable, and natural — the way someone would actually speak or write.
	- Avoid dictionary-style examples. Cover useful, varied contexts where possible, and preferably include at least one simple everyday example.
	- Preserve the exact expression where doing so is natural.
	- The purpose is active vocabulary acquisition, not merely definition. For example, if the original context for "pernicious" is obscure or technical, also produce memorable examples such as misinformation slowly eroding trust or a toxic culture spreading through an organisation.

4. Teach useful usage patterns.
	- When useful, mention common collocations or grammatical constructions, e.g. "conflate X with Y", "fret about/over", "pernicious effect/influence", "mete out punishment", "encroach on". Keep this concise.

5. Do not persist.
	- Never save anything from this tool. Do not produce source_urls, database-ready JSON, a strict headwords field, or a final persistence note — those belong to add_phrase.
	- Offer the refined original context and the memorable alternatives, and let the user choose one. Do not force a choice if the user then gives a direct save instruction that clearly identifies a phrase — in that case, construct the entry and call add_phrase.`

// registerTools attaches the Phrasely tools to the MCP server.
func registerTools(server *mcp.Server, api *apiClient, protectedResourceMetadataURL string) {
	// pFalse is a *bool pointing to false, used for pointer-typed ToolAnnotations fields.
	pFalse := new(bool)

	mcp.AddTool(server, &mcp.Tool{
		Meta:        oauthToolMeta(),
		Name:        "list_phrases",
		Title:       "List Phrasely phrases",
		Description: "List phrases in the user's Phrasely collection, newest first, optionally filtered by matching headword text. Use for recent phrases, browsing, or headword lookup.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: pFalse,
		},
	}, requireToolAuth("list_phrases", protectedResourceMetadataURL, listPhrasesHandler(api)))

	mcp.AddTool(server, &mcp.Tool{
		Meta:        oauthToolMeta(),
		Name:        "sample_phrases",
		Title:       "Sample Phrasely phrases",
		Description: "Randomly select phrases from the user's Phrasely collection for review, quizzes, or speaking practice. Do not use when the user wants a complete or recent list.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: pFalse,
		},
	}, requireToolAuth("sample_phrases", protectedResourceMetadataURL, samplePhrasesHandler(api)))

	mcp.AddTool(server, &mcp.Tool{
		Meta:        oauthToolMeta(),
		Name:        "explore_phrase",
		Title:       "Explore a phrase with Phrasely",
		Description: `Explore a word, phrase, or expression the user encountered in real life. Use this when the user wants to understand an expression, refine the context in which they heard it, or find memorable examples using the same headword. Return Phrasely's learning instructions for the assistant to apply locally. This tool does not call OpenAI and does not persist data. Do not use it when the user has already chosen a finished phrase and simply asks to save it.`,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: pFalse,
		},
	}, requireToolAuth("explore_phrase", protectedResourceMetadataURL, explorePhraseHandler()))

	mcp.AddTool(server, &mcp.Tool{
		Meta:        oauthToolMeta(),
		Name:        "add_phrase",
		Title:       "Add a phrase to Phrasely",
		Description: `Save one finished phrase entry to Phrasely. Construct the phrase, headwords, note, and source_urls locally first — see each field's description for the construction rules — then call this tool. There is no need to call explore_phrase first if the user already supplied or chose a clear phrase. A clear request such as "add it", "save it", "add this one", or "add that to Phrasely" is already confirmation; do not ask again. Never call this tool for a request that only asks for an explanation, definition, or rewrite. Ask which phrase only if the reference is genuinely ambiguous and cannot be resolved from conversation context.`,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: pFalse,
			OpenWorldHint:   pFalse,
		},
	}, requireToolAuth("add_phrase", protectedResourceMetadataURL, addPhraseHandler(api)))
}

// oauthToolMeta declares that every Phrasely tool requires OAuth. Phrasely
// currently grants one connector-wide permission, so the scopes array is empty.
// ChatGPT reads securitySchemes from _meta for MCP SDK compatibility.
func oauthToolMeta() mcp.Meta {
	return mcp.Meta{
		"securitySchemes": []map[string]any{
			{"type": "oauth2", "scopes": []string{}},
		},
	}
}

// requireToolAuth lets initialize and tools/list remain public while ensuring
// every actual tool call carries an OAuth access token. The backend performs
// cryptographic validation for tools that access user data.
func requireToolAuth[In, Out any](toolName, protectedResourceMetadataURL string, next mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		if requestBearer(req) != "" {
			slog.Debug("mcp: authenticated tool call", "tool", toolName)
			return next(ctx, req, in)
		}

		slog.Info("mcp: tool authentication challenged", "tool", toolName, "reason", "missing_or_invalid_bearer")
		var zero Out
		challenge := fmt.Sprintf(
			`Bearer resource_metadata="%s", error="invalid_token", error_description="Authentication required"`,
			protectedResourceMetadataURL,
		)
		return &mcp.CallToolResult{
			Meta: mcp.Meta{
				"mcp/www_authenticate": []string{challenge},
			},
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Authentication required: no access token provided."},
			},
			IsError: true,
		}, zero, nil
	}
}

// requestBearer reads the current HTTP request rather than the initialization
// request, because ChatGPT can add its OAuth token later on the same MCP session.
func requestBearer(req *mcp.CallToolRequest) string {
	if req == nil || req.Extra == nil {
		return ""
	}
	return bearerToken(req.Extra.Header.Get("Authorization"))
}

// ExplorePhraseInput is the input schema for the explore_phrase tool.
type ExplorePhraseInput struct {
	Phrase string `json:"phrase" jsonschema:"raw phrase, expression, or context the user wants to explore"`
}

// ExplorePhraseOutput is the output schema for the explore_phrase tool.
type ExplorePhraseOutput struct {
	Instructions string `json:"instructions"`
	Phrase       string `json:"phrase,omitempty"`
}

func explorePhraseHandler() mcp.ToolHandlerFor[ExplorePhraseInput, ExplorePhraseOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ExplorePhraseInput) (*mcp.CallToolResult, ExplorePhraseOutput, error) {
		slog.Debug("tool: explore_phrase", "phrase_len", len(in.Phrase))
		return nil, ExplorePhraseOutput{Instructions: explorePhraseInstructions, Phrase: in.Phrase}, nil
	}
}

// SamplePhrasesInput is the input schema for the sample_phrases tool.
type SamplePhrasesInput struct {
	Count int `json:"count,omitempty" jsonschema:"number of phrases to sample (default 1, max 10)"`
}

// SamplePhrasesOutput is the output schema for the sample_phrases tool.
type SamplePhrasesOutput struct {
	Total   int             `json:"total"`
	Phrases []PhraseSummary `json:"phrases"`
}

func samplePhrasesHandler(api *apiClient) mcp.ToolHandlerFor[SamplePhrasesInput, SamplePhrasesOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SamplePhrasesInput) (*mcp.CallToolResult, SamplePhrasesOutput, error) {
		jwt := requestBearer(req)
		count := in.Count
		if count < 1 {
			count = 1
		}
		if count > 10 {
			count = 10
		}
		slog.Debug("tool: sample_phrases", "count", count)
		phrases, err := api.GetRandomPhrases(jwt, count)
		if err != nil {
			slog.Error("tool: sample_phrases failed", "error", err)
			return nil, SamplePhrasesOutput{}, err
		}
		slog.Debug("tool: sample_phrases returned", "count", len(phrases))
		return nil, SamplePhrasesOutput{Total: len(phrases), Phrases: phrases}, nil
	}
}

// ListPhrasesInput is the input schema for the list_phrases tool.
type ListPhrasesInput struct {
	Headword string `json:"headword,omitempty" jsonschema:"filter phrases to those containing this headword"`
}

// ListPhrasesOutput is the output schema for the list_phrases tool.
type ListPhrasesOutput struct {
	Total   int             `json:"total"`
	Phrases []PhraseSummary `json:"phrases"`
}

func listPhrasesHandler(api *apiClient) mcp.ToolHandlerFor[ListPhrasesInput, ListPhrasesOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ListPhrasesInput) (*mcp.CallToolResult, ListPhrasesOutput, error) {
		jwt := requestBearer(req)
		slog.Debug("tool: list_phrases", "headword", in.Headword)
		phrases, err := api.ListPhrasesSummary(jwt, in.Headword)
		if err != nil {
			slog.Error("tool: list_phrases failed", "error", err)
			return nil, ListPhrasesOutput{}, err
		}
		slog.Debug("tool: list_phrases returned", "count", len(phrases))
		return nil, ListPhrasesOutput{Total: len(phrases), Phrases: phrases}, nil
	}
}

// AddPhraseInput is the input schema for the add_phrase tool.
type AddPhraseInput struct {
	Phrase     string   `json:"phrase" jsonschema:"Polished, natural English, usually one memorable sentence, preserving the user's original meaning and context. Insert a short plain-English meaning in parentheses immediately after each headword or expression, e.g. 'We sat around the campfire, yapping away (chatting continuously) until midnight.' No Markdown formatting."`
	Headwords  []string `json:"headwords" jsonschema:"Raw word or expression only, one per entry — no parentheses, no definitions, no quotation marks, no explanatory text. Treat an idiom or fixed expression as a single headword, using its natural taught form, e.g. ['yapping away']."`
	Note       string   `json:"note,omitempty" jsonschema:"1-3 concise sentences on usage, nuance, tone, register, or collocations. Do not repeat the phrase unnecessarily."`
	SourceURLs []string `json:"source_urls,omitempty" jsonschema:"One Merriam-Webster URL per headword, aligned by index: https://www.merriam-webster.com/dictionary/<lookup form>, URL-encoding spaces as %20. The lookup form is the actual dictionary entry title and is often not identical to the headword text: an inflected verb should normally resolve to its base/infinitive form, and an idiom built around a common light verb (make, take, give, etc.) often has its real Merriam-Webster entry filed under the noun phrase alone, with that light verb dropped — prefer that form when it applies. The headwords field itself keeps the natural taught form; only the source_urls lookup form changes. Example: headword 'yapping away' -> https://www.merriam-webster.com/dictionary/yap."`
}

// AddPhraseOutput is the output schema for the add_phrase tool.
// Returns only phrase and headwords — the internal ID and other fields are omitted
// as they are not needed by the model and should not be exposed.
type AddPhraseOutput struct {
	Phrase PhraseSummary `json:"phrase"`
}

func addPhraseHandler(api *apiClient) mcp.ToolHandlerFor[AddPhraseInput, AddPhraseOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in AddPhraseInput) (*mcp.CallToolResult, AddPhraseOutput, error) {
		jwt := requestBearer(req)
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
		slog.Debug("tool: add_phrase saved")
		return nil, AddPhraseOutput{Phrase: PhraseSummary{
			Phrase:    phrase.Phrase,
			Headwords: phrase.Headwords,
		}}, nil
	}
}
