package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerInstructionsPrioritizeSaveIntent(t *testing.T) {
	if !strings.Contains(serverInstructions, "do not ask again") {
		t.Errorf("instructions do not contain %q", "do not ask again")
	}
	if !strings.Contains(serverInstructions, "Never save from a request that only asks") {
		t.Errorf("instructions do not contain %q", "Never save from a request that only asks")
	}
}

func TestToolsAdvertisePhraselyTitlesAndIntent(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: serverVersion}, nil)
	registerTools(server, newAPIClient("http://localhost:8080"), "test-token")

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
