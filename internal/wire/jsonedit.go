package wire

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

// editing other people's json(c) configs. we only ever own two kinds of
// thing: one object member at a path, or a few strings inside an array.
// hujson keeps comments and the user's formatting

type jsonOp struct {
	Path []string // object keys from the root
	Val  any      // set this member...
	List []string // ...or make sure these strings are in the array here
	// only set Val when the key isn't there. undo removes it only if we added it
	IfMissing bool
}

func (op jsonOp) key() string { return strings.Join(op.Path, ".") }

func parseJSON(b []byte) (hujson.Value, map[string]any, error) {
	if len(strings.TrimSpace(string(b))) == 0 {
		b = []byte("{\n}\n") // so a new file closes on its own line
	}
	v, err := hujson.Parse(b)
	if err != nil {
		return v, nil, err
	}
	std := v.Clone()
	std.Standardize()
	var view map[string]any
	if err := json.Unmarshal(std.Pack(), &view); err != nil {
		return v, nil, fmt.Errorf("top level isn't an object")
	}
	if view == nil {
		view = map[string]any{}
	}
	return v, view, nil
}

// json pointer escaping
func ptr(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func pointer(path []string) string {
	var sb strings.Builder
	for _, p := range path {
		sb.WriteString("/" + ptr(p))
	}
	return sb.String()
}

// walk returns the value at path and how many keys of it exist
func walk(view map[string]any, path []string) (any, int) {
	var cur any = view
	for i, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, i
		}
		next, ok := m[k]
		if !ok {
			return nil, i
		}
		cur = next
	}
	return cur, len(path)
}

// same json, ignoring key order
func sameJSON(a, b any) bool {
	norm := func(x any) string {
		j, _ := json.Marshal(x)
		var y any
		json.Unmarshal(j, &y)
		k, _ := json.Marshal(y)
		return string(k)
	}
	return norm(a) == norm(b)
}

func pretty(x any, prefix, unit string) string {
	b, _ := json.MarshalIndent(x, prefix, unit)
	return string(b)
}

func patchOne(v *hujson.Value, op, path, val string) error {
	q, _ := json.Marshal(path)
	p := `[{"op":"` + op + `","path":` + string(q)
	if val != "" {
		p += `,"value":` + val // by hand: json.Marshal would squash the pretty value
	}
	return v.Patch([]byte(p + "}]"))
}

// added lists the IfMissing ops that went in this time
func jsonApply(b []byte, ops []jsonOp) (out []byte, added []string, err error) {
	v, view, err := parseJSON(b)
	if err != nil {
		return nil, nil, err
	}
	changed := false
	for _, op := range ops {
		root := v.Value.(*hujson.Object)
		unit := indentOf(root, "  ")
		if unit == "" {
			unit = "  "
		}
		cur, have := walk(view, op.Path)

		var val any = op.Val
		if op.List != nil {
			if arr, ok := cur.([]any); ok && have == len(op.Path) {
				seen := map[string]bool{}
				for _, x := range arr {
					if s, ok := x.(string); ok {
						seen[s] = true
					}
				}
				for _, s := range op.List {
					if !seen[s] {
						q, _ := json.Marshal(s)
						if err := patchOne(&v, "add", pointer(op.Path)+"/-", string(q)); err != nil {
							return nil, nil, err
						}
						changed = true
					}
				}
				fixPath(&v, op.Path, unit)
				_, view, _ = parseJSON(v.Pack())
				continue
			}
			val = op.List
		} else if have == len(op.Path) && (op.IfMissing || sameJSON(cur, op.Val)) {
			continue
		}
		if op.IfMissing {
			added = append(added, op.key())
		}

		// build the missing part of the path around the value
		at := have
		if at == len(op.Path) {
			at = len(op.Path) - 1 // whole path is there, replace the last member
		}
		for i := len(op.Path) - 1; i > at; i-- {
			val = map[string]any{op.Path[i]: val}
		}
		if err := patchOne(&v, "add", pointer(op.Path[:at+1]), pretty(val, strings.Repeat(unit, at+1), unit)); err != nil {
			return nil, nil, err
		}
		changed = true
		fixPath(&v, op.Path, unit)
		_, view, _ = parseJSON(v.Pack())
	}
	if !changed {
		return b, nil, nil
	}
	// no Format(): it would retab the whole file
	return v.Pack(), added, nil
}

