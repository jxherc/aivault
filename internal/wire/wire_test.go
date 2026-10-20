package wire

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlockRoundTrip(t *testing.T) {
	cases := []string{"", "hello", "hello\n", "a\r\nb\r\n", "x\n\n"}
	for _, orig := range cases {
		out, sep := mdMarks.set([]byte(orig), "@/v/AGENTS.md\n@/v/me.md")
		if !mdMarks.has(out) {
			t.Fatalf("%q: no block after set", orig)
		}
		if got := mdMarks.body(out); got != "@/v/AGENTS.md\n@/v/me.md" {
			t.Fatalf("%q: body = %q", orig, got)
		}
		// setting again changes nothing
		again, _ := mdMarks.set(out, "@/v/AGENTS.md\n@/v/me.md")
		if string(again) != string(out) {
			t.Fatalf("%q: second set changed the file", orig)
		}
		if back := mdMarks.strip(out, sep); string(back) != orig {
			t.Fatalf("%q: strip gave %q", orig, back)
		}
	}
	// crlf files keep crlf
	out, _ := mdMarks.set([]byte("a\r\n"), "x")
	if strings.Contains(strings.ReplaceAll(string(out), "\r\n", ""), "\n") {
		t.Fatalf("bare \\n in a crlf file: %q", out)
	}
}

type fake struct {
	e     *Env
	calls []string
	mcp   bool
}

func newFake(t *testing.T) *fake {
	home := t.TempDir()
	f := &fake{}
	f.e = &Env{
		Home:         home,
		ClaudeDir:    filepath.Join(home, ".claude"),
		CodexDir:     filepath.Join(home, ".codex"),
		OpencodeDir:  filepath.Join(home, ".config", "opencode"),
		AgentsSkills: filepath.Join(home, ".agents", "skills"),
		StateDir:     filepath.Join(home, ".aivault"),
		Bin:          "/usr/local/bin/aivault",
		Has:          func(string) bool { return true },
		Run: func(name string, args ...string) error {
			f.calls = append(f.calls, name+" "+strings.Join(args, " "))
			switch args[1] {
			case "add":
				f.mcp = true
			case "remove":
				f.mcp = false
			case "get":
				if !f.mcp {
					return os.ErrNotExist
				}
			}
			return nil
		},
	}
	return f
}

