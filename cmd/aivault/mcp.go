package main

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jxherc/aivault/internal/vault"
)

type searchIn struct {
	Query string `json:"query" jsonschema:"a few words about the task or topic. empty returns the newest memories"`
	Limit int    `json:"limit,omitempty" jsonschema:"max results, default 8"`
}

type searchOut struct {
	Results []vault.Hit `json:"results"`
}

type rememberIn struct {
	Text string   `json:"text" jsonschema:"one lasting fact, preference or correction, in plain words"`
	Tags []string `json:"tags,omitempty" jsonschema:"a few short topic words, like git or writing"`
}

type rememberOut struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// "gemini-cli-mcp-client" -> "gemini", "qwen-cli-mcp-client-ai-vault" -> "qwen",
// "codex-mcp-client" -> "codex", "pi-mcp-ai-vault" -> "pi". anything else
// stays as the client sent it
func clientName(n string) string {
	n = strings.ToLower(n)
	if i := strings.Index(n, "-mcp"); i > 0 {
		n = n[:i]
	}
	return strings.TrimSuffix(n, "-cli")
}

func serveMCP(ctx context.Context, dir string) error {
	s := mcp.NewServer(&mcp.Implementation{Name: "ai-vault", Version: version}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "vault_search",
		Description: "search what the user told other ai tools: their projects, servers, preferences and past decisions, plus their skills. call this first for any question about the user or their work, before searching files or saying you don't know.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
		hits, err := vault.Search(dir, in.Query, in.Limit)
		if hits == nil {
			hits = []vault.Hit{}
		}
		return nil, searchOut{Results: hits}, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "vault_remember",
		Description: "save one lasting fact, preference or correction so every ai tool the user has can find it later. call it whenever the user says remember, from now on, i prefer, always or never. never save secrets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in rememberIn) (*mcp.CallToolResult, rememberOut, error) {
		from := ""
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
			from = clientName(p.ClientInfo.Name)
		}
		m, err := vault.Remember(dir, in.Text, from, in.Tags)
		if err != nil {
			return nil, rememberOut{}, err
		}
		return nil, rememberOut{ID: m.ID, Path: m.Path}, nil
	})

	return s.Run(ctx, &mcp.StdioTransport{})
}
