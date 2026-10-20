package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jxherc/ai-vault/internal/vault"
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

func serveMCP(ctx context.Context, dir string) error {
	s := mcp.NewServer(&mcp.Implementation{Name: "ai-vault", Version: version}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "vault_search",
		Description: "search the user's ai vault: memories saved from any ai tool, plus skill descriptions. call at the start of a task.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
		hits, err := vault.Search(dir, in.Query, in.Limit)
		if hits == nil {
			hits = []vault.Hit{}
		}
		return nil, searchOut{Results: hits}, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "vault_remember",
		Description: "save one lasting preference, correction or fact to the user's ai vault so every ai tool can find it later. never save secrets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in rememberIn) (*mcp.CallToolResult, rememberOut, error) {
		from := ""
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
			from = p.ClientInfo.Name
		}
		m, err := vault.Remember(dir, in.Text, from, in.Tags)
		if err != nil {
			return nil, rememberOut{}, err
		}
		return nil, rememberOut{ID: m.ID, Path: m.Path}, nil
	})

	return s.Run(ctx, &mcp.StdioTransport{})
}
