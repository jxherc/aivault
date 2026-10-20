package main

import "testing"

func TestClientName(t *testing.T) {
	for in, want := range map[string]string{
		"gemini-cli-mcp-client":        "gemini",
		"qwen-cli-mcp-client-ai-vault": "qwen",
		"codex-mcp-client":             "codex",
		"claude-code":                  "claude-code",
		"opencode":                     "opencode",
		"pi-mcp-ai-vault":              "pi",
		"dsh":                          "dsh",
	} {
		if got := clientName(in); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
}
