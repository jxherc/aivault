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

if your tools already have rules, skills or their own memories, init offers to pull them into the vault (`--import` in scripts). after that, **`aivault sync` does it every time**: anything the tools gained since the last sync ends up in the vault too.

- **rules text**: whatever is in a tool's global rules file outside the ai vault block (`~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, `~/.qwen/QWEN.md` and so on) moves into `me.md`, one section per tool. identical text is only added once. relative `@file` imports get turned into full paths so they still work from the vault. this also catches notes a tool appends to its own rules file
- **skills**: new skill folders in a tool's skills folder (`~/.claude/skills/`, `~/.agents/skills/`, `~/.qwen/skills/`...) move to `skills/`, and a link takes their old place. identical copies count as one skill. skills that are links to some other folder stay where they are, and so does a skill whose name is already taken by a different one
- **the tools' own memories** get copied (not moved) into `memory/`, so every tool can search them:
  - Claude Code's auto memory (`~/.claude/projects/*/memory/`), tagged with the project
  - Codex's consolidated memory (`~/.codex/memories/MEMORY.md` and `memory_summary.md`), one memory per task group or section. its raw logs are left out
  - Qwen Code's `~/.qwen/memories/`
  - Hermes Agent's `~/.hermes/memories/MEMORY.md` and `USER.md`
  - DeepSeek Harness's `~/.dsh/lessons.md`, one memory per bullet

  each copy has a fixed id, so when a tool rewrites a memory the vault copy updates in place instead of piling up duplicates. the tool keeps its own memory exactly as it was
- **memories that look like they hold a password or key are left out**, and sync lists the files so you can copy what you want by hand. the check is a pattern match, so it can miss a secret or flag something harmless

the tool files end up holding only the ai vault block. `aivault remove` puts the old rules text and skill folders back. the vault keeps its copies, including synced memories.

## what's in the vault

```
ai-vault/
  AGENTS.md     rules every tool loads
  me.md         your preferences
  *.md          any other file you put here loads too (say remember.md, mistakes.md)
  index/        lookup lists, searched line by line (where your files are, what your nicknames mean)
  memory/       one .md file per memory
  skills/       <name>/SKILL.md
```

every `.md` at the top of the vault is part of the rules every tool loads: AGENTS.md, then me.md, then the rest a to z. keep them short, since every tool reads all of them every session. bigger lists belong in `index/`: `vault_search` returns the single matching line from them, with the headings above it, so a 300-line list of your files still answers "my chem module 4 worksheet" with one path.

it's plain markdown, so obsidian or any editor works. to use it on more than one machine, put the folder in icloud, dropbox, syncthing or a git repo.

## commands

| | |
| --- | --- |
| `aivault init` | set up the vault and connect tools |
| `aivault doctor` | check everything is still connected |
| `aivault sync` | refresh every tool after editing the vault, and pull in anything new from them (rules text, skills, their own memories) |
| `aivault import` | move existing rules and skills into the vault |
| `aivault remove [tool...]` | disconnect tools, all of them if none named |
| `aivault search <query>` | search memories, skills and `index/` lists |
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

left out, and why:
- iFlow CLI shut down (2026-04-17). Roo Code shut down (2026-05-15). Void is archived
- Continue, OpenHands CLI and the old Python Kimi CLI aren't maintained anymore
- Trae Agent only reads a config in the current folder, so there's nothing global to connect
- Copilot CLI, Cursor CLI, Amp, CodeBuddy and Comate aren't open source. Claude Code isn't either, but it's here because so many people use it

## precautions

aivault wires the vault in. it can't make a model use it. read this before you rely on it.

**nothing here is guaranteed**
- every connected tool gets the vault rules and the two vault tools, but the model decides when to call `vault_search` and `vault_remember`. the default rules push hard (search before answering anything about you, save whenever you say "remember"), and most models follow them, but some answers will skip the vault
- tools that get a copy of the rules (codex, pi, hermes, kimi, dsh and others marked "copy" above) only see your own edits after `aivault sync`. a line added with `vault_remember` and `file` updates their copies right away
- every top-level vault file goes into every tool's context. a big one costs tokens in every session and can push a tool over its size limit (Codex's is 32 KB by default). `doctor` warns about Codex
- tools load their rules when a session starts. restart them after `init` or `sync`
- a tool you install after setup isn't connected until you run `aivault init` again
- a memory a tool saves in its own memory reaches the vault at the next `aivault sync`, not right away. nothing runs sync for you
- search is plain keyword matching, not semantic search. a memory worded differently from the question can be missed
- `vault_remember` refuses text that looks like a password or key, but that check only catches obvious shapes. the vault is plain markdown, and it syncs wherever you put it (icloud, dropbox, git), so don't keep keys or passwords in it

