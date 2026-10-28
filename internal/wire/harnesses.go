package wire

import (
	"path/filepath"
	"strings"
)

// every harness beyond claude code, codex and opencode. paths checked against
// each tool's docs or source on 2026-10-19. they move, so re-check before a release

// gemini cli: GEMINI.md expands @imports
var gemini = build(&spec{
	name: "gemini", label: "Gemini CLI", bins: []string{"gemini"},
	// seen on gemini 0.64: in an untrusted folder every mcp server is off, user-level ones too
	note:  "gemini turns off all mcp servers in folders you haven't trusted. say yes when it asks to trust a folder",
	dir:   geminiDir,
	rules: in(geminiDir, "GEMINI.md"), rulesHow: rulesImport,
	skills:    agentsSkills,
	skillFrom: func(e *Env) []string { return []string{filepath.Join(geminiDir(e), "skills"), e.AgentsSkills} },
	mcpFile:   in(geminiDir, "settings.json"), mcpKind: mcpJSON,
	mcpPath: []string{"mcpServers", mcpName}, mcpEntry: cmdArgs,
})

// GEMINI_CLI_HOME stands in for the home folder, not for ~/.gemini
func geminiDir(e *Env) string {
	if v := e.getenv("GEMINI_CLI_HOME"); v != "" {
		return filepath.Join(v, ".gemini")
	}
	return filepath.Join(e.Home, ".gemini")
}

// kilo code is an opencode fork: same config shape, its own folder
var kiloDir = func(e *Env) string {
	if v := e.getenv("KILO_CONFIG_DIR"); v != "" {
		return v
	}
	return xdgDir("kilo")(e)
}

func kiloCfg(e *Env) string {
	jc := filepath.Join(kiloDir(e), "kilo.jsonc")
	if exists(jc) {
		return jc
	}
	return filepath.Join(kiloDir(e), "kilo.json")
}

var kilo = build(&spec{
	name: "kilo", label: "Kilo Code", bins: []string{"kilo", "kilocode"},
	dir:  kiloDir,
	list: kiloCfg, listPath: []string{"instructions"},
	importAt: in(kiloDir, "AGENTS.md"),
	skills:   agentsSkills,
	skillFrom: func(e *Env) []string {
		return []string{filepath.Join(kiloDir(e), "skills"), filepath.Join(e.Home, ".kilocode", "skills"), e.AgentsSkills}
	},
	mcpFile: kiloCfg, mcpKind: mcpJSON, mcpPath: []string{"mcp", mcpName},
	mcpEntry: opencodeEntry,
})

// crush: crush.json. newer builds also read a bash-like crushrc, but every
// release reads the json and merges both
var crush = build(&spec{
	name: "crush", label: "Crush", bins: []string{"crush"},
	dir:  crushDir,
	list: in(crushDir, "crush.json"), listPath: []string{"options", "global_context_paths"},
	importAt: in(crushDir, "CRUSH.md"),
	skills:   agentsSkills,
	skillFrom: func(e *Env) []string {
		return []string{filepath.Join(crushDir(e), "skills"), filepath.Join(e.Home, ".config", "agents", "skills"), e.AgentsSkills}
	},
	mcpFile: in(crushDir, "crush.json"), mcpKind: mcpJSON, mcpPath: []string{"mcp", mcpName},
	mcpEntry: func(bin string, args []string) any {
		return map[string]any{"type": "stdio", "command": bin, "args": args}
	},
})

// CRUSH_GLOBAL_CONFIG is the folder that holds crush.json
func crushDir(e *Env) string {
	if v := e.getenv("CRUSH_GLOBAL_CONFIG"); v != "" {
		return v
	}
	return xdgDir("crush")(e)
}

// goose: .goosehints is always loaded. mcp servers are `extensions` in config.yaml
var gooseDir = winDir("APPDATA", filepath.Join("Block", "goose", "config"), func(e *Env) string {
	if v := e.getenv("GOOSE_PATH_ROOT"); v != "" {
		return filepath.Join(v, "config")
	}
	return xdgDir("goose")(e)
})

