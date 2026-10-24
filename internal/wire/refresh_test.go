package wire

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// a new top-level vault file reaches every tool, a deleted one leaves every
// tool, and remove still leaves the tools' files as they were
func TestRefreshRules(t *testing.T) {
	f := newFake(t)
	e := f.e
	oc := filepath.Join(e.OpencodeDir, "opencode.json")
	put(t, oc, "{\n  \"instructions\": [\"mine.md\"]\n}\n")
	vdir := filepath.Join(e.Home, "v")
	s, _ := LoadState(e)
	if err := Apply(e, s, vdir, []string{"claude", "codex", "opencode"}, false, io.Discard); err != nil {
		t.Fatal(err)
	}

	put(t, filepath.Join(vdir, "mistakes.md"), "# mistakes\n- ran tests on real data -> use a temp dir\n")
	if err := RefreshRules(e, s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, filepath.Join(e.CodexDir, "AGENTS.md")), "use a temp dir") {
		t.Fatal("codex copy missed the new file")
	}
	if !strings.Contains(get(t, filepath.Join(e.ClaudeDir, "CLAUDE.md")), "mistakes.md") {
		t.Fatal("claude doesn't import the new file")
	}
	if !strings.Contains(get(t, oc), "/v/mistakes.md") {
		t.Fatalf("opencode doesn't list the new file:\n%s", get(t, oc))
	}
	if n := strings.Count(strings.Join(f.calls, "\n"), "mcp add"); n != 1 {
		t.Fatalf("refresh touched claude's mcp server (%d adds)", n)
	}

	os.Remove(filepath.Join(vdir, "mistakes.md"))
	RefreshRules(e, s)
	if strings.Contains(get(t, oc), "mistakes.md") || strings.Contains(get(t, filepath.Join(e.CodexDir, "AGENTS.md")), "temp dir") {
		t.Fatal("deleted file still wired")
	}

	// added again, then removed from the vault while wired: remove still cleans up
	put(t, filepath.Join(vdir, "dont.md"), "# dont\n")
	RefreshRules(e, s)
	os.Remove(filepath.Join(vdir, "dont.md"))
	if err := Remove(e, s, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := get(t, oc); strings.Contains(got, "/v/") || !strings.Contains(got, "mine.md") {
		t.Fatalf("opencode after remove:\n%s", got)
	}
}
