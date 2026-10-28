package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jxherc/aivault/internal/vault"
)

// most harnesses fit one shape: a global rules file (or a list of extra
// instruction files in their config), a skills folder, and an mcp entry in a
// config file. a spec says where those are; build turns it into a Tool

const (
	rulesImport = iota + 1 // marked block with @path lines (the tool expands them)
	rulesCopy              // marked block holding a copy of the vault rules
)

const (
	mcpJSON = iota + 1 // object member at path in a json(c) file
	mcpTOML            // [table] between marker comments
	mcpYAML            // entry under a top-level key, between marker comments
)

type spec struct {
	name, label string
	bins        []string
	dir         func(e *Env) string // config home. it existing counts as installed

	rules    func(e *Env) string // global rules file we write a block into. nil = none
	rulesHow int
	limit    *limit // what the tool cuts the rules file at, nil = reads it whole

	// or: the vault files go into a list in the tool's config
	list     func(e *Env) string // json file
	listPath []string
	ylist    func(e *Env) string // yaml file
	ylistKey string

	// or a raw yaml block of our own (dsh's top-level patch list)
	yblock      func(e *Env) string
	yblockKey   string // "" = the file is a top-level list
	yblockLines func(e *Env, s *State) []string

	skills    func(e *Env) string   // where we link vault skills. nil = none
	skillFrom func(e *Env) []string // folders import reads skills from. nil = just skills
	importAt  func(e *Env) string   // rules file import reads, when it's not `rules`

	check    func(e *Env, s *State) []string // anything tool-specific for doctor
	note     string                          // see Tool.Note
	memories []memSrc                        // see Tool.Memories
	hints    func(e *Env, s *State) []string

	mcpFile   func(e *Env) string
	mcpKind   int
	mcpPath   []string                              // json: path to our entry. toml: table name. yaml: [key]
	mcpEntry  func(bin string, args []string) any   // json shape
	extraJSON []jsonOp                              // more json the mcp file needs, like zcode's features.mcp
	tomlExtra []string                              // more lines in our toml table
	mcpYAML   func(bin string, args []string) []ykv // yaml shape, under mcpPath[0] as mcpName
}

func in(dir func(*Env) string, name ...string) func(*Env) string {
	return func(e *Env) string { return filepath.Join(append([]string{dir(e)}, name...)...) }
}

// ~/rel, or $env when it's set
func homeDir(env, rel string) func(*Env) string {
	return func(e *Env) string {
		if v := e.getenv(env); v != "" {
			return v
		}
		return filepath.Join(e.Home, rel)
	}
}

// $XDG_CONFIG_HOME/name, falling back to ~/.config/name on every os
func xdgDir(name string) func(*Env) string {
	return func(e *Env) string {
		if v := e.getenv("XDG_CONFIG_HOME"); v != "" {
			return filepath.Join(v, name)
		}
		return filepath.Join(e.Home, ".config", name)
	}
}

// some tools keep their config in %APPDATA% or %LOCALAPPDATA% on windows
func winDir(env string, rel string, other func(*Env) string) func(*Env) string {
	return func(e *Env) string {
		if runtime.GOOS == "windows" {
			if v := e.getenv(env); v != "" {
				return filepath.Join(v, rel)
			}
		}
		return other(e)
	}
}

func agentsSkills(e *Env) string { return e.AgentsSkills }

// the shape most tools use: {"command": ..., "args": [...]}
func cmdArgs(bin string, args []string) any {
	return map[string]any{"command": bin, "args": args}
}

func vaultPaths(s *State) []string {
	var paths []string
	for _, p := range vault.RulesFiles(s.Vault) {
		paths = append(paths, filepath.ToSlash(p))
	}
	return paths
}

// the vault paths in a json list. Prefix lets a later run drop a path whose
// file was deleted from the vault
func (sp *spec) listOp(s *State) jsonOp {
	return jsonOp{Path: sp.listPath, List: vaultPaths(s), Prefix: filepath.ToSlash(s.Vault) + "/"}
}

func (sp *spec) jsonOps(e *Env, s *State) map[string][]jsonOp {
	ops := map[string][]jsonOp{}
	if sp.list != nil {
		ops[sp.list(e)] = append(ops[sp.list(e)], sp.listOp(s))
	}
	if sp.mcpKind == mcpJSON {
		f := sp.mcpFile(e)
		ops[f] = append(ops[f], jsonOp{Path: sp.mcpPath, Val: sp.mcpEntry(e.Bin, mcpArgs(s))})
		ops[f] = append(ops[f], sp.extraJSON...)
	}
	return ops
}

type yamlEdit struct {
	key   string
	child []string
}