var goose = build(&spec{
	name: "goose", label: "Goose", bins: []string{"goose"},
	dir:   gooseDir,
	rules: in(gooseDir, ".goosehints"), rulesHow: rulesCopy,
	skills:  agentsSkills,
	mcpFile: in(gooseDir, "config.yaml"), mcpKind: mcpYAML, mcpPath: []string{"extensions"},
	mcpYAML: func(bin string, args []string) []ykv {
		return []ykv{
			{"type", "stdio"}, {"name", mcpName}, {"enabled", true},
			{"cmd", bin}, {"args", args}, {"env_keys", yamlRaw("[]")}, {"envs", yamlRaw("{}")}, {"timeout", 300},
		}
	},
})

// pi
var piDir = homeDir("PI_CODING_AGENT_DIR", filepath.Join(".pi", "agent"))

var pi = build(&spec{
	name: "pi", label: "Pi", bins: []string{"pi"},
	dir:   piDir,
	rules: in(piDir, "AGENTS.md"), rulesHow: rulesCopy,
	check: func(e *Env, s *State) []string {
		if ov, _ := read(filepath.Join(piDir(e), "AGENTS.override.md")); len(strings.TrimSpace(string(ov))) > 0 {
			return []string{"pi has AGENTS.override.md, so it ignores AGENTS.md and the vault rules"}
		}
		return nil
	},
	skills:    agentsSkills,
	skillFrom: func(e *Env) []string { return []string{filepath.Join(piDir(e), "skills"), e.AgentsSkills} },
	mcpFile:   in(piDir, "mcp.json"), mcpKind: mcpJSON,
	mcpPath: []string{"mcpServers", mcpName}, mcpEntry: cmdArgs,
})

// hermes: SOUL.md is the only global context file it has
var hermesDir = winDir("LOCALAPPDATA", "hermes", homeDir("HERMES_HOME", ".hermes"))

var hermes = build(&spec{
	name: "hermes", label: "Hermes Agent", bins: []string{"hermes"},
	memories: []memSrc{
		{files: memFile(func(e *Env) string { return filepath.Join(hermesDir(e), "memories", "MEMORY.md") }), split: splitSep, prefix: "§"},
		{files: memFile(func(e *Env) string { return filepath.Join(hermesDir(e), "memories", "USER.md") }), split: splitSep, prefix: "§"},
	},
	dir:   hermesDir,
	rules: in(hermesDir, "SOUL.md"), rulesHow: rulesCopy,
	// SOUL.md loses its middle past context_file_max_chars. unset, that's
	// 6% of the model's window and never under 20000 chars, so pin it
	// (agent/prompt_builder.py)
	limit:   &limit{def: 20000, chars: true, file: in(hermesDir, "config.yaml"), key: "context_file_max_chars", yaml: true},
	skills:  in(hermesDir, "skills"),
	mcpFile: in(hermesDir, "config.yaml"), mcpKind: mcpYAML, mcpPath: []string{"mcp_servers"},
	mcpYAML: func(bin string, args []string) []ykv {
		return []ykv{{"command", bin}, {"args", args}}
	},
})

// cline: every .md in ~/.cline/rules loads, so we get a file of our own
var clineDir = homeDir("CLINE_DIR", ".cline")

var cline = build(&spec{
	name: "cline", label: "Cline", bins: []string{"cline"},
	dir:   clineDir,
	rules: in(clineDir, "rules", "ai-vault.md"), rulesHow: rulesCopy,
	skills:    agentsSkills,
	skillFrom: func(e *Env) []string { return []string{filepath.Join(clineDir(e), "skills"), e.AgentsSkills} },
	mcpFile:   in(clineDir, "data", "settings", "cline_mcp_settings.json"), mcpKind: mcpJSON,
	mcpPath: []string{"mcpServers", mcpName},
	mcpEntry: func(bin string, args []string) any {
		return map[string]any{"command": bin, "args": args, "disabled": false}
	},
})

// zed
var zedDir = winDir("APPDATA", "Zed", xdgDir("zed"))

