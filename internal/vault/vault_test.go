package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateKeepsExisting(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "me.md"), []byte("mine"), 0o644)
	if err := Create(d); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(d, "me.md")); string(b) != "mine" {
		t.Fatal("Create overwrote me.md")
	}
	if _, err := os.Stat(filepath.Join(d, "AGENTS.md")); err != nil {
		t.Fatal("no AGENTS.md")
	}
}

func TestRememberAndSearch(t *testing.T) {
	d := t.TempDir()
	Create(d)
	if _, err := Remember(d, "  ", "x", nil); err == nil {
		t.Fatal("empty memory should fail")
	}
	a, err := Remember(d, "lowercase commit titles, no co-author lines", "claude-code", []string{"Git", "commits!"})
	if err != nil {
		t.Fatal(err)
	}
	Remember(d, "use pnpm, not npm", "codex", []string{"node"})
	os.MkdirAll(filepath.Join(d, "skills", "deploy"), 0o755)
	os.WriteFile(filepath.Join(d, "skills", "deploy", "SKILL.md"), []byte("---\nname: deploy\ndescription: ship the app to fly.io\n---\n"), 0o644)

	hits, err := Search(d, "how should I write a commit", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Path != a.Path {
		t.Fatalf("commit memory not first: %+v", hits)
	}
	if hits[0].From != "claude-code" || len(hits[0].Tags) != 2 || hits[0].Tags[1] != "commits" {
		t.Fatalf("frontmatter lost: %+v", hits[0])
	}
	if hits, _ := Search(d, "deploy", 5); len(hits) != 1 || hits[0].Kind != "skill" {
		t.Fatalf("skill search: %+v", hits)
	}
	if hits, _ := Search(d, "", 5); len(hits) != 2 {
		t.Fatalf("empty query should list memories: %+v", hits)
	}
	if hits, _ := Search(d, "kubernetes", 5); len(hits) != 0 {
		t.Fatalf("expected nothing: %+v", hits)
	}
}
