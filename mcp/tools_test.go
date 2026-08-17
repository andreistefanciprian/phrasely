package main

import (
	"context"
	"encoding/json"
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
			`One useful connection:`,
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

func TestToolsAdvertisePhraselyTitlesAndIntent(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: serverVersion}, nil)
	const protectedResourceMetadataURL = "https://mcp.example.com/.well-known/oauth-protected-resource"
	registerResources(server)
	registerTools(server, newAPIClient("http://localhost:8080"), protectedResourceMetadataURL)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 5 {
		t.Fatalf("got %d tools, want 5", len(result.Tools))
	}

	for _, tool := range result.Tools {
		if tool.Title == "" {
			t.Errorf("tool %q has no human-readable title", tool.Name)
		}
		if !strings.Contains(tool.Title, "Phrasely") {
			t.Errorf("tool %q title %q does not mention Phrasely", tool.Name, tool.Title)
		}

		if tool.Annotations == nil {
			t.Errorf("tool %q has no action annotations", tool.Name)
		} else {
			wantReadOnly := tool.Name != "add_phrase"
			if tool.Annotations.ReadOnlyHint != wantReadOnly {
				t.Errorf("tool %q readOnlyHint = %v, want %v", tool.Name, tool.Annotations.ReadOnlyHint, wantReadOnly)
			}
			if tool.Annotations.IdempotentHint != wantReadOnly {
				t.Errorf("tool %q idempotentHint = %v, want %v", tool.Name, tool.Annotations.IdempotentHint, wantReadOnly)
			}
			if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
				t.Errorf("tool %q destructiveHint = %v, want explicit false", tool.Name, tool.Annotations.DestructiveHint)
			}
			if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
				t.Errorf("tool %q openWorldHint = %v, want explicit false", tool.Name, tool.Annotations.OpenWorldHint)
			}
		}

		rawSchemes, err := json.Marshal(tool.Meta["securitySchemes"])
		if err != nil {
			t.Fatalf("marshal security schemes for %q: %v", tool.Name, err)
		}
		var schemes []struct {
			Type   string   `json:"type"`
			Scopes []string `json:"scopes"`
		}
		if err := json.Unmarshal(rawSchemes, &schemes); err != nil {
			t.Fatalf("decode security schemes for %q: %v", tool.Name, err)
		}
		if len(schemes) != 1 {
			t.Errorf("tool %q securitySchemes = %s, want exactly one scheme", tool.Name, rawSchemes)
			continue
		}
		if tool.Name == "explore_phrase" || tool.Name == "render_phrase_choices" {
			if schemes[0].Type != "noauth" {
				t.Errorf("tool %q securitySchemes = %s, want noauth", tool.Name, rawSchemes)
			}
		} else if schemes[0].Type != "oauth2" || schemes[0].Scopes == nil || len(schemes[0].Scopes) != 0 {
			t.Errorf("tool %q securitySchemes = %s, want oauth2 with empty scopes", tool.Name, rawSchemes)
		}
	}

	byName := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		byName[tool.Name] = tool
	}
	explorePhraseTool, ok := byName["explore_phrase"]
	if !ok {
		t.Fatal("explore_phrase tool is missing")
	}
	addPhraseTool, ok := byName["add_phrase"]
	if !ok {
		t.Fatal("add_phrase tool is missing")
	}

	if !strings.Contains(explorePhraseTool.Description, "Do not use it when the user has already chosen a finished phrase") {
		t.Error("explore_phrase description does not distinguish exploration from save intent")
	}
	if !strings.Contains(explorePhraseTool.Description, "one concise high-value learning connection") {
		t.Error("explore_phrase description does not require a learning connection")
	}
	if !strings.Contains(addPhraseTool.Description, "do not ask again") {
		t.Error("add_phrase description does not recognize conversational confirmation")
	}
	renderTool, ok := byName["render_phrase_choices"]
	if !ok {
		t.Fatal("render_phrase_choices tool is missing")
	}
	uiMeta, ok := renderTool.Meta["ui"].(map[string]any)
	if !ok || uiMeta["resourceUri"] != phraseChoicesTemplateURI {
		t.Fatalf("render_phrase_choices ui metadata = %#v, want resource URI %q", renderTool.Meta["ui"], phraseChoicesTemplateURI)
	}
	addUIMeta, ok := addPhraseTool.Meta["ui"].(map[string]any)
	if !ok {
		t.Fatalf("add_phrase ui metadata = %#v, want app visibility", addPhraseTool.Meta["ui"])
	}
	visibilityJSON, err := json.Marshal(addUIMeta["visibility"])
	if err != nil {
		t.Fatal(err)
	}
	var visibility []string
	if err := json.Unmarshal(visibilityJSON, &visibility); err != nil {
		t.Fatal(err)
	}
	if len(visibility) != 2 || visibility[0] != "model" || visibility[1] != "app" {
		t.Fatalf("add_phrase ui visibility = %#v, want [model app]", addUIMeta["visibility"])
	}
}