var zed = build(&spec{
	name: "zed", label: "Zed", bins: []string{"zed"},
	dir:   zedDir,
	rules: in(zedDir, "AGENTS.md"), rulesHow: rulesCopy,
	skills:  agentsSkills,
	mcpFile: in(zedDir, "settings.json"), mcpKind: mcpJSON,
	mcpPath: []string{"context_servers", mcpName}, mcpEntry: cmdArgs,
})

// warp keeps its own rules in the cloud. its agent reads ~/.agents/AGENTS.md
var warp = build(&spec{
	name: "warp", label: "Warp",
	dir:   homeDir("", ".warp"),
	rules: func(e *Env) string { return filepath.Join(filepath.Dir(e.AgentsSkills), "AGENTS.md") }, rulesHow: rulesCopy,
	skills:  agentsSkills,
	mcpFile: in(homeDir("", ".warp"), ".mcp.json"), mcpKind: mcpJSON,
	mcpPath: []string{"mcpServers", mcpName}, mcpEntry: cmdArgs,
})

// open interpreter is a codex fork now: AGENTS.md copy, toml mcp
var oiDir = homeDir("INTERPRETER_HOME", ".openinterpreter")

var openInterpreter = build(&spec{
	name: "interpreter", label: "Open Interpreter", bins: []string{"interpreter"},
	dir:   oiDir,
	rules: in(oiDir, "AGENTS.md"), rulesHow: rulesCopy,
	skills:  agentsSkills,
	mcpFile: in(oiDir, "config.toml"), mcpKind: mcpTOML, mcpPath: []string{"mcp_servers", mcpName},
})

// aider: no mcp, no skills, no global rules file. `read:` loads files every session
var aider = build(&spec{
	name: "aider", label: "Aider", bins: []string{"aider"},
	dir:   func(e *Env) string { return filepath.Join(e.Home, ".aider.conf.yml") },
	ylist: func(e *Env) string { return filepath.Join(e.Home, ".aider.conf.yml") }, ylistKey: "read",
})

// ---- chinese harnesses, checked 2026-10-19/20 ----

// qwen code (alibaba): a gemini cli fork, QWEN.md expands @imports
var qwenDir = homeDir("QWEN_HOME", ".qwen")

var qwen = build(&spec{
	name: "qwen", label: "Qwen Code", bins: []string{"qwen"},
	// qwen saves "remember this" here instead of the vault, so sync pulls it in
	memories: []memSrc{{files: memTree(func(e *Env) string { return filepath.Join(qwenDir(e), "memories") })}},
	dir:      qwenDir,
	rules:    in(qwenDir, "QWEN.md"), rulesHow: rulesImport,
	skills:    agentsSkills,
	skillFrom: func(e *Env) []string { return []string{filepath.Join(qwenDir(e), "skills"), e.AgentsSkills} },
	mcpFile:   in(qwenDir, "settings.json"), mcpKind: mcpJSON,
	mcpPath: []string{"mcpServers", mcpName}, mcpEntry: cmdArgs,
})

// kimi code (moonshot). the old python kimi cli in ~/.kimi is archived
var kimiDir = homeDir("KIMI_CODE_HOME", ".kimi-code")

var kimi = build(&spec{
	name: "kimi", label: "Kimi Code", bins: []string{"kimi"},
	dir:   kimiDir,
	rules: in(kimiDir, "AGENTS.md"), rulesHow: rulesCopy,
	skills:    agentsSkills,
	skillFrom: func(e *Env) []string { return []string{filepath.Join(kimiDir(e), "skills"), e.AgentsSkills} },
	mcpFile:   in(kimiDir, "mcp.json"), mcpKind: mcpJSON,
	mcpPath: []string{"mcpServers", mcpName}, mcpEntry: cmdArgs,
})

// deepseek harness. mcp servers are `insert` patches in a top-level yaml list
var dshDir = homeDir("DSH_HOME", ".dsh")