func put(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyRemoveRestoresFiles(t *testing.T) {
	f := newFake(t)
	e := f.e
	claudeMD := filepath.Join(e.ClaudeDir, "CLAUDE.md")
	codexMD := filepath.Join(e.CodexDir, "AGENTS.md")
	codexCfg := filepath.Join(e.CodexDir, "config.toml")
	ocCfg := filepath.Join(e.OpencodeDir, "opencode.jsonc")

	origClaude := "# mine\nkeep this\n"
	origCfg := "model = \"x\"\n\n[mcp_servers.other]\ncommand = \"y\"\n"
	origOC := "{\n  // my comment\n  \"model\": \"a/b\",\n  \"instructions\": [\"x.md\"]\n}\n"
	put(t, claudeMD, origClaude)
	put(t, codexCfg, origCfg)
	put(t, ocCfg, origOC)

	vdir := filepath.Join(e.Home, "my vault") // space on purpose
	put(t, filepath.Join(vdir, "skills", "deploy", "SKILL.md"), "---\nname: deploy\ndescription: ship it\n---\nsteps\n")

	s, _ := LoadState(e)
	if err := Apply(e, s, vdir, []string{"claude", "codex", "opencode"}, false, io.Discard); err != nil {
		t.Fatal(err)
	}

	if c := get(t, claudeMD); !strings.HasPrefix(c, origClaude) || !strings.Contains(c, claudeImport(filepath.Join(vdir, "AGENTS.md"))) || !strings.Contains(c, `my\ vault`) {
		t.Fatalf("claude md:\n%s", c)
	}
	if c := get(t, codexMD); !strings.Contains(c, "vault_search") {
		t.Fatalf("codex md has no rules copy:\n%s", c)
	}
	if c := get(t, codexCfg); !strings.Contains(c, "[mcp_servers.ai-vault]") || !strings.HasPrefix(c, origCfg) {
		t.Fatalf("codex cfg:\n%s", c)
	}
	oc := get(t, ocCfg)
	if !strings.Contains(oc, "// my comment") || !strings.Contains(oc, `"x.md"`) || !strings.Contains(oc, "AGENTS.md") || !strings.Contains(oc, `"ai-vault"`) {
		t.Fatalf("opencode cfg:\n%s", oc)
	}
	for _, p := range []string{filepath.Join(e.ClaudeDir, "skills", "deploy"), filepath.Join(e.AgentsSkills, "deploy")} {
		if _, err := os.Stat(filepath.Join(p, "SKILL.md")); err != nil {
			t.Fatalf("skill not linked at %s", p)
		}
	}
	if !f.mcp {
		t.Fatal("claude mcp add never ran")
	}
	if bad := Doctor(e, s); len(bad) != 0 {
		t.Fatalf("doctor after apply: %v", bad)
	}

	// running again changes no file
	before := get(t, claudeMD) + get(t, codexMD) + get(t, codexCfg) + get(t, ocCfg)
	s2, _ := LoadState(e)
	if err := Apply(e, s2, vdir, s2.Tools, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	if after := get(t, claudeMD) + get(t, codexMD) + get(t, codexCfg) + get(t, ocCfg); after != before {
		t.Fatal("second apply changed files")
	}

	s3, _ := LoadState(e)
	if err := Remove(e, s3, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if get(t, claudeMD) != origClaude || get(t, codexCfg) != origCfg || get(t, ocCfg) != origOC {
		t.Fatal("remove didn't restore originals byte for byte")
	}
	if _, err := os.Stat(codexMD); !os.IsNotExist(err) {
		t.Fatal("codex AGENTS.md was created by us and should be gone")
	}
	if _, err := os.Lstat(filepath.Join(e.AgentsSkills, "deploy")); !os.IsNotExist(err) {
		t.Fatal("skill link left behind")
	}
	if f.mcp {
		t.Fatal("claude mcp still registered")
	}
	if _, err := os.Stat(filepath.Join(e.StateDir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("state file left behind")
	}
	if _, err := os.Stat(filepath.Join(vdir, "AGENTS.md")); err != nil {
		t.Fatal("remove must never touch the vault")
	}
}

func TestRemoveAfterUserEdits(t *testing.T) {
	f := newFake(t)
	e := f.e
	claudeMD := filepath.Join(e.ClaudeDir, "CLAUDE.md")
	ocCfg := filepath.Join(e.OpencodeDir, "opencode.json")
	put(t, claudeMD, "top\n")
	vdir := filepath.Join(e.Home, "v")

	s, _ := LoadState(e)
	if err := Apply(e, s, vdir, []string{"claude", "opencode"}, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	// user adds lines after our block, and opencode gets another setting
	put(t, claudeMD, get(t, claudeMD)+"added later\n")
	put(t, ocCfg, strings.Replace(get(t, ocCfg), "{", "{\n  \"theme\": \"dark\",", 1))

	s2, _ := LoadState(e)
	if err := Remove(e, s2, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if c := get(t, claudeMD); c != "top\nadded later\n" {
		t.Fatalf("claude md after remove = %q", c)
	}
	oc := get(t, ocCfg)
	if strings.Contains(oc, "ai-vault") || strings.Contains(oc, "AGENTS.md") || !strings.Contains(oc, `"theme"`) {
		t.Fatalf("opencode after remove:\n%s", oc)
	}
}

func TestCodexRefusesDuplicateTable(t *testing.T) {
	f := newFake(t)
	put(t, filepath.Join(f.e.CodexDir, "config.toml"), "[mcp_servers.ai-vault]\ncommand = \"old\"\n")
	s, _ := LoadState(f.e)
	if err := Apply(f.e, s, filepath.Join(f.e.Home, "v"), []string{"codex"}, false, io.Discard); err == nil {
		t.Fatal("expected an error for a duplicate mcp table")
	}
}

func TestDoctorCatchesProblems(t *testing.T) {
	f := newFake(t)
	e := f.e
	vdir := filepath.Join(e.Home, "v")
	put(t, filepath.Join(vdir, "skills", "a", "SKILL.md"), "---\ndescription: a\n---\n")
	s, _ := LoadState(e)
	if err := Apply(e, s, vdir, []string{"claude", "codex"}, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(e.CodexDir, "AGENTS.override.md"), "other rules\n")
	put(t, filepath.Join(vdir, "me.md"), "changed\n")
	os.Remove(filepath.Join(e.ClaudeDir, "CLAUDE.md"))
	os.RemoveAll(filepath.Join(vdir, "skills", "a"))

	got := strings.Join(Doctor(e, s), "\n")
	for _, want := range []string{"override", "old copy", "CLAUDE.md has no ai-vault block", "skill link is broken"} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor missed %q in:\n%s", want, got)
		}
	}
}

func TestCodexMax(t *testing.T) {
	f := newFake(t)
	lim := codexMax(func(e *Env) string { return e.CodexDir })
	if lim(f.e) != codexLimit {
		t.Fatal("default limit wrong")
	}
	put(t, filepath.Join(f.e.CodexDir, "config.toml"), "# raised\nproject_doc_max_bytes = 65536\nmodel = \"x\"\n\n[tools]\nproject_doc_max_bytes = 1\n")
	if lim(f.e) != 65536 {
		t.Fatalf("got %d", lim(f.e))
	}
	put(t, filepath.Join(f.e.CodexDir, "config.toml"), "[profiles.x]\nproject_doc_max_bytes = 99\n")
	if lim(f.e) != codexLimit {
		t.Fatal("a key inside a table must not count")
	}
}
