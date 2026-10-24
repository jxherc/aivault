package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jxherc/aivault/internal/vault"
	"github.com/jxherc/aivault/internal/wire"
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
	File string   `json:"file,omitempty" jsonschema:"also add it as a line to this vault file, like remember or mistakes. the vault's AGENTS.md says which file fits"`
}

type rememberOut struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	File string `json:"file,omitempty"`
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
		Description: "save one lasting fact, preference or correction so every ai tool the user has can find it later. call it whenever the user says remember, from now on, i prefer, always or never, or corrects you. pass file (like remember or mistakes) when the vault's rules say the fact belongs in one. never save secrets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in rememberIn) (*mcp.CallToolResult, rememberOut, error) {
		from := ""
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
			from = clientName(p.ClientInfo.Name)
		}
		file, text := noteTarget(dir, in.File, in.Text)
		if vault.LooksSecret(text) {
			return nil, rememberOut{}, errors.New("that looks like a password or key. the vault doesn't keep those")
		}
		var out rememberOut
		if file != "" {
			p, err := vault.AddNote(dir, file, text, from)
			if err != nil {
				return nil, rememberOut{}, err
			}
			out.File = p
			// a top-level file is part of every tool's rules. tools that keep a
			// copy would miss the new line until the next sync
			if filepath.Dir(p) == filepath.Clean(dir) {
				refresh(dir)
			}
		}
		tags := in.Tags
		if file != "" {
			tags = append(tags, file)
		}
		m, err := vault.Remember(dir, text, from, tags)
		if err != nil {
			return nil, out, err
		}
		out.ID, out.Path = m.ID, m.Path
		return nil, out, nil
	})

	return s.Run(ctx, &mcp.StdioTransport{})
}

// tools that can't pass `file` are told to write "remember: ..." instead
func noteTarget(dir, file, text string) (string, string) {
	if file != "" {
		return file, text
	}
	if name, rest, ok := strings.Cut(text, ":"); ok && slices.Contains(vault.NoteFiles(dir), strings.ToLower(strings.TrimSpace(name))) {
		return strings.ToLower(strings.TrimSpace(name)), strings.TrimSpace(rest)
	}
	return "", text
}

// best effort: the note is saved either way, and `aivault sync` fixes copies
func refresh(dir string) {
	e, s, err := load()
	if err == nil && filepath.Clean(s.Vault) == filepath.Clean(dir) {
		err = wire.RefreshRules(e, s)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ai-vault: saved, but couldn't update the tools' copies of the rules:", err)
	}
}
