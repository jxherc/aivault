// wire connects the vault to each ai tool and can undo it.
package wire

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Env is every path we touch. tests swap it for a fake home
type Env struct {
	Home         string
	ClaudeDir    string
	CodexDir     string
	OpencodeDir  string
	AgentsSkills string // ~/.agents/skills, read by codex and opencode
	StateDir     string
	Bin          string // what tools should launch for `mcp`

	// runs a cli (only claude's, for now). nil = don't run anything
	Run func(name string, args ...string) error
	// is this cli on PATH
	Has func(name string) bool
	// env lookups for tool homes like GEMINI_CLI_HOME. nil = none set
	Getenv func(string) string
}

func (e *Env) getenv(k string) string {
	if e.Getenv == nil || k == "" {
		return ""
	}
	return e.Getenv(k)
}

func DefaultEnv() (*Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	e := &Env{
		Home:         home,
		ClaudeDir:    envOr("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude")),
		CodexDir:     envOr("CODEX_HOME", filepath.Join(home, ".codex")),
		OpencodeDir:  filepath.Join(envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config")), "opencode"),
		AgentsSkills: filepath.Join(home, ".agents", "skills"),
		StateDir:     filepath.Join(home, ".aivault"),
		Bin:          selfPath(),
		Getenv:       os.Getenv,
		Has: func(name string) bool {
			_, err := exec.LookPath(name)
			return err == nil
		},
		Run: func(name string, args ...string) error {
			c := exec.Command(name, args...)
			c.Stdout, c.Stderr = nil, nil
			return c.Run()
		},
	}
	return e, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// prefer the PATH copy (brew/scoop symlink) over the resolved binary, so an
// upgrade doesn't leave tools pointing at an old versioned folder
func selfPath() string {
	if p, err := exec.LookPath("aivault"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
	}
	p, err := os.Executable()
	if err != nil {
		return "aivault"
	}
	return p
}