// yaml files and the lines we own in each
func (sp *spec) yamlEdits(e *Env, s *State) map[string]yamlEdit {
	out := map[string]yamlEdit{}
	if sp.ylist != nil {
		out[sp.ylist(e)] = yamlEdit{sp.ylistKey, yamlItems(vaultPaths(s))}
	}
	if sp.mcpKind == mcpYAML {
		out[sp.mcpFile(e)] = yamlEdit{sp.mcpPath[0], yamlEntry(mcpName, sp.mcpYAML(e.Bin, mcpArgs(s)))}
	}
	if sp.yblock != nil {
		out[sp.yblock(e)] = yamlEdit{sp.yblockKey, sp.yblockLines(e, s)}
	}
	return out
}

func (sp *spec) rulesBody(s *State) (string, error) {
	// claude documents `\ ` for spaces in @paths. others don't say, so copy instead
	if sp.rulesHow == rulesImport && !strings.Contains(s.Vault, " ") {
		var lines []string
		for _, f := range vault.RulesFiles(s.Vault) {
			lines = append(lines, claudeImport(f))
		}
		return strings.Join(lines, "\n"), nil
	}
	rules, err := vault.Rules(s.Vault)
	if err != nil {
		return "", err
	}
	return "<!-- copy of " + filepath.ToSlash(s.Vault) + ". edit the vault, then run `aivault sync` -->\n" + rules, nil
}

func (sp *spec) tomlTable(e *Env, s *State) string {
	args, _ := json.Marshal(mcpArgs(s)) // a json string array is a valid toml array
	t := fmt.Sprintf("[%s]\ncommand = %s\nargs = %s", strings.Join(sp.mcpPath, "."), tomlStr(e.Bin), args)
	for _, l := range sp.tomlExtra {
		t += "\n" + l
	}
	return t
}