func TestPhraseChoicesResource(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: serverVersion}, nil)
	registerResources(server)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: phraseChoicesTemplateURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("got %d resource contents, want 1", len(result.Contents))
	}
	content := result.Contents[0]
	if content.MIMEType != mcpAppHTMLMIMEType {
		t.Fatalf("resource MIME type = %q, want %q", content.MIMEType, mcpAppHTMLMIMEType)
	}
	uiMeta, ok := content.Meta["ui"].(map[string]any)
	if !ok || uiMeta["domain"] != phraseChoicesWidgetDomain {
		t.Fatalf("resource ui metadata = %#v, want domain %q", content.Meta["ui"], phraseChoicesWidgetDomain)
	}
	legacyCSP, ok := content.Meta["openai/widgetCSP"].(map[string]any)
	if !ok {
		t.Fatalf("resource CSP compatibility metadata = %#v, want object", content.Meta["openai/widgetCSP"])
	}
	for _, field := range []string{"connect_domains", "resource_domains"} {
		domainsJSON, err := json.Marshal(legacyCSP[field])
		if err != nil {
			t.Fatalf("marshal resource CSP compatibility %s: %v", field, err)
		}
		if string(domainsJSON) != "[]" {
			t.Errorf("resource CSP compatibility %s = %#v, want empty string array", field, legacyCSP[field])
		}
	}
	for _, want := range []string{
		"Choose a phrase to save",
		`request("ui/initialize"`,
		`request("tools/call"`,
		`name: "add_phrase"`,
		"ResizeObserver",
		`notify("ui/notifications/size-changed"`,
		"window.openai.notifyIntrinsicHeight()",
		"window.openai?.toolResponseMetadata",
		"window.openai?.toolInput",
		"window.openai?.widgetState",
		"window.openai.setWidgetState(state)",
		"savedChoiceIndexes.add(index)",
		"const isSaved = savedChoiceIndexes.has(index)",
		"resizeObserver?.disconnect()",
		"recommended-marker",
	} {
		if !strings.Contains(content.Text, want) {
			t.Errorf("phrase choice UI does not contain %q", want)
		}
	}
	if strings.Contains(content.Text, `<img class="logo"`) {
		t.Error("phrase choice UI repeats the Phrasely logo inside the widget")
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

	result, output, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{choice, secondChoice}})
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
		"choose a numbered option to save",
	} {
		if !strings.Contains(textContent.Text, want) {
			t.Errorf("render fallback does not contain %q:\n%s", want, textContent.Text)
		}
	}
	if strings.Contains(textContent.Text, choice.SourceURLs[0]) {
		t.Error("render fallback exposes source_urls")
	}
	if len(output.Choices) != 2 || output.Choices[0].Phrase != choice.Phrase || output.Choices[1].Phrase != secondChoice.Phrase {
		t.Fatalf("render output = %#v, want original choices", output)
	}

	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{}); err == nil {
		t.Fatal("empty choices did not return an error")
	}
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{
		choice,
		choice,
	}}); err == nil {
		t.Fatal("multiple recommended choices did not return an error")
	}

	blankHeadword := choice
	blankHeadword.Headwords = []string{" "}
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{blankHeadword}}); err == nil {
		t.Fatal("blank headword did not return an error")
	}

	misalignedSourceURLs := choice
	misalignedSourceURLs.Headwords = []string{"pernicious", "effect"}
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{misalignedSourceURLs}}); err == nil {
		t.Fatal("misaligned source_urls did not return an error")
	}

	differentHeadword := secondChoice
	differentHeadword.Headwords = []string{"pernicious effect"}
	if _, _, err := handler(context.Background(), nil, RenderPhraseChoicesInput{Choices: []PhraseChoice{choice, differentHeadword}}); err == nil {
		t.Fatal("different headwords across choices did not return an error")
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
