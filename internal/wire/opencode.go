package wire

import (
	"path/filepath"

	"github.com/jxherc/aivault/internal/vault"
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

func opencodeOps(e *Env, s *State) []jsonOp {
	return []jsonOp{
		{Path: []string{"instructions"}, List: opencodePaths(s)},
		{Path: []string{"mcp", mcpName}, Val: map[string]any{
			"type":    "local",
			"command": append([]string{e.Bin}, mcpArgs(s)...),
			"enabled": true,
		}},
	}
}

var opencode = build(&spec{
	name: "opencode", label: "OpenCode", bins: []string{"opencode"},
	dir:      func(e *Env) string { return e.OpencodeDir },
	list:     opencodeCfg,
	listPath: []string{"instructions"},
	skills:   agentsSkills,
	// opencode also reads its own skills folder and ~/.claude/skills
	skillFrom: func(e *Env) []string { return []string{filepath.Join(e.OpencodeDir, "skills"), e.AgentsSkills} },
	// only for import: opencode's global rules. we load the vault through instructions instead
	importAt: func(e *Env) string { return filepath.Join(e.OpencodeDir, "AGENTS.md") },
	mcpFile:  opencodeCfg,
	mcpKind:  mcpJSON,
	mcpPath:  []string{"mcp", mcpName},
	mcpEntry: opencodeEntry,
})

// opencode and kilo: one `command` array, program first
func opencodeEntry(bin string, args []string) any {
	return map[string]any{"type": "local", "command": append([]string{bin}, args...), "enabled": true}
}
