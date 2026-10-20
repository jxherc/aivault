package wire

import (
	"fmt"
	"path/filepath"
	"strings"

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
	hints:    opencodeSkillHint,
})

// a `"*": "deny"` skill allowlist silently blocks vault skills that aren't
// listed. we don't edit someone's permission rules, we say which ones
func opencodeSkillHint(e *Env, s *State) []string {
	b, _ := read(opencodeCfg(e))
	_, view, err := parseJSON(b)
	if err != nil {
		return nil
	}
	rules, _ := walk(view, []string{"permission", "skill"})
	m, ok := rules.(map[string]any)
	if !ok || m["*"] != "deny" {
		return nil
	}
	skills, _ := vault.Skills(s.Vault)
	var blocked []string
	for _, sk := range skills {
		if m[sk.Name] != "allow" {
			blocked = append(blocked, sk.Name)
		}
	}
	if len(blocked) == 0 {
		return nil
	}
	ex := blocked
	if len(ex) > 5 {
		ex = ex[:5]
	}
	return []string{fmt.Sprintf("your opencode skill allowlist (\"*\": \"deny\") blocks %d of %d vault skills, e.g. %s. to allow one, add \"<name>\": \"allow\" under permission.skill in %s",
		len(blocked), len(skills), strings.Join(ex, ", "), tilde(opencodeCfg(e), e.Home))}
}

// opencode and kilo: one `command` array, program first
func opencodeEntry(bin string, args []string) any {
	return map[string]any{"type": "local", "command": append([]string{bin}, args...), "enabled": true}
}
