package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerInstructionsPrioritizeSaveIntent(t *testing.T) {
	firstWindow := serverInstructions
	if len(firstWindow) > 512 {
		firstWindow = firstWindow[:512]
	}

	for _, want := range []string{"call add_phrase", "do not ask again", "Never save from a request that only asks"} {
		if !strings.Contains(firstWindow, want) {
			t.Errorf("first 512 instruction characters do not contain %q", want)
		}
	}
}

func TestExplorationInstructionsRequireOneUsefulConnection(t *testing.T) {
	for name, instructions := range map[string]string{
		"server": serverInstructions,
		"tool":   explorePhraseInstructions,
	} {
		for _, want := range []string{
			`exactly one`,
			`third`,
			`One useful connection`,
			`confusable word`,
			`meaningful opposite or contrast`,
			`Never force`,
		} {
			if !strings.Contains(instructions, want) {
				t.Errorf("%s exploration instructions do not contain %q", name, want)
			}
		}
	}
}

func TestExplorationInstructionsRequireCanonicalHeadwordsAcrossChoices(t *testing.T) {
	for _, want := range []string{
		`"unbeknownst to me" and "unbeknownst to the engineering team" both use the headword "unbeknownst to"`,
		`Reuse the exact same canonical headwords`,
	} {
		if !strings.Contains(explorePhraseInstructions, want) {
			t.Errorf("exploration instructions do not contain %q", want)
		}
	}
}

func TestExplorationInstructionsPersonalizeChoicesWithoutInventingUserFacts(t *testing.T) {
	for name, instructions := range map[string]string{
		"server": serverInstructions,
		"tool":   explorePhraseInstructions,
	} {
		instructions = strings.ToLower(instructions)
		for _, want := range []string{
			"choice 1",
			"choice 2",
			"choice 3",
			"user's life",
			"never invent personal facts",
			"only a word or expression",
		} {
			if !strings.Contains(instructions, want) {
				t.Errorf("%s exploration instructions do not contain %q", name, want)
			}
		}
	}
}

func TestRenderPhraseChoicesHandler(t *testing.T) {
	handler := renderPhraseChoicesHandler()
	choice := PhraseChoice{
		Label:       "Everyday example",
		Recommended: true,
		Phrase:      "The delay had a pernicious (gradually harmful) effect on morale.",
		Headwords:   []string{"pernicious"},
		Note:        "Formal; often describes harm that develops gradually.",
		SourceURLs:  []string{"https://www.merriam-webster.com/dictionary/pernicious"},
	}
	secondChoice := PhraseChoice{
		Label:     "Original context",
		Phrase:    "The article described the pernicious effect of misinformation.",
		Headwords: []string{"pernicious"},
	}
	connection := PhraseChoice{
		Label:      "A nuanced near-synonym",
		Phrase:     "The policy's insidious (gradually and subtly harmful) effects only became clear years later.",
		Headwords:  []string{"insidious"},
		Note:       "Insidious also describes gradual harm, but it more strongly suggests that the harm develops subtly or deceptively.",
		SourceURLs: []string{"https://www.merriam-webster.com/dictionary/insidious"},
	}
	choices := []PhraseChoice{choice, secondChoice, connection}

	result, output, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: choices})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("render result = %#v, want one text content item", result)
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("render content = %T, want *mcp.TextContent", result.Content[0])
	}
	for _, want := range []string{
		"nothing has been saved yet",
		"1. Recommended",
		choice.Phrase,
		"2. Original context",
		secondChoice.Phrase,
		"Headwords: pernicious",
		choice.Note,
		"3. " + connection.Label,
		connection.Phrase,
		"Headwords: insidious",
		connection.Note,
		"choose option 1, 2, or 3 to save",
	} {
		if !strings.Contains(textContent.Text, want) {
			t.Errorf("render fallback does not contain %q:\n%s", want, textContent.Text)
		}
	}
	for _, sourceURL := range []string{choice.SourceURLs[0], connection.SourceURLs[0]} {
		if strings.Contains(textContent.Text, sourceURL) {
			t.Errorf("render fallback exposes source_url %q", sourceURL)
		}
	}
	if len(output.Choices) != 3 || output.Choices[0].Phrase != choice.Phrase || output.Choices[2].Phrase != connection.Phrase {
		t.Fatalf("render output = %#v, want original choices", output)
	}

	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{}); err == nil {
		t.Fatal("empty choices did not return an error")
	}
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: choices[:2]}); err == nil {
		t.Fatal("two choices did not return an error")
	}
	secondRecommended := secondChoice
	secondRecommended.Recommended = true
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{choice, secondRecommended, connection}}); err == nil {
		t.Fatal("multiple recommended choices did not return an error")
	}
	invalidConnection := connection
	invalidConnection.Label = "One useful connection"
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{choice, secondChoice, invalidConnection}}); err == nil {
		t.Fatal("generic connection title did not return an error")
	}
	emptyConnection := connection
	emptyConnection.Note = " "
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{choice, secondChoice, emptyConnection}}); err == nil {
		t.Fatal("empty connection note did not return an error")
	}

	blankHeadword := choice
	blankHeadword.Headwords = []string{" "}
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{blankHeadword, secondChoice, connection}}); err == nil {
		t.Fatal("blank headword did not return an error")
	}
	misalignedSourceURLs := choice
	misalignedSourceURLs.Headwords = []string{"pernicious", "effect"}
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{misalignedSourceURLs, secondChoice, connection}}); err == nil {
		t.Fatal("misaligned source_urls did not return an error")
	}
	differentHeadword := secondChoice
	differentHeadword.Headwords = []string{"pernicious effect"}
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{choice, differentHeadword, connection}}); err == nil {
		t.Fatal("different headwords across the first two choices did not return an error")
	}
}