func build(sp *spec) *Tool {
	t := &Tool{Name: sp.name, Label: sp.label, Note: sp.note, Hints: sp.hints}
	if sp.memories != nil {
		t.Memories = func(e *Env) []memSrc { return sp.memories }
	}
	t.Detect = func(e *Env) bool {
		if exists(sp.dir(e)) {
			return true
		}
		for _, b := range sp.bins {
			if e.Has(b) {
				return true
			}
		}
		return false
	}
	t.Skills = func(e *Env) string {
		if sp.skills == nil {
			return ""
		}
		return sp.skills(e)
	}
	t.RulesFile = func(e *Env) string {
		switch {
		case sp.rules != nil:
			return sp.rules(e)
		case sp.importAt != nil:
			return sp.importAt(e)
		}
		return ""
	}
	t.SkillFrom = func(e *Env) []string {
		if sp.skillFrom != nil {
			return sp.skillFrom(e)
		}
		if sp.skills != nil {
			return []string{sp.skills(e)}
		}
		return nil
	}

	// everything that lists or copies the rules files, nothing else
	t.Refresh = func(e *Env, s *State) error {
		if sp.rules != nil {
			body, err := sp.rulesBody(s)
			if err != nil {
				return err
			}
			if err := setBlock(s, sp.rules(e), mdMarks, body); err != nil {
				return err
			}
			if err := sp.fitLimit(e, s); err != nil {
				return err
			}
		}
		if sp.list != nil {
			f := sp.list(e)
			if err := applyJSON(s, map[string][]jsonOp{f: {sp.listOp(s)}}); err != nil {
				return err
			}
		}
		if sp.ylist != nil {
			return applyYAML(s, map[string]yamlEdit{sp.ylist(e): {sp.ylistKey, yamlItems(vaultPaths(s))}})
		}
		return nil
	}
	t.Apply = func(e *Env, s *State) error {
		if sp.rules != nil {
			body, err := sp.rulesBody(s)
			if err != nil {
				return err
			}
			if err := setBlock(s, sp.rules(e), mdMarks, body); err != nil {
				return err
			}
			if err := sp.fitLimit(e, s); err != nil {
				return err
			}
		}
		if err := applyJSON(s, sp.jsonOps(e, s)); err != nil {
			return err
		}
		if err := applyYAML(s, sp.yamlEdits(e, s)); err != nil {
			return err
		}
		if sp.mcpKind == mcpTOML {
			f := sp.mcpFile(e)
			b, err := read(f)
			if err != nil {
				return err
			}
			// someone already has a table with our name outside the block.
			// a duplicate would stop the tool from starting
			header := "[" + strings.Join(sp.mcpPath, ".") + "]"
			if bytes.Contains(tomlMarks.strip(b, ""), []byte(header)) {
				return fmt.Errorf("%s already has %s. remove it and run again", f, header)
			}
			if err := setBlock(s, f, tomlMarks, sp.tomlTable(e, s)); err != nil {
				return err
			}
		}
		return nil
	}

	t.Remove = func(e *Env, s *State) error {
		// the limit key can share a file with another block of ours. undo
		// drops the file's record, so both go in the same pass
		lf := ""
		if sp.limit != nil && sp.limit.file != nil {
			lf = sp.limit.file(e)
		}
		lim := func(f string, b []byte) []byte {
			if f == lf {
				return limitMarks.strip(b, "")
			}
			return b
		}
		sep := func(f string) string {
			if rec := s.Files[f]; rec != nil {
				return rec.Sep
			}
			return ""
		}
		if sp.rules != nil {
			f := sp.rules(e)
			if err := s.undo(f, func(b []byte) ([]byte, error) { return lim(f, mdMarks.strip(b, sep(f))), nil }); err != nil {
				return err
			}
		}
		for f, ops := range sp.jsonOps(e, s) {
			ops := ops
			var ours []string
			if rec := s.Files[f]; rec != nil {
				ours = rec.Added
			}
			if err := s.undo(f, func(b []byte) ([]byte, error) { return jsonUndo(b, ops, ours) }); err != nil {
				return err
			}
		}
		for f, y := range sp.yamlEdits(e, s) {
			if rec := s.Files[f]; rec != nil {
				f, key, line := f, y.key, rec.Line
				if err := s.undo(f, func(b []byte) ([]byte, error) { return lim(f, yamlStrip(b, key, line)), nil }); err != nil {
					return err
				}
			}
		}
		if sp.mcpKind == mcpTOML {
			f := sp.mcpFile(e)
			if err := s.undo(f, func(b []byte) ([]byte, error) { return lim(f, tomlMarks.strip(b, sep(f))), nil }); err != nil {
				return err
			}
		}
		if lf != "" {
			// a file of its own, or one the passes above didn't touch
			return s.undo(lf, func(b []byte) ([]byte, error) { return limitMarks.strip(b, ""), nil })
		}
		return nil
	}

	t.Check = func(e *Env, s *State) []string {
		var bad []string
		if sp.check != nil {
			bad = append(bad, sp.check(e, s)...)
		}
		if sp.rules != nil {
			p := sp.rules(e)
			b, _ := read(p)
			switch {
			case !mdMarks.has(b):
				bad = append(bad, p+" has no ai-vault block")
			default:
				if want, err := sp.rulesBody(s); err == nil && mdMarks.body(b) != strings.ReplaceAll(strings.Trim(want, "\n"), "\r\n", "\n") {
					bad = append(bad, p+" has an old copy of the vault rules. run `aivault sync`")
				}
				if sp.limit != nil {
					bad = append(bad, sp.limit.check(e, sp.label, p, b)...)
				}
			}
		}
		for f, ops := range sp.jsonOps(e, s) {
			b, _ := read(f)
			miss, err := jsonMissing(b, ops)
			if err != nil {
				bad = append(bad, f+" can't be parsed: "+err.Error())
			}
			for _, m := range miss {
				bad = append(bad, f+": "+m)
			}
		}
		for f, y := range sp.yamlEdits(e, s) {
			b, _ := read(f)
			want, _, _ := yamlSet(nil, y.key, y.child)
			switch {
			case !yamlHas(b):
				bad = append(bad, f+" has no ai-vault block")
			case !strings.HasSuffix(yamlBody(want), yamlBody(b)):
				// want has the key line inside the block, b may not
				bad = append(bad, f+": the ai-vault entry is out of date. run `aivault sync`")
			}
		}
		if sp.mcpKind == mcpTOML {
			f := sp.mcpFile(e)
			b, _ := read(f)
			if !tomlMarks.has(b) {
				bad = append(bad, f+" has no ai-vault mcp server")
			} else if tomlMarks.body(b) != sp.tomlTable(e, s) {
				bad = append(bad, f+": the ai-vault mcp entry is out of date. run `aivault sync`")
			}
		}
		return bad
	}
	return t
}

func (sp *spec) fitLimit(e *Env, s *State) error {
	if sp.limit == nil || sp.limit.file == nil {
		return nil
	}
	b, err := read(sp.rules(e))
	if err != nil {
		return err
	}
	return sp.limit.fit(e, s, sp.limit.size(b))
}

// a cap a tool puts on its rules file, from the tool's own source. when
// the tool has a setting for it, sync raises that setting with a key of our
// own at the top of the config (top-level keys have to come before any
// toml [table]). it's set again on every sync, so an app rewriting its
// config without it doesn't leave the rules cut
type limit struct {
	def    int  // what the tool cuts at when the setting isn't there
	chars  bool // counted in characters, not bytes
	shared bool // project instruction files share the same budget
	file   func(e *Env) string
	key    string
	yaml   bool // the config is yaml, not toml
}

