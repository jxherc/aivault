# ai vault

one folder of rules, skills and memory that all your ai coding tools read: claude code, codex, opencode, gemini cli, qwen code, kimi code, deepseek harness and 14 more.

save a preference in claude code, and codex or qwen code knows it the next time you open it. write a skill once and every tool has it.

## install

```sh
brew install --cask jxherc/tap/aivault
```

windows, with scoop:

```sh
scoop bucket add jxherc https://github.com/jxherc/scoop-bucket
scoop install aivault
```

or with go: `go install github.com/jxherc/aivault/cmd/aivault@latest`. binaries for mac, linux and windows are on the releases page.

the mac binary isn't signed yet. the brew cask clears the quarantine flag after install so macos doesn't block it.

## setup

```sh
aivault init
```

init asks where to keep the vault (default `~/ai-vault`) and which tools to connect, then restart those tools. for scripts:

```sh
aivault init --yes --vault ~/ai-vault --tools claude,codex,qwen
```

> **installed a new ai tool after setup? connect it yourself.** aivault only wires the tools it finds when you run `init`. it doesn't watch for new ones. run `aivault init` again: tools already connected stay connected, and the new one shows up ticked. `aivault doctor` also lists anything installed but not connected.

## bringing in what you already have

if your tools already have rules or skills, init offers to move them into the vault (`--import` in scripts, or `aivault import` later):

- the text of each connected tool's global rules file (`~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, `~/.qwen/QWEN.md` and so on) goes into `me.md`, one section per tool. identical text is only added once. relative `@file` imports get turned into full paths so they still work from the vault
- skill folders from each tool's skills folder (`~/.claude/skills/`, `~/.agents/skills/`, `~/.qwen/skills/`...) move to `skills/`, and a link takes their old place. identical copies count as one skill
- skills that are links to some other folder stay where they are. so does a skill whose name is already taken by a different one

the tool files end up holding only the ai vault block. `aivault remove` puts the old text and skill folders back; the vault keeps its copies. if the rules from all your tools add up past codex's 32 KB limit, init warns you so you can trim `me.md`.

## what's in the vault

```
ai-vault/
  AGENTS.md     rules every tool loads
  me.md         your preferences
  memory/       one .md file per memory
  skills/       <name>/SKILL.md
```

it's plain markdown, so obsidian or any editor works. to use it on more than one machine, put the folder in icloud, dropbox, syncthing or a git repo.

## commands

| | |
| --- | --- |
| `aivault init` | set up the vault and connect tools |
| `aivault doctor` | check everything is still connected |
| `aivault sync` | run after editing the vault (codex keeps a copy of the rules) |
| `aivault import` | move existing rules and skills into the vault |
| `aivault remove [tool...]` | disconnect tools, all of them if none named |
| `aivault search <query>` | search memories and skills |
| `aivault mcp` | the mcp server the tools launch |

## supported tools

checked against each tool's docs or source on 2026-10-19. `~` is your home folder (`%USERPROFILE%` on windows).

| tool | rules go into | mcp server goes into |
| --- | --- | --- |
| Claude Code | `~/.claude/CLAUDE.md` (imports the vault files) | `claude mcp add --scope user` |
| Codex | `~/.codex/AGENTS.md` (copy) | `~/.codex/config.toml` |
| OpenCode | `instructions` in `~/.config/opencode/opencode.json(c)` | same file, `mcp` |
| Gemini CLI | `~/.gemini/GEMINI.md` (imports) | `~/.gemini/settings.json` |
| Kilo Code | `instructions` in `~/.config/kilo/kilo.json(c)` | same file, `mcp` |
| Crush | `options.global_context_paths` in `~/.config/crush/crush.json` | same file, `mcp` |
| Goose | `~/.config/goose/.goosehints` (copy) | `extensions` in `~/.config/goose/config.yaml` |
| Pi | `~/.pi/agent/AGENTS.md` (copy) | `~/.pi/agent/mcp.json` |
| Hermes Agent | `~/.hermes/SOUL.md` (copy) | `mcp_servers` in `~/.hermes/config.yaml` |
| Cline | `~/.cline/rules/ai-vault.md` (copy) | `~/.cline/data/settings/cline_mcp_settings.json` |
| Zed | `~/.config/zed/AGENTS.md` (copy) | `context_servers` in `~/.config/zed/settings.json` |
| Warp | `~/.agents/AGENTS.md` (copy) | `~/.warp/.mcp.json` |
| Open Interpreter | `~/.openinterpreter/AGENTS.md` (copy) | `~/.openinterpreter/config.toml` |
| Aider | `read:` in `~/.aider.conf.yml` | (aider has no mcp) |
| Qwen Code | `~/.qwen/QWEN.md` (imports) | `~/.qwen/settings.json` |
| Kimi Code | `~/.kimi-code/AGENTS.md` (copy) | `~/.kimi-code/mcp.json` |
| DeepSeek Harness | `~/.dsh/AGENTS.md` (copy) | an `insert` patch in `~/.dsh/cordis.patch.yml` |
| ZCode | `~/.zcode/AGENTS.md` (copy) | `mcp.servers` in `~/.zcode/cli/config.json` |
| MiniMax Code | `~/.minimax/AGENTS.md` (copy) | `~/.minimax/mcp.json` |
| Kode | (kode has no global rules file) | `~/.kode.json` |
| Neovate Code | `~/.neovate/AGENTS.md` (copy) | `~/.neovate/config.json` |