func TestRequireToolAuth(t *testing.T) {
	const protectedResourceMetadataURL = "https://mcp.example.com/.well-known/oauth-protected-resource"

	t.Run("missing token returns ChatGPT OAuth challenge", func(t *testing.T) {
		called := false
		next := func(context.Context, *mcp.CallToolRequest, ExplorePhraseInput) (*mcp.CallToolResult, ExplorePhraseOutput, error) {
			called = true
			return nil, ExplorePhraseOutput{Instructions: "should not run"}, nil
		}
		handler := requireToolAuth("explore_phrase", protectedResourceMetadataURL, next)

		result, _, err := handler(context.Background(), nil, ExplorePhraseInput{Phrase: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatal("authenticated handler ran without a token")
		}
		if result == nil || !result.IsError {
			t.Fatal("missing token did not return a tool error")
		}
		challenges, ok := result.Meta["mcp/www_authenticate"].([]string)
		if !ok || len(challenges) != 1 {
			t.Fatalf("mcp/www_authenticate = %#v, want one challenge", result.Meta["mcp/www_authenticate"])
		}
		for _, want := range []string{protectedResourceMetadataURL, `error="invalid_token"`, `error_description="Authentication required"`} {
			if !strings.Contains(challenges[0], want) {
				t.Errorf("challenge %q does not contain %q", challenges[0], want)
			}
		}
	})

	t.Run("present token calls authenticated handler", func(t *testing.T) {
		called := false
		next := func(context.Context, *mcp.CallToolRequest, ExplorePhraseInput) (*mcp.CallToolResult, ExplorePhraseOutput, error) {
			called = true
			return nil, ExplorePhraseOutput{Instructions: "ok"}, nil
		}
		handler := requireToolAuth("explore_phrase", protectedResourceMetadataURL, next)
		req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: http.Header{
			"Authorization": []string{"Bearer test-token"},
		}}}

		_, output, err := handler(context.Background(), req, ExplorePhraseInput{Phrase: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if !called || output.Instructions != "ok" {
			t.Fatalf("authenticated handler result = %#v, called = %v", output, called)
		}
	})

	t.Run("backend auth rejection returns ChatGPT OAuth challenge", func(t *testing.T) {
		next := func(context.Context, *mcp.CallToolRequest, ExplorePhraseInput) (*mcp.CallToolResult, ExplorePhraseOutput, error) {
			return nil, ExplorePhraseOutput{}, &apiStatusError{StatusCode: http.StatusUnauthorized}
		}
		handler := requireToolAuth("list_phrases", protectedResourceMetadataURL, next)
		req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: http.Header{
			"Authorization": []string{"Bearer expired-token"},
		}}}

		result, _, err := handler(context.Background(), req, ExplorePhraseInput{Phrase: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if result == nil || !result.IsError {
			t.Fatal("backend auth rejection did not return a tool error")
		}
		challenges, ok := result.Meta["mcp/www_authenticate"].([]string)
		if !ok || len(challenges) != 1 || !strings.Contains(challenges[0], `error="invalid_token"`) {
			t.Fatalf("mcp/www_authenticate = %#v, want invalid_token challenge", result.Meta["mcp/www_authenticate"])
		}
	})
}