// ours lists IfMissing keys we added; other IfMissing ops are left alone
func jsonUndo(b []byte, ops []jsonOp, ours []string) ([]byte, error) {
	v, view, err := parseJSON(b)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		cur, have := walk(view, op.Path)
		if have != len(op.Path) {
			continue
		}
		if op.IfMissing && !hasStr(ours, op.key()) {
			continue
		}
		if op.List == nil {
			if err := patchOne(&v, "remove", pointer(op.Path), ""); err != nil {
				return nil, err
			}
		} else if arr, ok := cur.([]any); ok {
			ours := map[string]bool{}
			for _, s := range op.List {
				ours[s] = true
			}
			var idx []int
			for i, x := range arr {
				if s, ok := x.(string); ok && ours[s] {
					idx = append(idx, i)
				}
			}
			sort.Sort(sort.Reverse(sort.IntSlice(idx))) // back to front so indexes stay valid
			for _, i := range idx {
				if err := patchOne(&v, "remove", fmt.Sprintf("%s/%d", pointer(op.Path), i), ""); err != nil {
					return nil, err
				}
			}
		}
		_, view, _ = parseJSON(v.Pack())
	}
	return v.Pack(), nil
}

// jsonMissing says which ops aren't in place, for doctor
func jsonMissing(b []byte, ops []jsonOp) ([]string, error) {
	_, view, err := parseJSON(b)
	if err != nil {
		return nil, err
	}
	var bad []string
	for _, op := range ops {
		cur, have := walk(view, op.Path)
		name := strings.Join(op.Path, ".")
		switch {
		case have != len(op.Path):
			bad = append(bad, "no "+name)
		case op.IfMissing:
			if !sameJSON(cur, op.Val) {
				bad = append(bad, name+" is set to something else")
			}
		case op.List != nil:
			arr, _ := cur.([]any)
			seen := map[string]bool{}
			for _, x := range arr {
				if s, ok := x.(string); ok {
					seen[s] = true
				}
			}
			for _, s := range op.List {
				if !seen[s] {
					bad = append(bad, name+" doesn't list "+s)
				}
			}
		case !sameJSON(cur, op.Val):
			bad = append(bad, name+" is out of date. run `aivault sync`")
		}
	}
	return bad, nil
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

// hujson puts new members right after the comma with no newline. give the
// ones along our path the same indent as their neighbours
func fixPath(v *hujson.Value, path []string, unit string) {
	o, _ := v.Value.(*hujson.Object)
	if o != nil && len(o.Members) > 0 && !strings.Contains(string(o.AfterExtra), "\n") {
		o.AfterExtra = hujson.Extra("\n") // `{}` grew keys: close on its own line
	}
	for d, k := range path {
		if o == nil {
			return
		}
		m := member(o, k)
		if m == nil {
			return
		}
		ind := indentOf(o, strings.Repeat(unit, d+1))
		if len(m.Name.BeforeExtra) == 0 {
			m.Name.BeforeExtra = hujson.Extra("\n" + ind)
			if len(m.Value.BeforeExtra) == 0 {
				m.Value.BeforeExtra = hujson.Extra(" ") // `"key": value`, not `"key":value`
			}
		}
		if a, ok := m.Value.Value.(*hujson.Array); ok && len(a.Elements) > 0 {
			// appended items copy the first item's indent
			// or a space when the array is on one line
			sep := " "
			first := string(a.Elements[0].BeforeExtra)
			if i := strings.LastIndex(first, "\n"); i >= 0 {
				sep = first[i:]
			}
			for j := 1; j < len(a.Elements); j++ {
				if len(a.Elements[j].BeforeExtra) == 0 {
					a.Elements[j].BeforeExtra = hujson.Extra(sep)
				}
			}
		}
		o, _ = m.Value.Value.(*hujson.Object)
	}
}