var limitMarks = marks{"# ai-vault:limit:start", "# ai-vault:limit:end"}

func (l *limit) size(b []byte) int {
	if l.chars {
		return utf8.RuneCount(b)
	}
	return len(b)
}

func (l *limit) unit() string {
	if l.chars {
		return "characters"
	}
	return "bytes"
}

// the top-level key in b: its raw value, and whether it's there at all
func (l *limit) find(b []byte) (string, bool) {
	top := strings.ReplaceAll(string(b), "\r\n", "\n")
	sep := `\s*=\s*`
	if l.yaml {
		sep = `:[ \t]*` // unindented, so only top-level keys match
	} else {
		if i := strings.Index(top, "\n["); i >= 0 {
			top = top[:i]
		}
		if strings.HasPrefix(top, "[") {
			top = ""
		}
		top = regexp.MustCompile(`(?m)^[ \t]+`).ReplaceAllString(top, "")
	}
	m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(l.key) + sep + `([^#\n]*)`).FindStringSubmatch(top)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

// what the tool reads up to right now
func (l *limit) max(e *Env) int {
	if l.file == nil {
		return l.def
	}
	b, _ := read(l.file(e))
	v, ok := l.find(b)
	if n, err := strconv.Atoi(v); ok && err == nil && n > 0 {
		return n
	}
	return l.def
}

// room for n, in 32k steps so small edits don't rewrite the config each time
func need(n int) int {
	const step = 32 << 10
	return (n + 16<<10 + step - 1) / step * step
}

// raises the setting to fit n. drops our key when the default is enough,
// or when the user set the key themselves: a second one breaks the config
func (l *limit) fit(e *Env, s *State, n int) error {
	p := l.file(e)
	b, err := read(p)
	if err != nil {
		return err
	}
	rest := limitMarks.strip(b, "")
	if _, theirs := l.find(rest); theirs || n <= l.def-4096 {
		if !limitMarks.has(b) {
			return nil
		}
		return s.write(p, rest)
	}
	eq := " = "
	if l.yaml {
		eq = ": "
	}
	body := l.key + eq + strconv.Itoa(max(need(n), l.def))
	if limitMarks.body(b) == body {
		return nil
	}
	head, tail := []byte(nil), rest
	if l.yaml && bytes.HasPrefix(rest, []byte("---")) {
		// a yaml document start stays first
		i := bytes.IndexByte(rest, '\n') + 1
		if i == 0 {
			i = len(rest)
		}
		head, tail = rest[:i], rest[i:]
	}
	out := append(append(append([]byte{}, head...), limitMarks.render(body, nl(b))...), tail...)
	return s.write(p, out)
}

func (l *limit) check(e *Env, label, p string, b []byte) []string {
	n, lim := l.size(b), l.max(e)
	if n <= lim-1024 {
		return nil
	}
	msg := fmt.Sprintf("%s is %d %s, %s cuts it at %d", p, n, l.unit(), label, lim)
	switch {
	case l.file == nil && l.shared:
		msg = fmt.Sprintf("%s is %d %s, and %s fits it and every project AGENTS.md into %d. trim the vault rules", p, n, l.unit(), label, lim)
	case l.file == nil:
		msg += ". trim the vault rules"
	default:
		f, _ := read(l.file(e))
		if _, theirs := l.find(limitMarks.strip(f, "")); theirs {
			msg += fmt.Sprintf(". raise %s in %s to %d", l.key, l.file(e), need(n))
		} else {
			msg += ". run `aivault sync`"
		}
	}
	return []string{msg}
}

func applyJSON(s *State, ops map[string][]jsonOp) error {
	for f, ops := range ops {
		b, err := read(f)
		if err != nil {
			return err
		}
		out, added, err := jsonApply(b, ops)
		if err != nil {
			return fmt.Errorf("can't edit %s: %w", f, err)
		}
		if !bytes.Equal(out, b) {
			if err := s.write(f, out); err != nil {
				return err
			}
			for _, a := range added {
				if !hasStr(s.Files[f].Added, a) {
					s.Files[f].Added = append(s.Files[f].Added, a)
				}
			}
		}
	}
	return nil
}

func applyYAML(s *State, edits map[string]yamlEdit) error {
	for f, y := range edits {
		b, err := read(f)
		if err != nil {
			return err
		}
		out, orig, err := yamlSet(b, y.key, y.child)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		if !bytes.Equal(out, b) {
			if err := s.write(f, out); err != nil {
				return err
			}
			if orig != "" {
				s.Files[f].Line = orig
			}
		}
	}
	return nil
}
