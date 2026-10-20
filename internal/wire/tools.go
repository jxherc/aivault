package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jxherc/aivault/internal/vault"
)

const mcpName = "ai-vault"

// the vault path goes in the command itself, so the server works no matter
// which home or state file the tool launches it with
func mcpArgs(s *State) []string { return []string{"mcp", "--vault", s.Vault} }

type Tool struct {
	Name   string // used in flags and state
	Label  string
	Detect func(e *Env) bool
	Skills func(e *Env) string // where we link vault skills for this tool, "" = none
	// for import: the global rules file, and every skills folder the tool reads
	RulesFile func(e *Env) string
	SkillFrom func(e *Env) []string
	Apply     func(e *Env, s *State) error
	Remove    func(e *Env, s *State) error
	Check     func(e *Env, s *State) []string
}

var All = []*Tool{
	claude, codex, opencode, gemini, kilo, crush, goose, pi, hermes,
	cline, zed, warp, openInterpreter, aider,
	qwen, kimi, dsh, zcode, minimax, kode, neovate,
}

func Find(name string) *Tool {
	for _, t := range All {
		if t.Name == name {
			return t
		}
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ---- claude code ----
// CLAUDE.md gets a block with @imports, mcp goes through `claude mcp add`
// because ~/.claude.json is big, busy and written by claude itself

// claude splits an @import at the first space unless it's escaped
func claudeImport(p string) string {
	return "@" + strings.ReplaceAll(filepath.ToSlash(p), " ", `\ `)
}

var claude = &Tool{
	Name:      "claude",
	Label:     "Claude Code",
	RulesFile: func(e *Env) string { return filepath.Join(e.ClaudeDir, "CLAUDE.md") },
	SkillFrom: func(e *Env) []string { return []string{filepath.Join(e.ClaudeDir, "skills")} },
	Detect: func(e *Env) bool {
		return exists(e.ClaudeDir) || e.Has("claude")
	},
	Skills: func(e *Env) string { return filepath.Join(e.ClaudeDir, "skills") },
	Apply: func(e *Env, s *State) error {
		p := filepath.Join(e.ClaudeDir, "CLAUDE.md")
		var lines []string
		for _, f := range vault.RulesFiles(s.Vault) {
			lines = append(lines, claudeImport(f))
		}
		if err := setBlock(s, p, mdMarks, strings.Join(lines, "\n")); err != nil {
			return err
		}
		if !e.Has("claude") {
			return fmt.Errorf("rules are wired, but the claude cli isn't on PATH so the mcp server wasn't added. run `aivault sync` once it is")
		}
		_ = e.Run("claude", "mcp", "remove", "--scope", "user", mcpName) // fine if it wasn't there
		if err := e.Run("claude", append([]string{"mcp", "add", "--scope", "user", mcpName, "--", e.Bin}, mcpArgs(s)...)...); err != nil {
			return fmt.Errorf("claude mcp add failed: %w", err)
		}
		s.ClaudeMCP = true
		return nil
	},
	Remove: func(e *Env, s *State) error {
		if err := unBlock(s, filepath.Join(e.ClaudeDir, "CLAUDE.md"), mdMarks); err != nil {
			return err
		}
		if s.ClaudeMCP {
			if !e.Has("claude") {
				return fmt.Errorf("claude cli not on PATH, couldn't remove the mcp server. run `claude mcp remove --scope user %s` later", mcpName)
			}
			if err := e.Run("claude", "mcp", "remove", "--scope", "user", mcpName); err != nil {
				return fmt.Errorf("claude mcp remove failed: %w", err)
			}
			s.ClaudeMCP = false
		}
		return nil
	},
	Check: func(e *Env, s *State) []string {
		var bad []string
		p := filepath.Join(e.ClaudeDir, "CLAUDE.md")
		b, _ := read(p)
		if !mdMarks.has(b) {
			bad = append(bad, p+" has no ai-vault block")
		}
		switch {
		case !s.ClaudeMCP:
			bad = append(bad, "the mcp server was never added to claude code. run `aivault sync` with the claude cli on PATH")
		case e.Has("claude") && e.Run("claude", "mcp", "get", mcpName) != nil:
			bad = append(bad, "claude code has no ai-vault mcp server")
		}
		return bad
	},
}

// ---- codex ----
// codex doesn't expand @imports, so its AGENTS.md block holds a copy.
// mcp is a toml table in config.toml inside our own marker comments

var codex = build(&spec{
	name: "codex", label: "Codex", bins: []string{"codex"},
	dir:   func(e *Env) string { return e.CodexDir },
	rules: func(e *Env) string { return filepath.Join(e.CodexDir, "AGENTS.md") }, rulesHow: rulesCopy, maxRules: codexLimit,
	skills:  agentsSkills,
	mcpFile: func(e *Env) string { return filepath.Join(e.CodexDir, "config.toml") },
	mcpKind: mcpTOML, mcpPath: []string{"mcp_servers", mcpName},
	check: func(e *Env, s *State) []string {
		if ov, _ := read(filepath.Join(e.CodexDir, "AGENTS.override.md")); len(bytes.TrimSpace(ov)) > 0 {
			return []string{"codex has AGENTS.override.md, so it ignores AGENTS.md and the vault rules"}
		}
		return nil
	},
})

const codexLimit = 32 * 1024 // codex's project_doc_max_bytes default

func tomlStr(v string) string {
	b, _ := json.Marshal(v) // json string escapes are valid toml basic strings
	return string(b)
}

// shared helpers for marker blocks

func setBlock(s *State, p string, m marks, body string) error {
	b, err := read(p)
	if err != nil {
		return err
	}
	out, sep := m.set(b, body)
	if bytes.Equal(out, b) {
		return nil // already right, don't touch the file
	}
	if err := s.write(p, out); err != nil {
		return err
	}
	if sep != "" {
		s.Files[p].Sep = sep
	}
	return nil
}

func unBlock(s *State, p string, m marks) error {
	rec := s.Files[p]
	if rec == nil {
		return nil
	}
	return s.undo(p, func(b []byte) ([]byte, error) { return m.strip(b, rec.Sep), nil })
}