**if you already have a setup**
- check whether another installer or dotfiles script manages the same files (`~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md` and so on). aivault adds its own marked block, and a script that guards those files may refuse its next run or overwrite the block. retire it or exclude those files first
- back up first. `aivault remove` undoes aivault's own changes, but it can't know what another tool did before or after
- `init --import` moves rules text and skill folders. it leaves skills alone when they're links to somewhere else, or when a different skill already has the same name. read what it prints

**per tool**
- **OpenCode**: if your `opencode.json` has a skill allowlist (`"permission": {"skill": {"*": "deny", ...}}`), vault skills not on that list stay blocked. aivault doesn't edit your permission rules: `init` and `doctor` say how many vault skills are blocked, name some, and show the line to add. OpenCode also reads `~/.claude/skills` and `~/.agents/skills`, so it can list a skill twice
- **Codex**: it blocks mcp calls nobody approved, and a headless run can't ask, so aivault pre-approves its own two tools (`default_tools_approval_mode = "approve"`). Codex also stops reading instructions at 32 KB. if your rules are bigger, raise `project_doc_max_bytes` in `~/.codex/config.toml`. `doctor` follows that setting
- **Gemini CLI**: every mcp server is off in a folder you haven't trusted, user-level ones included. the vault rules still load, but `vault_search` only shows up once you trust the folder. if you route it through a proxy, Gemini CLI also calls its own built-in model names, which the proxy has to know
- **Qwen Code**: "remember this" tends to land in Qwen's own memory (`~/.qwen/memories/`) instead of the vault, even when the rules say otherwise. `aivault sync` copies those memories into the vault, so they reach your other tools after the next sync, not right away
- **Hermes Agent**: `SOUL.md` is its only global file, so the vault rules go there next to its persona
- **ZCode** only loads mcp servers when `features.mcp` is on. aivault turns it on only if you never set it. your own `false` stays, and `doctor` tells you
- **Neovate** also reads `~/.claude/CLAUDE.md` by itself, so with Claude Code connected it sees the rules twice
- **DeepSeek Harness**: `~/.dsh/cordis.patch.yml` must stay a yaml list. aivault refuses to edit it otherwise
- **Crush**'s license (FSL) is source-available, not open source
- **Gemini CLI** stopped serving free accounts on 2026-06-18. it still works with an api key
- a vault path with spaces (like icloud's `Mobile Documents`) works for Claude Code's imports. Gemini and Qwen get a copy instead of an import, so run `sync` after edits there too
- on windows, skill links fall back to copies when links aren't allowed. run `sync` after changing skills

**what was tested for real** (2026-10-20, prompts that never mention the vault): Claude Code, Codex, OpenCode, Pi and DeepSeek Harness each answered from the vault rules, found a fact that only existed in a vault memory, and saved a new memory. Qwen Code passed the first two, and what it saved to its own memory showed up in the vault after a sync. Claude Code also found a memory that had only come from Hermes's own memory. the other tools are checked by unit tests and their own `mcp list` where installed, not by live model runs

## what it changes

only text it marks as its own (or, in json files, its own keys), and it backs up every file first (`~/.aivault/backups/`).

`aivault remove` undoes all of it. if you haven't touched a file since, you get the original back byte for byte. if you have, only the ai vault parts come out. the vault folder itself is never deleted.

## agents get two tools

- `vault_search`: finds memories, skills and `index/` lines by keyword
- `vault_remember`: saves one memory, tagged with which tool saved it. with `file` (say `remember` or `mistakes`), it also adds the line to that vault file, top level or `index/`. the file has to exist already, so a typo can't make a new file every tool then loads. a tool that can't pass `file` can start the text with the file name instead (`remember: ...`). it refuses text that looks like a password or key

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
