package main

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const curateInstructions = `You are an English phrase curator for a personal phrase database.

The user gives you a rough phrase, sentence, or fragment they heard in a podcast, conversation, book, movie, article, social media post, or everyday speech.

Your goal is to transform it into a memorable, natural phrase that teaches the target word or expression in context.

Rules:

1. Curate the phrase.
	- Correct grammar, spelling, punctuation, and awkward wording.
	- Keep the user's original meaning, tone, and subject matter.
	- Preserve the target word or expression.
	- If the phrase is incomplete, complete it naturally.
	- If the phrase is too short, vague, or lacks context, enrich it with a realistic continuation.
	- Do not overexpand. Usually return one sentence.
	- The final phrase should sound like something an articulate native speaker might actually say.
	- Prefer vivid, memorable, podcast quality examples over dictionary style examples.

2. Identify the headword or expression being illustrated.
	- Treat idioms and fixed expressions as ONE headword.
	- Return multiple headwords only when the phrase genuinely teaches multiple independent words or expressions.
	- If multiple words form a single idiom or fixed expression, return only one headword.

3. Insert a short meaning in parentheses immediately after each headword or expression.
	- If the user wrote "(?)", replace it with the meaning.
	- If there is no "(?)", add the meaning after the headword.
	- Meanings must be short, clear, natural, easy to understand, and specific to the context.
	- Avoid long dictionary definitions.

4. Generate the headwords field.
	- The headwords field must contain ONLY the raw word or expression.
	- Do NOT include definitions.
	- Do NOT include parentheses.
	- Do NOT include quotation marks inside the headword text.
	- Do NOT include explanatory text.
	- Do NOT include punctuation unless it is part of the expression.

5. Write a concise note.
	- Explain how the headword is used in this context.
	- Mention tone, nuance, register, or etymology only if useful.
	- Keep it to 1-3 short sentences.

7. Generate one Merriam-Webster URL for each headword.
	- Format: https://www.merriam-webster.com/dictionary/<lookup form>
	- URL encode spaces as %20.
	- The <lookup form> is the form that would appear as a Merriam-Webster dictionary entry title, which is often not identical to the headword text.
	- If the headword is an idiom built around a verb, use the verb's base/infinitive form, not the inflected form from the phrase.
	- For many such verb idioms, Merriam-Webster's actual entry is filed under the noun phrase without the verb. When the idiom centers on a noun preceded by a common light verb, prefer the lookup form without that verb.
	- The headwords field should still contain the natural form of the expression as used or taught; only the source_urls lookup form changes per the rules above.`

// registerTools attaches the phrasely tools to the MCP server.
// jwt is the per-request OAuth access token forwarded to the backend.
func registerTools(server *mcp.Server, api *apiClient, jwt string) {
	// pFalse is a *bool pointing to false, used for pointer-typed ToolAnnotations fields.
	pFalse := new(bool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_phrases",
		Description: "List the user's saved phrases (phrase and headwords only), optionally filtered by headword.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: pFalse,
		},
	}, listPhrasesHandler(api, jwt))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sample_phrases",
		Description: "Randomly sample N phrases from the user's saved collection. Use this when the user asks to pick, show, quiz, or suggest a random phrase or a few random phrases — not when they want to list all phrases.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: pFalse,
		},
	}, samplePhrasesHandler(api, jwt))

	mcp.AddTool(server, &mcp.Tool{
		Name: "curate",
		Description: `Use this first for any raw phrase input from the user. When the user mentions a phrase, wants to save an expression they heard in conversation/podcast/movie/book, or provides a fragment or example sentence, you MUST call this tool to receive the curation rules. Apply those rules locally to transform the raw text into a fully curated phrase with meanings in parentheses, headwords, a usage note, and Merriam-Webster URLs. Only then call add_phrase with the finished, curated result. This tool does not call OpenAI and does not persist data.`,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: pFalse,
		},
	}, curateHandler())

	mcp.AddTool(server, &mcp.Tool{
		Name: "add_phrase",
		Description: `Save a phrase that has been fully curated by the assistant. CRITICAL: Never call this tool directly on raw user input. If the user provides a raw phrase, fragment, or example from speech, you MUST call curate first, receive the curation rules, apply them locally, and then call this tool with the finished result. Only skip curate if the user explicitly provides a fully curated phrase with headwords and their meanings already formatted in parentheses within the phrase text (e.g., 'She was conspicuous (easy to notice) in the crowd.').`,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: pFalse,
			OpenWorldHint:   pFalse,
		},
	}, addPhraseHandler(api, jwt))
}

// CurateInput is the input schema for the curate tool.
type CurateInput struct {
	Phrase string `json:"phrase" jsonschema:"raw phrase or expression to curate before saving"`
}

// CurateOutput is the output schema for the curate tool.
type CurateOutput struct {
	Instructions string `json:"instructions"`
	Phrase       string `json:"phrase,omitempty"`
}

func curateHandler() mcp.ToolHandlerFor[CurateInput, CurateOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CurateInput) (*mcp.CallToolResult, CurateOutput, error) {
		slog.Debug("tool: curate", "phrase_len", len(in.Phrase))
		return nil, CurateOutput{Instructions: curateInstructions, Phrase: in.Phrase}, nil
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

func samplePhrasesHandler(api *apiClient, jwt string) mcp.ToolHandlerFor[SamplePhrasesInput, SamplePhrasesOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SamplePhrasesInput) (*mcp.CallToolResult, SamplePhrasesOutput, error) {
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

func listPhrasesHandler(api *apiClient, jwt string) mcp.ToolHandlerFor[ListPhrasesInput, ListPhrasesOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ListPhrasesInput) (*mcp.CallToolResult, ListPhrasesOutput, error) {
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
	Phrase     string   `json:"phrase" jsonschema:"already curated phrase with each headword's meaning in parentheses immediately after it"`
	Headwords  []string `json:"headwords" jsonschema:"raw headword or expression only; treat idioms as a single entry"`
	Note       string   `json:"note,omitempty" jsonschema:"1-3 sentences on usage, tone, nuance, or register; include etymology if useful"`
	SourceURLs []string `json:"source_urls,omitempty" jsonschema:"one Merriam-Webster URL per headword aligned by index"`
}

// AddPhraseOutput is the output schema for the add_phrase tool.
// Returns only phrase and headwords — the internal ID and other fields are omitted
// as they are not needed by the model and should not be exposed.
type AddPhraseOutput struct {
	Phrase PhraseSummary `json:"phrase"`
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
		slog.Debug("tool: add_phrase saved")
		return nil, AddPhraseOutput{Phrase: PhraseSummary{
			Phrase:    phrase.Phrase,
			Headwords: phrase.Headwords,
		}}, nil
	}
}
