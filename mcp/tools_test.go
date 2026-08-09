package main

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerInstructionsPrioritizeSaveIntent(t *testing.T) {
	firstWindow := serverInstructions
	if len(firstWindow) > 512 {
		firstWindow = firstWindow[:512]
	}

	for _, want := range []string{"call curate before add_phrase", "do not ask again", "Never save from a request that only asks"} {
		if !strings.Contains(firstWindow, want) {
			t.Errorf("first 512 instruction characters do not contain %q", want)
		}
	}
}

func TestToolsAdvertisePhraselyTitlesAndIntent(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "phrasely", Version: "0.3.3"}, nil)
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
	if !strings.Contains(byName["curate"].Description, "Do not call it merely") {
		t.Error("curate description does not distinguish discussion from save intent")
	}
	if !strings.Contains(byName["add_phrase"].Description, "do not ask again") {
		t.Error("add_phrase description does not recognize conversational confirmation")
	}
}
