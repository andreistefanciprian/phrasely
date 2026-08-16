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

func TestToolsAdvertisePhraselyTitlesAndIntent(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: serverVersion}, nil)
	const protectedResourceMetadataURL = "https://mcp.example.com/.well-known/oauth-protected-resource"
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
	if len(result.Tools) != 4 {
		t.Fatalf("got %d tools, want 4", len(result.Tools))
	}

	for _, tool := range result.Tools {
		if tool.Title == "" {
			t.Errorf("tool %q has no human-readable title", tool.Name)
		}
		if !strings.Contains(tool.Title, "Phrasely") {
			t.Errorf("tool %q title %q does not mention Phrasely", tool.Name, tool.Title)
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
		if len(schemes) != 1 || schemes[0].Type != "oauth2" || schemes[0].Scopes == nil || len(schemes[0].Scopes) != 0 {
			t.Errorf("tool %q securitySchemes = %s, want one oauth2 scheme with empty scopes", tool.Name, rawSchemes)
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
	if !strings.Contains(addPhraseTool.Description, "do not ask again") {
		t.Error("add_phrase description does not recognize conversational confirmation")
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
}
