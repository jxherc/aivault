package wire

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitMem(t *testing.T) {
	got := splitMem(memSrc{split: splitHeading, prefix: "# Task Group"}, "intro\n# Task Group: a\nx\n# Task Group: b\ny\n")
	if len(got) != 2 || got[0][0] != "# Task Group: a" || !strings.Contains(got[1][1], "y") {
		t.Fatalf("heading: %q", got)
	}
	got = splitMem(memSrc{split: splitSep, prefix: "§"}, "one\n§\ntwo\nmore\n§\n")
	if len(got) != 2 || got[1][1] != "two\nmore" {
		t.Fatalf("sep: %q", got)
	}
	got = splitMem(memSrc{split: splitBullets}, "# lessons\n\nintro\n---\n- a: one\n  wrapped\n- b: two\n")
	if len(got) != 2 || got[0][1] != "a: one\n  wrapped" {
		t.Fatalf("bullets: %q", got)
	}
	got = splitMem(memSrc{split: splitFile}, "---\nname: x\n---\n\nbody here\n")
	if len(got) != 1 || got[0][1] != "body here" {
		t.Fatalf("file: %q", got)
	}
}

func TestLooksSecret(t *testing.T) {
	for _, s := range []string{"AUTH password v9001989", "api_key: abcdef123456", "token = 0123456789ab", "sk-proj-abc123def456ghi789", "ghp_abcdefghijklmnopqrstuvwxyz12"} {
		if !looksSecret(s) {
			t.Errorf("missed: %s", s)
		}
	}
	for _, s := range []string{"never save secrets", "the token budget is small", "reset the password flow later", "use api keys from env", "logs/2026-09-25/task-clone-and-initialize-delta"} {
		if looksSecret(s) {
			t.Errorf("false alarm: %s", s)
		}
	}
}

func TestSyncNative(t *testing.T) {
	f := newFake(t)
	e := f.e
	cm := filepath.Join(e.ClaudeDir, "projects", "-Users-me-Projects-alles", "memory")
	put(t, filepath.Join(cm, "MEMORY.md"), "- [a](a.md)\n") // index, skipped
	put(t, filepath.Join(cm, "a.md"), "---\nname: a\n---\nalles runs on port 8002\n")
	put(t, filepath.Join(cm, "b.md"), "---\nname: b\n---\nlogin with password hunter2xx\n")
	put(t, filepath.Join(e.CodexDir, "memories", "MEMORY.md"), "# Task Group: deploy\nship from dev\n# Task Group: tests\nrun go test\n")
	put(t, filepath.Join(e.Home, ".qwen", "memories", "user", "tabs.md"), "---\ncategory: style\n---\nuse tabs in go\n")

	vdir := filepath.Join(e.Home, "v")
	s, _ := LoadState(e)
	var out bytes.Buffer
	if err := Apply(e, s, vdir, []string{"claude", "codex", "qwen"}, true, &out); err != nil {
		t.Fatal(err)
	}
	mem := func() []string {
		ents, _ := os.ReadDir(filepath.Join(vdir, "memory"))
		var names []string
		for _, d := range ents {
			names = append(names, d.Name())
		}
		return names
	}
	if n := len(mem()); n != 4 {
		t.Fatalf("want 4 synced memories (a, 2 codex groups, qwen), got %d\n%s", n, out.String())
	}
	if !strings.Contains(out.String(), "b.md") || !strings.Contains(out.String(), "password or key") {
		t.Fatalf("secret file not reported:\n%s", out.String())
	}
	all := ""
	for _, n := range mem() {
		all += get(t, filepath.Join(vdir, "memory", n))
	}
	if strings.Contains(all, "hunter2") {
		t.Fatal("a secret reached the vault")
	}
	if !strings.Contains(all, "from: claude") || !strings.Contains(all, "tags: [claude, native, alles]") {
		t.Fatalf("claude memory metadata:\n%s", all)
	}

	// again: nothing new. then a tool rewrites one memory: updated in place
	out.Reset()
	s2, _ := LoadState(e)
	Apply(e, s2, vdir, s2.Tools, true, &out)
	if len(mem()) != 4 || strings.Contains(out.String(), "new,") {
		t.Fatalf("second sync changed things:\n%s", out.String())
	}
	put(t, filepath.Join(e.CodexDir, "memories", "MEMORY.md"), "# Task Group: deploy\nship from main now\n# Task Group: tests\nrun go test\n")
	out.Reset()
	s3, _ := LoadState(e)
	Apply(e, s3, vdir, s3.Tools, true, &out)
	if len(mem()) != 4 || !strings.Contains(out.String(), "0 new, 1 updated") {
		t.Fatalf("rewrite should update in place:\n%s", out.String())
	}

	// the tools keep their own memory, and remove never deletes vault memories
	if _, err := os.Stat(filepath.Join(cm, "a.md")); err != nil {
		t.Fatal("sync must copy, not move")
	}
	s4, _ := LoadState(e)
	Remove(e, s4, nil, io.Discard)
	if len(mem()) != 4 {
		t.Fatal("remove deleted vault memories")
	}
}
