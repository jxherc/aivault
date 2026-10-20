package wire

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// yaml configs (hermes, goose...): our entry goes under a top-level map key,
// between marker comments, as plain text. no yaml library, so the rest of
// the file stays byte for byte what the user wrote

const (
	yamlStart = "# ai-vault:start"
	yamlEnd   = "# ai-vault:end"
)

// ordered so the output is stable
type ykv struct {
	k string
	v any // string, bool, int, []string, []ykv
}

// written as is, like {} for an empty map
type yamlRaw string

func yamlScalar(v any) string {
	switch x := v.(type) {
	case yamlRaw:
		return string(x)
	case string:
		b, _ := json.Marshal(x) // a json string is a valid yaml double-quoted string
		return string(b)
	case []string:
		b, _ := json.Marshal(x) // and a json array is a valid yaml flow list
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}

func yamlLines(kvs []ykv, ind, unit string) []string {
	var out []string
	for _, kv := range kvs {
		if sub, ok := kv.v.([]ykv); ok {
			out = append(out, ind+kv.k+":")
			out = append(out, yamlLines(sub, ind+unit, unit)...)
			continue
		}
		out = append(out, ind+kv.k+": "+yamlScalar(kv.v))
	}
	return out
}

func indentOfLine(l string) string {
	return l[:len(l)-len(strings.TrimLeft(l, " \t"))]
}

// lines of the marked region, -1 if none
func yamlRegion(lines []string) (int, int) {
	s := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == yamlStart {
			s = i
		} else if t == yamlEnd && s >= 0 {
			return s, i
		}
	}
	return -1, -1
}

func splitLines(b []byte) ([]string, string) {
	eol := nl(b)
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil, eol
	}
	return strings.Split(s, "\n"), eol
}

func joinLines(lines []string, eol string) []byte {
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, eol) + eol)
}

// a map entry `name: {...}`, as lines for yamlSet
func yamlEntry(name string, entry []ykv) []string {
	return append([]string{name + ":"}, yamlLines(entry, "  ", "  ")...)
}

// list items, as lines for yamlSet
func yamlItems(items []string) []string {
	var out []string
	for _, it := range items {
		out = append(out, "- "+yamlScalar(it))
	}
	return out
}

// yamlSet puts child lines under top-level `key`. orig is the user's line we
// had to rewrite (`key: {}` or `key: []`), so strip can put it back
//
// key "" means the file itself is a list (dsh's cordis.patch.yml): child
// lines get appended as top-level items
func yamlSet(b []byte, key string, child []string) ([]byte, string, error) {
	lines, eol := splitLines(b)
	if key == "" {
		return yamlAppendTop(lines, eol, child)
	}
	block := func(ind string) []string {
		l := []string{ind + yamlStart}
		for _, c := range child {
			l = append(l, ind+c)
		}
		return append(l, ind+yamlEnd)
	}

	if s, e := yamlRegion(lines); s >= 0 {
		ind := indentOfLine(lines[s])
		nb := block(ind)
		if ind == "" && s+1 < e && strings.TrimSpace(lines[s+1]) == key+":" {
			// we made the key too, so it stays inside the markers
			inner := block("  ")
			nb = append([]string{yamlStart, key + ":"}, inner[1:len(inner)-1]...)
			nb = append(nb, yamlEnd)
		}
		out := append(append(append([]string{}, lines[:s]...), nb...), lines[e+1:]...)
		return joinLines(out, eol), "", nil
	}

	blockKey := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:\s*(#.*)?$`)
	emptyFlow := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:\s*(\{\s*\}|\[\s*\])\s*(#.*)?$`)
	anyKey := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:`)
	for i, l := range lines {
		switch {
		case blockKey.MatchString(l):
			ind := "  "
			for _, n := range lines[i+1:] {
				t := strings.TrimSpace(n)
				if t == "" || strings.HasPrefix(t, "#") {
					continue
				}
				if x := indentOfLine(n); x != "" {
					ind = x
				} else if strings.HasPrefix(t, "- ") {
					ind = "" // list items flush with the key
				}
				break
			}
			out := append(append(append([]string{}, lines[:i+1]...), block(ind)...), lines[i+1:]...)
			return joinLines(out, eol), "", nil
		case emptyFlow.MatchString(l):
			out := append(append(append([]string{}, lines[:i]...), key+":"), block("  ")...)
			out = append(out, lines[i+1:]...)
			return joinLines(out, eol), l, nil
		case anyKey.MatchString(l):
			return nil, "", fmt.Errorf("`%s` is written in a style aivault can't edit safely. add the ai-vault server by hand", key)
		}
	}

	// key isn't there: add it, inside the markers
	ls := append([]string{}, lines...)
	if len(ls) > 0 && strings.TrimSpace(ls[len(ls)-1]) != "" {
		ls = append(ls, "")
	}
	inner := block("  ")
	ls = append(ls, yamlStart, key+":")
	ls = append(ls, inner[1:len(inner)-1]...)
	ls = append(ls, yamlEnd)
	return joinLines(ls, eol), "", nil
}

func yamlAppendTop(lines []string, eol string, child []string) ([]byte, string, error) {
	blk := append(append([]string{yamlStart}, child...), yamlEnd)
	if s, e := yamlRegion(lines); s >= 0 {
		out := append(append(append([]string{}, lines[:s]...), blk...), lines[e+1:]...)
		return joinLines(out, eol), "", nil
	}
	var content []string
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "#") {
			content = append(content, t)
		}
	}
	switch {
	case len(content) == 1 && strings.HasPrefix(content[0], "[") && strings.TrimSpace(strings.Trim(content[0], "[]")) == "":
		// `[]`: swap that line for our items, strip puts it back
		var out []string
		orig := ""
		for _, l := range lines {
			if strings.TrimSpace(l) == content[0] {
				orig = l
				out = append(out, blk...)
				continue
			}
			out = append(out, l)
		}
		return joinLines(out, eol), orig, nil
	case len(content) > 0 && !strings.HasPrefix(content[0], "- ") && content[0] != "-":
		return nil, "", fmt.Errorf("the file isn't a yaml list. add the ai-vault entry by hand")
	}
	return joinLines(append(append([]string{}, lines...), blk...), eol), "", nil
}

func yamlStrip(b []byte, key, orig string) []byte {
	lines, eol := splitLines(b)
	s, e := yamlRegion(lines)
	if s < 0 {
		return b
	}
	out := append(append([]string{}, lines[:s]...), lines[e+1:]...)
	if key == "" {
		if orig != "" {
			out = append(append(append([]string{}, lines[:s]...), orig), lines[e+1:]...)
		}
		return joinLines(out, eol)
	}
	// we added a blank line before a block we appended at the end
	if e+1 == len(lines) && s > 0 && strings.TrimSpace(out[len(out)-1]) == "" && indentOfLine(lines[s]) == "" {
		out = out[:len(out)-1]
	}
	if orig != "" && s > 0 && strings.TrimSpace(out[s-1]) == key+":" {
		out[s-1] = orig
	}
	return joinLines(out, eol)
}

func yamlHas(b []byte) bool {
	lines, _ := splitLines(b)
	s, _ := yamlRegion(lines)
	return s >= 0
}

// the region as text, for doctor to compare
func yamlBody(b []byte) string {
	lines, _ := splitLines(b)
	s, e := yamlRegion(lines)
	if s < 0 {
		return ""
	}
	var out []string
	for _, l := range lines[s+1 : e] {
		out = append(out, strings.TrimSpace(l))
	}
	return strings.Join(out, "\n")
}
