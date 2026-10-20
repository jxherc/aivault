package wire

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/jxherc/ai-vault/internal/vault"
)

// opencode: no markers possible in json, so we patch two things in
// opencode.json(c): our paths in `instructions` and `mcp.ai-vault`.
// hujson keeps the user's comments

func opencodeCfg(e *Env) string {
	jc := filepath.Join(e.OpencodeDir, "opencode.jsonc")
	if exists(jc) {
		return jc
	}
	return filepath.Join(e.OpencodeDir, "opencode.json")
}

func opencodePaths(s *State) []string {
	var out []string
	for _, p := range vault.RulesFiles(s.Vault) {
		out = append(out, filepath.ToSlash(p))
	}
	return out
}

type ocView struct {
	Instructions []any          `json:"instructions"`
	MCP          map[string]any `json:"mcp"`
}

func parseOC(b []byte) (hujson.Value, ocView, error) {
	var view ocView
	if len(strings.TrimSpace(string(b))) == 0 {
		b = []byte(`{"$schema": "https://opencode.ai/config.json"}`)
	}
	v, err := hujson.Parse(b)
	if err != nil {
		return v, view, err
	}
	std := v.Clone()
	std.Standardize()
	if err := json.Unmarshal(std.Pack(), &view); err != nil {
		return v, view, err
	}
	return v, view, nil
}

// json pointer escaping
func ptr(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func patch(v *hujson.Value, ops []map[string]any) error {
	if len(ops) == 0 {
		return nil
	}
	b, err := json.Marshal(ops)
	if err != nil {
		return err
	}
	return v.Patch(b)
}

type ocServer struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}

// indent used by an object's members, from the whitespace before the first name
func indentOf(o *hujson.Object, def string) string {
	for _, m := range o.Members {
		ex := string(m.Name.BeforeExtra)
		if i := strings.LastIndex(ex, "\n"); i >= 0 {
			return ex[i+1:]
		}
	}
	return def
}

func member(o *hujson.Object, name string) *hujson.ObjectMember {
	for i := range o.Members {
		if l, ok := o.Members[i].Name.Value.(hujson.Literal); ok && l.String() == name {
			return &o.Members[i]
		}
	}
	return nil
}

// hujson puts new members right after the comma with no newline. give them
// the same indent as their neighbours
func fixNewline(m *hujson.ObjectMember, ind string) {
	if m != nil && len(m.Name.BeforeExtra) == 0 {
		m.Name.BeforeExtra = hujson.Extra("\n" + ind)
		if len(m.Value.BeforeExtra) == 0 {
			m.Value.BeforeExtra = hujson.Extra(" ") // `"key": value`, not `"key":value`
		}
	}
}

func pretty(x any, prefix, unit string) string {
	b, _ := json.MarshalIndent(x, prefix, unit)
	return string(b)
}

