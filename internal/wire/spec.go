package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

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
	maxRules int // bytes the tool stops reading at, 0 = no limit

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

	check func(e *Env, s *State) []string // anything tool-specific for doctor
	note  string                          // see Tool.Note

	mcpFile   func(e *Env) string
	mcpKind   int
	mcpPath   []string                              // json: path to our entry. toml: table name. yaml: [key]
	mcpEntry  func(bin string, args []string) any   // json shape
	extraJSON []jsonOp                              // more json the mcp file needs, like zcode's features.mcp
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

func (sp *spec) jsonOps(e *Env, s *State) map[string][]jsonOp {
	ops := map[string][]jsonOp{}
	if sp.list != nil {
		ops[sp.list(e)] = append(ops[sp.list(e)], jsonOp{Path: sp.listPath, List: vaultPaths(s)})
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
	return fmt.Sprintf("[%s]\ncommand = %s\nargs = %s", strings.Join(sp.mcpPath, "."), tomlStr(e.Bin), args)
}

func build(sp *spec) *Tool {
	t := &Tool{Name: sp.name, Label: sp.label, Note: sp.note}
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

	t.Apply = func(e *Env, s *State) error {
		if sp.rules != nil {
			body, err := sp.rulesBody(s)
			if err != nil {
				return err
			}
			if err := setBlock(s, sp.rules(e), mdMarks, body); err != nil {
				return err
			}
		}
		for f, ops := range sp.jsonOps(e, s) {
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
		for f, y := range sp.yamlEdits(e, s) {
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
		if sp.rules != nil {
			if err := unBlock(s, sp.rules(e), mdMarks); err != nil {
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
				key, line := y.key, rec.Line
				if err := s.undo(f, func(b []byte) ([]byte, error) { return yamlStrip(b, key, line), nil }); err != nil {
					return err
				}
			}
		}
		if sp.mcpKind == mcpTOML {
			return unBlock(s, sp.mcpFile(e), tomlMarks)
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
				if sp.maxRules > 0 && len(b) > sp.maxRules-1024 {
					bad = append(bad, fmt.Sprintf("%s is %d bytes, %s stops reading at %d", p, len(b), sp.label, sp.maxRules))
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
