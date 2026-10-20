package wire

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// a home that looks like a real one: rules in two tools (same text), a
// relative @import, skills that are real folders, links between tools, a link
// to somewhere else, and a name clash
func importHome(t *testing.T) (*fake, string) {
	f := newFake(t)
	e := f.e
	rules := "# my rules\n\nuse tabs.\n\n@extra.md\n\nping @alice about it\n\n```\n@not-an-import.md\n```\n"
	put(t, filepath.Join(e.ClaudeDir, "CLAUDE.md"), rules)
	put(t, filepath.Join(e.ClaudeDir, "extra.md"), "extra stuff\n")
	put(t, filepath.Join(e.ClaudeDir, "not-an-import.md"), "x\n")
	put(t, filepath.Join(e.CodexDir, "AGENTS.md"), rules)
	put(t, filepath.Join(e.OpencodeDir, "AGENTS.md"), "opencode only rules\n")

	put(t, filepath.Join(e.AgentsSkills, "shared", "SKILL.md"), "---\ndescription: shared one\n---\n")
	put(t, filepath.Join(e.ClaudeDir, "skills", "solo", "SKILL.md"), "---\ndescription: claude only\n---\n")
	put(t, filepath.Join(e.ClaudeDir, "skills", "solo", "notes.txt"), "keep me\n")
	put(t, filepath.Join(e.OpencodeDir, "skills", "solo", "SKILL.md"), "---\ndescription: a different solo\n---\n")
	put(t, filepath.Join(e.OpencodeDir, "skills", "shared", "SKILL.md"), "---\ndescription: shared one\n---\n") // identical copy
	put(t, filepath.Join(e.ClaudeDir, "skills", "INDEX.md"), "not a skill\n")
	put(t, filepath.Join(e.Home, "elsewhere", "ext", "SKILL.md"), "---\ndescription: managed elsewhere\n---\n")
	os.Symlink(filepath.Join("..", "..", ".agents", "skills", "shared"), filepath.Join(e.ClaudeDir, "skills", "shared"))
	os.Symlink(filepath.Join(e.Home, "elsewhere", "ext"), filepath.Join(e.ClaudeDir, "skills", "ext"))
	return f, rules
}

// every file under dir, path -> content (links shown as "-> target")
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if strings.HasPrefix(rel, ".aivault") || strings.HasPrefix(rel, "v") {
			return nil
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			l, _ := os.Readlink(p)
			m[rel] = "-> " + l
			return nil
		}
		if !fi.IsDir() {
			b, _ := os.ReadFile(p)
			m[rel] = string(b)
		}
		return nil
	})
	return m
}

