package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesFilesOrder(t *testing.T) {
	d := t.TempDir()
	Create(d)
	for _, n := range []string{"remember.md", "Dont.md", "notes.txt", ".hidden.md"} {
		os.WriteFile(filepath.Join(d, n), []byte("x\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(d, "index"), 0o755)
	os.WriteFile(filepath.Join(d, "index", "files.md"), []byte("x\n"), 0o644)
	var got []string
	for _, p := range RulesFiles(d) {
		got = append(got, filepath.Base(p))
	}
	if strings.Join(got, " ") != "AGENTS.md me.md Dont.md remember.md" {
		t.Fatalf("rules files: %v", got)
	}
}

func TestAddNote(t *testing.T) {
	d := t.TempDir()
	Create(d)
	os.WriteFile(filepath.Join(d, "remember.md"), []byte("# remember"), 0o644) // no trailing newline
	os.MkdirAll(filepath.Join(d, "index"), 0o755)
	os.WriteFile(filepath.Join(d, "index", "files.md"), []byte("# files\n"), 0o644)

	p, err := AddNote(d, "Remember.md", "  use tabs\n in go ", "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "# remember\n- use tabs in go (") || !strings.HasSuffix(string(b), ", claude-code)\n") {
		t.Fatalf("remember.md: %q", b)
	}
	if p, err := AddNote(d, "files", "cs worksheet: /x.pdf", ""); err != nil || filepath.Base(filepath.Dir(p)) != "index" {
		t.Fatalf("index note: %v %s", err, p)
	}
	if _, err := AddNote(d, "mistakes", "x", ""); err == nil || !strings.Contains(err.Error(), "files, me, remember") {
		t.Fatalf("missing file should fail and list the real ones: %v", err)
	}
	if _, err := AddNote(d, "../me", "x", ""); err == nil {
		t.Fatal("path in name accepted")
	}
	if _, err := AddNote(d, "remember", "db password: hunter2xx", ""); err == nil {
		t.Fatal("secret accepted")
	}
}

func TestSearchIndex(t *testing.T) {
	d := t.TempDir()
	Create(d)
	os.MkdirAll(filepath.Join(d, "index"), 0o755)
	os.WriteFile(filepath.Join(d, "index", "files.md"), []byte("# files\n\n## school\n\n### chemistry (sch3u, chem)\n- module 4 worksheet: `/d/chem4.pdf`\n- lab report: `/d/lab.docx`\n  percentage yield\n\n### computer science (cs)\n- chapter 5 worksheet: `/d/cs5.pdf`\n"), 0o644)
	// a long memory that repeats one query word a lot shouldn't beat the right line
	Remember(d, strings.Repeat("worksheet ", 40), "x", nil)

	hits, _ := Search(d, "chem module 4 worksheet", 3)
	if len(hits) == 0 || hits[0].Kind != "index" || !strings.Contains(hits[0].Text, "chem4.pdf") {
		t.Fatalf("chem: %+v", hits)
	}
	if !strings.HasPrefix(hits[0].Text, "files / school / chemistry") {
		t.Fatalf("headings not kept: %q", hits[0].Text)
	}
	if hits, _ := Search(d, "cs worksheet", 1); len(hits) == 0 || !strings.Contains(hits[0].Text, "cs5.pdf") {
		t.Fatalf("cs: %+v", hits)
	}
	if hits, _ := Search(d, "percentage yield", 1); len(hits) == 0 || !strings.Contains(hits[0].Text, "lab.docx") {
		t.Fatalf("wrapped line lost: %+v", hits)
	}
	if hits, _ := Search(d, "", 10); len(hits) != 1 || hits[0].Kind != "memory" {
		t.Fatalf("empty query should list memories only: %+v", hits)
	}
}