func ocAdd(e *Env, s *State, b []byte, paths []string) ([]byte, error) {
	v, view, err := parseOC(b)
	if err != nil {
		return nil, err
	}
	root, ok := v.Value.(*hujson.Object)
	if !ok {
		return nil, fmt.Errorf("top level isn't an object")
	}
	ind := indentOf(root, "  ")
	if ind == "" {
		ind = "  "
	}
	q := func(s string) string { j, _ := json.Marshal(s); return string(j) }

	// built by hand: json.Marshal would squash the pretty values
	var ops []string
	add := func(path, val string) {
		ops = append(ops, `{"op":"add","path":`+q(path)+`,"value":`+val+`}`)
	}
	if view.Instructions == nil {
		add("/instructions", pretty(paths, ind, ind))
	} else {
		have := map[string]bool{}
		for _, x := range view.Instructions {
			if s, ok := x.(string); ok {
				have[s] = true
			}
		}
		for _, p := range paths {
			if !have[p] {
				add("/instructions/-", q(p))
			}
		}
	}

	server := ocServer{Type: "local", Command: append([]string{e.Bin}, mcpArgs(s)...), Enabled: true}
	cur, _ := json.Marshal(view.MCP[mcpName])
	want, _ := json.Marshal(map[string]any{"type": server.Type, "command": server.Command, "enabled": true})
	switch {
	case view.MCP == nil:
		add("/mcp", pretty(map[string]ocServer{mcpName: server}, ind, ind))
	case string(cur) != string(want): // both sides go through map ordering, so this compares fine
		add("/mcp/"+ptr(mcpName), pretty(server, ind+ind, ind))
	}
	if len(ops) == 0 {
		return b, nil
	}
	if err := v.Patch([]byte("[" + strings.Join(ops, ",") + "]")); err != nil {
		return nil, err
	}

	root = v.Value.(*hujson.Object)
	if m := member(root, "instructions"); m != nil {
		fixNewline(m, ind)
		// appended array items: copy the first item's indent if it has one
		if a, ok := m.Value.Value.(*hujson.Array); ok && len(a.Elements) > 0 {
			first := string(a.Elements[0].BeforeExtra)
			if i := strings.LastIndex(first, "\n"); i >= 0 {
				for j := range a.Elements {
					if len(a.Elements[j].BeforeExtra) == 0 {
						a.Elements[j].BeforeExtra = hujson.Extra(first[i:])
					}
				}
			}
		}
	}
	if m := member(root, "mcp"); m != nil {
		fixNewline(m, ind)
		if o, ok := m.Value.Value.(*hujson.Object); ok {
			fixNewline(member(o, mcpName), indentOf(o, ind+ind))
		}
	}
	// no Format(): it would retab the whole file
	return v.Pack(), nil
}

func ocStrip(b []byte, paths []string) ([]byte, error) {
	v, view, err := parseOC(b)
	if err != nil {
		return nil, err
	}
	ours := map[string]bool{}
	for _, p := range paths {
		ours[p] = true
	}
	var idx []int
	for i, x := range view.Instructions {
		if s, ok := x.(string); ok && ours[s] {
			idx = append(idx, i)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(idx))) // back to front so indexes stay valid
	var ops []map[string]any
	for _, i := range idx {
		ops = append(ops, map[string]any{"op": "remove", "path": fmt.Sprintf("/instructions/%d", i)})
	}
	if _, ok := view.MCP[mcpName]; ok {
		ops = append(ops, map[string]any{"op": "remove", "path": "/mcp/" + ptr(mcpName)})
	}
	if err := patch(&v, ops); err != nil {
		return nil, err
	}
	// no Format(): it would retab the whole file
	return v.Pack(), nil
}

var opencode = &Tool{
	Name:  "opencode",
	Label: "OpenCode",
	Detect: func(e *Env) bool {
		return exists(e.OpencodeDir) || e.Has("opencode")
	},
	Skills: func(e *Env) string { return e.AgentsSkills },
	Apply: func(e *Env, s *State) error {
		p := opencodeCfg(e)
		b, err := read(p)
		if err != nil {
			return err
		}
		out, err := ocAdd(e, s, b, opencodePaths(s))
		if err != nil {
			return fmt.Errorf("can't edit %s: %w", p, err)
		}
		if string(out) == string(b) {
			return nil
		}
		return s.write(p, out)
	},
	Remove: func(e *Env, s *State) error {
		// state knows which file we edited, even if a .jsonc showed up later
		for p := range s.Files {
			if b := filepath.Base(p); filepath.Dir(p) == e.OpencodeDir && (b == "opencode.json" || b == "opencode.jsonc") {
				paths := opencodePaths(s)
				if err := s.undo(p, func(b []byte) ([]byte, error) { return ocStrip(b, paths) }); err != nil {
					return err
				}
			}
		}
		return nil
	},
	Check: func(e *Env, s *State) []string {
		p := opencodeCfg(e)
		b, _ := read(p)
		_, view, err := parseOC(b)
		if err != nil {
			return []string{p + " can't be parsed: " + err.Error()}
		}
		var bad []string
		have := map[string]bool{}
		for _, x := range view.Instructions {
			if s, ok := x.(string); ok {
				have[s] = true
			}
		}
		for _, want := range opencodePaths(s) {
			if !have[want] {
				bad = append(bad, p+" doesn't list "+want+" in instructions")
			}
		}
		if _, ok := view.MCP[mcpName]; !ok {
			bad = append(bad, p+" has no ai-vault mcp server")
		}
		return bad
	},
}
