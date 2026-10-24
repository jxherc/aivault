package main

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestNoteTarget(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "remember.md"), []byte("# remember\n"), 0o644)
	for _, c := range [][4]string{
		{"", "Remember: tabs in go", "remember", "tabs in go"},
		{"", "note: tabs", "", "note: tabs"},
		{"mistakes", "x: y", "mistakes", "x: y"},
		{"", "plain fact", "", "plain fact"},
	} {
		f, txt := noteTarget(d, c[0], c[1])
		if f != c[2] || txt != c[3] {
			t.Errorf("%q %q: got %q %q", c[0], c[1], f, txt)
		}
	}
}
