package wire

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// every tool in the table, alone and all together: apply, doctor is clean,
// apply again changes nothing, remove gives back the home byte for byte
func TestEveryTool(t *testing.T) {
	run := func(t *testing.T, names []string) {
		f := newFake(t)
		e := f.e
		vdir := filepath.Join(e.Home, "v")
		os.MkdirAll(vdir, 0o755)
		put(t, filepath.Join(vdir, "skills", "s1", "SKILL.md"), "---\ndescription: one\n---\n")
		before := snapshot(t, e.Home)

		s, _ := LoadState(e)
		if err := Apply(e, s, vdir, names, false, io.Discard); err != nil {
			t.Fatal(err)
		}
		if bad := Doctor(e, s); len(bad) != 0 {
			t.Fatalf("doctor: %v", bad)
		}
		mid := snapshot(t, e.Home)
		s2, _ := LoadState(e)
		if err := Apply(e, s2, vdir, names, false, io.Discard); err != nil {
			t.Fatal(err)
		}
		for k, v := range snapshot(t, e.Home) {
			if mid[k] != v {
				t.Fatalf("second apply changed %s", k)
			}
		}
		s3, _ := LoadState(e)
		if err := Remove(e, s3, nil, io.Discard); err != nil {
			t.Fatal(err)
		}
		after := snapshot(t, e.Home)
		for k, v := range before {
			if after[k] != v {
				t.Errorf("%s not restored", k)
			}
		}
		for k := range after {
			if _, ok := before[k]; !ok {
				t.Errorf("left behind: %s", k)
			}
		}
	}
	var all []string
	for _, tool := range All {
		all = append(all, tool.Name)
		name := tool.Name
		t.Run(name, func(t *testing.T) { run(t, []string{name}) })
	}
	t.Run("all", func(t *testing.T) { run(t, all) })

	// a tool with no skills folder once made links in the current folder
	if ents, _ := os.ReadDir("."); true {
		for _, d := range ents {
			if d.Name() == "s1" {
				t.Fatal("a skill link landed in the current folder")
			}
		}
	}
}

// same, but every file a tool touches already has the user's own stuff in it
func TestEveryToolKeepsUserContent(t *testing.T) {
	seed := func(p string) string {
		if filepath.Base(p) == "cordis.patch.yml" { // dsh: the file is a list
			return "# mine\n- keep: 1\n"
		}
		switch filepath.Ext(p) {
		case ".json", ".jsonc":
			return "{\n  // mine\n  \"keep\": 1\n}\n"
		case ".yaml", ".yml":
			return "# mine\nkeep: 1\n"
		case ".toml":
			return "# mine\nkeep = 1\n"
		case ".md", ".goosehints":
			return "# mine\nkeep this\n"
		}
		return "# mine\noption keep 1\n"
	}
	for _, tool := range All {
		name := tool.Name
		t.Run(name, func(t *testing.T) {
			// first find out which files it writes
			probe := newFake(t)
			ps, _ := LoadState(probe.e)
			if err := Apply(probe.e, ps, filepath.Join(probe.e.Home, "v"), []string{name}, false, io.Discard); err != nil {
				t.Fatal(err)
			}
			var rels []string
			for p := range ps.Files {
				rel, _ := filepath.Rel(probe.e.Home, p)
				rels = append(rels, rel)
			}

			f := newFake(t)
			e := f.e
			for _, rel := range rels {
				put(t, filepath.Join(e.Home, rel), seed(rel))
			}
			before := snapshot(t, e.Home)
			s, _ := LoadState(e)
			if err := Apply(e, s, filepath.Join(e.Home, "v"), []string{name}, false, io.Discard); err != nil {
				t.Fatal(err)
			}
			for _, rel := range rels {
				got := get(t, filepath.Join(e.Home, rel))
				if !strings.Contains(got, "mine") || !strings.Contains(got, "keep") {
					t.Fatalf("%s lost the user's content:\n%s", rel, got)
				}
			}
			if bad := Doctor(e, s); len(bad) != 0 {
				t.Fatalf("doctor: %v", bad)
			}
			// user edits something after, so remove has to cut just our part
			for _, rel := range rels {
				p := filepath.Join(e.Home, rel)
				if strings.HasSuffix(rel, ".json") || strings.HasSuffix(rel, ".jsonc") {
					put(t, p, strings.Replace(get(t, p), "\"keep\": 1", "\"keep\": 2", 1))
				} else {
					put(t, p, strings.Replace(get(t, p), "keep", "kept", 1))
				}
			}
			s2, _ := LoadState(e)
			if err := Remove(e, s2, nil, io.Discard); err != nil {
				t.Fatal(err)
			}
			for _, rel := range rels {
				got := get(t, filepath.Join(e.Home, rel))
				want := strings.Replace(before[rel], "keep", "kept", 1)
				if strings.HasSuffix(rel, ".json") || strings.HasSuffix(rel, ".jsonc") {
					// json comes back valid with our keys gone; formatting may differ a bit
					if strings.Contains(got, "ai-vault") || !strings.Contains(got, "\"keep\": 2") {
						t.Fatalf("%s after remove:\n%s", rel, got)
					}
					continue
				}
				if got != want {
					t.Fatalf("%s after remove:\nwant %q\ngot  %q", rel, want, got)
				}
			}
		})
	}
}
