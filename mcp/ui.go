package main

import (
	"context"
	_ "embed"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	phraseChoicesTemplateURI = "ui://phrasely/phrase-choices-v1.html"
	mcpAppHTMLMIMEType       = "text/html;profile=mcp-app"
)

// phraseChoicesHTML is kept as a single, dependency-free document so the MCP
// service can continue shipping as one static Go binary.
//
//go:embed ui/phrase-choices.html
var phraseChoicesHTML string

// registerResources attaches the optional MCP Apps UI resources to the server.
func registerResources(server *mcp.Server) {
	server.AddResource(&mcp.Resource{
		URI:         phraseChoicesTemplateURI,
		Name:        "phrasely-phrase-choices",
		Title:       "Phrasely phrase choices",
		Description: "Inline phrase choices with one-click saving to Phrasely.",
		MIMEType:    mcpAppHTMLMIMEType,
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
			{
				URI:      phraseChoicesTemplateURI,
				MIMEType: mcpAppHTMLMIMEType,
				Text:     phraseChoicesHTML,
				Meta: mcp.Meta{
					"ui": map[string]any{
						"prefersBorder": true,
						"csp": map[string]any{
							"connectDomains":  []string{},
							"resourceDomains": []string{},
						},
					},
				},
			},
		}}, nil
	})
}

func renderPhraseChoicesToolMeta() mcp.Meta {
	return mcp.Meta{
		"securitySchemes": []map[string]any{
			{"type": "noauth"},
		},
		"ui": map[string]any{
			"resourceUri": phraseChoicesTemplateURI,
			"visibility":  []string{"model"},
		},
		// ChatGPT compatibility alias; the MCP Apps ui.resourceUri field above
		// remains the canonical association.
		"openai/outputTemplate":          phraseChoicesTemplateURI,
		"openai/toolInvocation/invoking": "Preparing phrase choices…",
		"openai/toolInvocation/invoked":  "Phrase choices ready",
	}
}

func oauthAppToolMeta() mcp.Meta {
	meta := oauthToolMeta()
	meta["ui"] = map[string]any{
		"visibility": []string{"model", "app"},
	}
	// Compatibility for ChatGPT hosts that predate MCP Apps visibility.
	meta["openai/widgetAccessible"] = true
	return meta
}
