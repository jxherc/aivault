package vault

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Hit struct {
	Kind    string   `json:"kind"` // memory, skill or index
	Path    string   `json:"path"`
	Text    string   `json:"text"`
	Tags    []string `json:"tags,omitempty"`
	From    string   `json:"from,omitempty"`
	Created string   `json:"created,omitempty"`
	score   int
}

func words(s string) []string {
	f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var out []string
	for _, w := range f {
		if len([]rune(w)) > 1 {
			out = append(out, w)
		}
	}
	return out
}

// Search is plain word matching. empty query = newest memories.
// the rules files aren't searched since every tool already has them loaded.
// index/*.md are lookup lists (where files are, what names mean): each bullet
// is its own hit, so a big list still answers with the one line that matters
func Search(dir, query string, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 8
	}
	mems, err := Memories(dir)
	if err != nil {
		return nil, err
	}
	skills, err := Skills(dir)
	if err != nil {
		return nil, err
	}
	q := words(query)

	var hits []Hit
	for _, m := range mems {
		h := Hit{Kind: "memory", Path: m.Path, Text: m.Text, Tags: m.Tags, From: m.From, Created: m.Created}
		h.score = score(q, m.Text, strings.Join(m.Tags, " "))
		if len(q) == 0 || h.score > 0 {
			hits = append(hits, h)
		}
	}
	for _, s := range skills {
		h := Hit{Kind: "skill", Path: s.Dir, Text: s.Name + ": " + s.Desc}
		h.score = score(q, s.Desc, s.Name)
		if len(q) > 0 && h.score > 0 {
			hits = append(hits, h)
		}
	}
	if len(q) > 0 {
		for _, e := range indexEntries(dir) {
			h := Hit{Kind: "index", Path: e.path, Text: e.text}
			h.score = score(q, e.text, e.head)
			if h.score > 0 {
				hits = append(hits, h)
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].Created > hits[j].Created // rfc3339 sorts as text
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	// synced tool memories can be long. keep results small, the path has the rest
	for i := range hits {
		if r := []rune(hits[i].Text); len(r) > 1500 {
			hits[i].Text = string(r[:1500]) + " … (cut, full text in " + hits[i].Path + ")"
		}
	}
	return hits, nil
}

// how many of the query words a doc has decides the order: a short line with
// "chemistry", "module" and "worksheet" beats a long note that says
// "module" forty times. tags/name count extra, repeats only break ties
func score(q []string, body, tags string) int {
	if len(q) == 0 {
		return 0
	}
	bw := words(body)
	tw := words(tags)
	match := func(w, b string) bool { return b == w || (len(w) > 3 && strings.HasPrefix(b, w)) }
	n, extra := 0, 0
	for _, w := range q {
		found := 0
		for _, b := range bw {
			if match(w, b) {
				found++
			}
		}
		inTags := false
		for _, t := range tw {
			if match(w, t) {
				inTags = true
			}
		}
		if found > 0 || inTags {
			n += 10
		}
		if inTags {
			n += 5
		}
		extra += found
	}
	if n == 0 {
		return 0
	}
	if extra > 5 {
		extra = 5
	}
	return n + extra
}

type indexEntry struct{ path, head, text string }

// one entry per top-level bullet in index/*.md, with the headings above it,
// since "## chemistry" is often the word that finds "module 4 worksheet"
func indexEntries(dir string) []indexEntry {
	paths, _ := filepath.Glob(filepath.Join(dir, "index", "*.md"))
	var out []indexEntry
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		heads := map[int]string{}
		head := func() string {
			var hs []string
			for lvl := 1; lvl <= 6; lvl++ {
				if h, ok := heads[lvl]; ok {
					hs = append(hs, h)
				}
			}
			return strings.Join(hs, " / ")
		}
		var cur *indexEntry
		flush := func() {
			if cur != nil {
				out = append(out, *cur)
				cur = nil
			}
		}
		for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
			switch {
			case strings.HasPrefix(l, "#"):
				flush()
				lvl := len(l) - len(strings.TrimLeft(l, "#"))
				for k := range heads {
					if k >= lvl {
						delete(heads, k)
					}
				}
				heads[lvl] = strings.TrimSpace(strings.TrimLeft(l, "#"))
			case strings.HasPrefix(l, "- "):
				flush()
				h := head()
				text := strings.TrimPrefix(l, "- ")
				if h != "" {
					text = h + ": " + text
				}
				cur = &indexEntry{path: p, head: h, text: text}
			case cur != nil && strings.HasPrefix(l, " ") && strings.TrimSpace(l) != "":
				cur.text += "\n" + strings.TrimSpace(l)
			default:
				flush()
			}
		}
		flush()
	}
	return out
}