func TestScan(t *testing.T) {
	f, _ := importHome(t)
	s, _ := LoadState(f.e)
	found, err := Scan(f.e, s, filepath.Join(f.e.Home, "v"), []string{"claude", "codex", "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Rules) != 2 || len(found.Rules[0].Paths) != 2 {
		t.Fatalf("want claude+codex grouped and opencode alone, got %+v", found.Rules)
	}
	var names []string
	for _, sk := range found.Skills {
		names = append(names, sk.Name)
	}
	if len(names) != 3 || found.unique() != 2 {
		t.Fatalf("want solo + shared + shared's identical copy, got %v", names)
	}
	if found.Linked != 1 {
		t.Fatalf("the link to elsewhere/ext should be left alone, Linked = %d", found.Linked)
	}
	if len(found.Skipped) != 1 || !strings.Contains(found.Skipped[0], "different skill") {
		t.Fatalf("name clash not reported: %v", found.Skipped)
	}
}

func TestImportAndRemove(t *testing.T) {
	f, _ := importHome(t)
	e := f.e
	before := snapshot(t, e.Home)
	vdir := filepath.Join(e.Home, "v")

	s, _ := LoadState(e)
	var out bytes.Buffer
	if err := Apply(e, s, vdir, []string{"claude", "codex", "opencode"}, true, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "isn't ours") {
		t.Fatalf("user's own links shouldn't warn:\n%s", out.String())
	}

	me := get(t, filepath.Join(vdir, "me.md"))
	if strings.Count(me, "use tabs.") != 1 || !strings.Contains(me, "## from Claude Code, Codex") || !strings.Contains(me, "opencode only rules") {
		t.Fatalf("me.md:\n%s", me)
	}
	if !strings.Contains(me, "@"+filepath.ToSlash(filepath.Join(e.ClaudeDir, "extra.md"))) {
		t.Fatalf("relative import not made absolute:\n%s", me)
	}
	if !strings.Contains(me, "@alice") || !strings.Contains(me, "\n@not-an-import.md\n") {
		t.Fatalf("mention or fenced text got rewritten:\n%s", me)
	}
	if strings.Contains(me, "write your preferences here") {
		t.Fatal("starter text should go once real rules come in")
	}

	if c := get(t, filepath.Join(e.ClaudeDir, "CLAUDE.md")); strings.Contains(c, "use tabs") || !mdMarks.has([]byte(c)) {
		t.Fatalf("CLAUDE.md should hold only our block now:\n%s", c)
	}
	if c := get(t, filepath.Join(e.OpencodeDir, "AGENTS.md")); strings.Contains(c, "opencode only") {
		t.Fatalf("opencode AGENTS.md still has its text:\n%s", c)
	}

	for _, sk := range []string{"shared", "solo"} {
		if _, err := os.Stat(filepath.Join(vdir, "skills", sk, "SKILL.md")); err != nil {
			t.Fatalf("%s not in vault", sk)
		}
	}
	if get(t, filepath.Join(vdir, "skills", "solo", "notes.txt")) != "keep me\n" {
		t.Fatal("skill files beyond SKILL.md got lost")
	}
	// the user's own link still works and now ends at the vault
	real, _ := filepath.EvalSymlinks(filepath.Join(e.ClaudeDir, "skills", "shared"))
	want, _ := filepath.EvalSymlinks(filepath.Join(vdir, "skills", "shared"))
	if real != want {
		t.Fatalf("claude's shared link ends at %s, want %s", real, want)
	}
	if _, err := os.Stat(filepath.Join(vdir, "skills", "ext")); err == nil {
		t.Fatal("a skill linked from elsewhere was imported")
	}
	if bad := Doctor(e, s); len(bad) != 0 {
		t.Fatalf("doctor after import: %v", bad)
	}

	// second run finds nothing new
	s2, _ := LoadState(e)
	if again, _ := Scan(e, s2, vdir, s2.Tools); !again.Empty() {
		t.Fatalf("second scan found %+v", again)
	}

	s3, _ := LoadState(e)
	if err := Remove(e, s3, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, e.Home)
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s not restored:\nwant %q\ngot  %q", k, v, after[k])
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			t.Errorf("left behind: %s", k)
		}
	}
	if _, err := os.Stat(filepath.Join(vdir, "skills", "solo")); err != nil {
		t.Fatal("remove must not delete the vault copy")
	}
}

func TestRemoveAfterEditPutsMovedTextBack(t *testing.T) {
	f, _ := importHome(t)
	e := f.e
	vdir := filepath.Join(e.Home, "v")
	s, _ := LoadState(e)
	if err := Apply(e, s, vdir, []string{"claude"}, true, io.Discard); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(e.ClaudeDir, "CLAUDE.md")
	put(t, p, get(t, p)+"new line from the user\n")

	s2, _ := LoadState(e)
	if err := Remove(e, s2, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	c := get(t, p)
	if !strings.HasPrefix(c, "# my rules") || !strings.Contains(c, "new line from the user") || mdMarks.has([]byte(c)) {
		t.Fatalf("CLAUDE.md after remove:\n%s", c)
	}
}

func TestLinkedRulesFile(t *testing.T) {
	f := newFake(t)
	e := f.e
	put(t, filepath.Join(e.ClaudeDir, "CLAUDE.md"), "shared rules\n")
	os.MkdirAll(e.CodexDir, 0o755)
	os.Symlink(filepath.Join(e.ClaudeDir, "CLAUDE.md"), filepath.Join(e.CodexDir, "AGENTS.md"))
	vdir := filepath.Join(e.Home, "v")

	s, _ := LoadState(e)
	if err := Apply(e, s, vdir, []string{"claude", "codex"}, true, io.Discard); err != nil {
		t.Fatal(err)
	}
	if me := get(t, filepath.Join(vdir, "me.md")); strings.Count(me, "shared rules") != 1 {
		t.Fatalf("me.md:\n%s", me)
	}
	s2, _ := LoadState(e)
	if err := Remove(e, s2, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if l, err := os.Readlink(filepath.Join(e.CodexDir, "AGENTS.md")); err != nil || l != filepath.Join(e.ClaudeDir, "CLAUDE.md") {
		t.Fatalf("codex AGENTS.md should be a link again: %q %v", l, err)
	}
	if get(t, filepath.Join(e.ClaudeDir, "CLAUDE.md")) != "shared rules\n" {
		t.Fatal("CLAUDE.md not restored")
	}
}
