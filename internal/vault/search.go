package vault

import (
	"sort"
	"strings"
	"unicode"
)

type Hit struct {
	Kind    string   `json:"kind"` // memory or skill
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
// me.md and AGENTS.md aren't searched since every tool already has them loaded
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

// each query word found in body = 1 per hit, in tags/name = 3. words that match
// nothing don't sink a doc, so "commit rules please" still finds commit stuff
func score(q []string, body, tags string) int {
	if len(q) == 0 {
		return 0
	}
	bw := words(body)
	tw := words(tags)
	n := 0
	for _, w := range q {
		for _, b := range bw {
			if b == w || (len(w) > 3 && strings.HasPrefix(b, w)) {
				n++
			}
		}
		for _, t := range tw {
			if t == w || (len(w) > 3 && strings.HasPrefix(t, w)) {
				n += 3
			}
		}
	}
	return n
}