var dsh = build(&spec{
	name: "dsh", label: "DeepSeek Harness", bins: []string{"dsh"},
	memories: []memSrc{{files: memFile(func(e *Env) string { return filepath.Join(dshDir(e), "lessons.md") }), split: splitBullets}},
	dir:      dshDir,
	rules:    in(dshDir, "AGENTS.md"), rulesHow: rulesCopy,
	// every AGENTS.md of a session shares one budget, global first
	// (dsh-agent-instructions maxBytes in dsh-base/cordis.patch.yml)
	limit:     &limit{def: 65536, shared: true},
	skills:    agentsSkills,
	skillFrom: func(e *Env) []string { return []string{filepath.Join(dshDir(e), "skills"), e.AgentsSkills} },
	yblock:    in(dshDir, "cordis.patch.yml"),
	yblockLines: func(e *Env, s *State) []string {
		cfg := yamlLines([]ykv{
			{"serverName", mcpName}, {"transport", "stdio"},
			{"command", e.Bin}, {"args", mcpArgs(s)},
		}, "        ", "  ")
		return append([]string{
			"- insert:",
			"    - id: mcp-" + mcpName,
			"      name: " + yamlScalar("@deepseek-ai/dsh-mcp-client"),
			"      config:",
		}, cfg...)
	},
})

// zcode (zhipu)
var zcodeDir = homeDir("", ".zcode")

var zcode = build(&spec{
	name: "zcode", label: "ZCode", bins: []string{"zcode"},
	dir:   zcodeDir,
	rules: in(zcodeDir, "AGENTS.md"), rulesHow: rulesCopy,
	skills:    agentsSkills,
	skillFrom: func(e *Env) []string { return []string{filepath.Join(zcodeDir(e), "skills"), e.AgentsSkills} },
	mcpFile:   in(zcodeDir, "cli", "config.json"), mcpKind: mcpJSON,
	mcpPath: []string{"mcp", "servers", mcpName},
	mcpEntry: func(bin string, args []string) any {
		return map[string]any{"type": "stdio", "command": bin, "args": args}
	},
	// zcode skips mcp servers unless this is on. a user's explicit false stays
	extraJSON: []jsonOp{{Path: []string{"features", "mcp"}, Val: true, IfMissing: true}},
})

// minimax code (mcode). data lives in ~/.minimax
var minimaxDir = homeDir("MINIMAX_DATA_DIR", ".minimax")

var minimax = build(&spec{
	name: "minimax", label: "MiniMax Code", bins: []string{"mcode"},
	dir:   minimaxDir,
	rules: in(minimaxDir, "AGENTS.md"), rulesHow: rulesCopy,
	skills:    agentsSkills,
	skillFrom: func(e *Env) []string { return []string{filepath.Join(minimaxDir(e), "skills"), e.AgentsSkills} },
	mcpFile:   in(minimaxDir, "mcp.json"), mcpKind: mcpJSON,
	mcpPath: []string{"mcpServers", mcpName},
	mcpEntry: func(bin string, args []string) any {
		return map[string]any{"type": "stdio", "command": bin, "args": args}
	},
})

// kode: no global rules file at all, so it gets skills and the mcp tools
func kodeCfg(e *Env) string {
	if v := e.getenv("KODE_CONFIG_DIR"); v != "" {
		return filepath.Join(v, "config.json")
	}
	return filepath.Join(e.Home, ".kode.json")
}

var kode = build(&spec{
	name: "kode", label: "Kode", bins: []string{"kode"},
	dir:     homeDir("", ".kode"),
	skills:  in(homeDir("", ".kode"), "skills"),
	mcpFile: kodeCfg, mcpKind: mcpJSON,
	mcpPath: []string{"mcpServers", mcpName}, mcpEntry: cmdArgs,
})

// neovate code (ant group). it also reads ~/.claude/CLAUDE.md on its own
var neovateDir = homeDir("", ".neovate")

var neovate = build(&spec{
	name: "neovate", label: "Neovate Code", bins: []string{"neovate"},
	dir:   neovateDir,
	rules: in(neovateDir, "AGENTS.md"), rulesHow: rulesCopy,
	skills:  in(neovateDir, "skills"),
	mcpFile: in(neovateDir, "config.json"), mcpKind: mcpJSON,
	mcpPath: []string{"mcpServers", mcpName}, mcpEntry: cmdArgs,
})