skills get linked into `~/.agents/skills/`, which most of these read. claude code, hermes, kode and neovate get their own skills folder linked too.

"imports" means the tool loads the vault files itself, so edits show up on the next session. "copy" means the tool can't import files, so it gets a copy of the rules: run `aivault sync` after editing the vault.

on windows, goose, zed and hermes keep config under `%APPDATA%` or `%LOCALAPPDATA%`. aivault follows that, along with the env vars that move these folders (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `XDG_CONFIG_HOME`, `GEMINI_CLI_HOME`, `QWEN_HOME`, `KIMI_CODE_HOME`, `DSH_HOME`, `HERMES_HOME`, `PI_CODING_AGENT_DIR`, `CLINE_DIR`, `KILO_CONFIG_DIR`, `CRUSH_GLOBAL_CONFIG`, `GOOSE_PATH_ROOT`, `INTERPRETER_HOME`, `MINIMAX_DATA_DIR`, `KODE_CONFIG_DIR`).

some quirks:
- ZCode only loads mcp servers when `features.mcp` is on. aivault turns it on if you never set it. if you set it to `false` yourself, it stays off and `doctor` tells you
- Neovate also reads `~/.claude/CLAUDE.md` by itself, so with Claude Code connected it sees the rules twice
- Crush's license (FSL) is source-available, not open source
- Gemini CLI stopped serving free accounts on 2026-06-18. it still works with an api key
- Codex blocks mcp calls that aren't approved, and a headless run can't ask. aivault marks its own two tools as approved (`default_tools_approval_mode = "approve"`)
- Qwen Code tends to save "remember this" into its own memory (`~/.qwen/memories/`) instead of the vault, even when told not to. it still reads the vault rules and searches the vault fine
- Gemini CLI turns off every mcp server, user-level ones included, in a folder you haven't trusted. the vault rules still load, but `vault_search` only shows up once you trust the folder

left out, and why:
- iFlow CLI shut down (2026-04-17). Roo Code shut down (2026-05-15). Void is archived
- Continue, OpenHands CLI and the old Python Kimi CLI aren't maintained anymore
- Trae Agent only reads a config in the current folder, so there's nothing global to connect
- Copilot CLI, Cursor CLI, Amp, CodeBuddy and Comate aren't open source. Claude Code isn't either, but it's here because so many people use it

## what it changes

only text it marks as its own (or, in json files, its own keys), and it backs up every file first (`~/.aivault/backups/`).

`aivault remove` undoes all of it. if you haven't touched a file since, you get the original back byte for byte. if you have, only the ai vault parts come out. the vault folder itself is never deleted.

## agents get two tools

- `vault_search`: finds memories and skills by keyword
- `vault_remember`: saves one memory, tagged with which tool saved it

there's no delete over mcp on purpose. delete memory files yourself.

## dev

```sh
go test ./...
go run ./cmd/aivault
```

releases: push a tag like `v0.1.0`. the release workflow runs goreleaser, which builds every platform, makes the github release and updates the brew cask and scoop manifest. it needs a `TAP_TOKEN` secret that can write to `jxherc/homebrew-tap` and `jxherc/scoop-bucket`. try it locally with `goreleaser release --snapshot --clean`.

see [SPEC.md](SPEC.md) for the v1 plan.

## license

MIT, see [LICENSE](LICENSE).
