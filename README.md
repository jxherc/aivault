# ai vault

[简体中文](README.zh-Hans.md)

one folder of rules, skills and memory that all your ai coding tools read: claude code, codex, opencode, gemini cli, qwen code, kimi code, deepseek harness and 14 more.

save a preference in claude code, and codex or qwen code knows it the next time you open it. write a skill once and every tool has it.

## install

mac or linux:

```sh
curl -fsSL https://raw.githubusercontent.com/jxherc/aivault/main/install.sh | sh
```

windows (powershell):

```powershell
irm https://raw.githubusercontent.com/jxherc/aivault/main/install.ps1 | iex
```

both download the latest release, check its checksum, put `aivault` on your PATH (`~/.local/bin`, or `%LOCALAPPDATA%\Programs\aivault` on windows) and offer to run setup. `AIVAULT_VERSION=0.2.0` pins a version, `AIVAULT_DIR` picks the folder, `AIVAULT_NO_INIT=1` skips the setup question.

other ways:

| | |
| --- | --- |
| go | `go install github.com/jxherc/aivault/cmd/aivault@latest` |
| by hand | download the file for your computer from the [releases page](https://github.com/jxherc/aivault/releases), like `aivault-macos-apple-silicon.tar.gz` |

`aivault update` updates it in place (it checks the checksum too).

the mac binary isn't signed yet. the install script avoids the quarantine flag, so macos doesn't block it. a file downloaded with a browser needs `xattr -d com.apple.quarantine aivault`.

## setup

```sh
aivault init
```

init asks where to keep the vault (default `~/ai-vault`), which tools to connect, and whether to keep them in sync in the background. then restart those tools and run:

```sh
aivault doctor
```

for scripts:

```sh
aivault init --yes --vault ~/ai-vault --tools claude,codex,qwen --autosync
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
  mistakes.md   mistakes an ai already made, each with the rule that stops a repeat
  *.md          any other file you put here loads too (say remember.md)
  index/        lookup lists, searched line by line (where your files are, what your nicknames mean)
  memory/       one .md file per memory
  skills/       <name>/SKILL.md
```

when you report a bug or point out a mistake, the rules tell the tool to find the root cause, fix every place it applies, and add the lesson to mistakes.md. every tool loads that file, so the same mistake shouldn't come back in any of them.

every `.md` at the top of the vault is part of the rules every tool loads: AGENTS.md, then me.md, then the rest a to z. keep them short, since every tool reads all of them every session. bigger lists belong in `index/`: `vault_search` returns the single matching line from them, with the headings above it, so a 300-line list of your files still answers "my chem module 4 worksheet" with one path.

it's plain markdown, so obsidian or any editor works. to use it on more than one machine, put the folder in icloud, dropbox, syncthing or a git repo.

## commands

| | |
| --- | --- |
| `aivault` | what's connected, what's in the vault and what got saved lately |
| `aivault init` | set up the vault and connect tools |
| `aivault doctor` | check everything (aliases: `status`, `health`) |
| `aivault doctor --fix` | reconnect broken tools and refresh stale copies, then check again |
| `aivault sync` | refresh every tool after editing the vault, and pull in anything new from them (rules text, skills, their own memories) |
| `aivault autosync on\|off\|status` | keep everything in sync in the background, from login on |
| `aivault watch` | the same in the foreground: refresh tools when a rules file changes (`--sync 1h` also runs a full sync every hour) |
| `aivault search <query>` | search memories, skills and `index/` lists |
| `aivault add [--file remember] <text>` | save a memory from the terminal, and add it to a vault file too |
| `aivault open [file]` | open the vault, or one file (`remember`, `memory`, `skills`...) |
| `aivault import` | move existing rules and skills into the vault |
| `aivault remove [tool...]` | disconnect tools, all of them if none named |
| `aivault update` | update aivault itself (`--check` only looks) |
| `aivault completion <shell>` | tab completion for bash, zsh, fish or powershell |
| `aivault mcp` | the mcp server the tools launch |

### home screen

`aivault` on its own:

```
$ aivault
aivault  0.2.0

  vault    ~/ai-vault
  tools    ✓ Claude Code  ✓ Codex  ✓ OpenCode
           not connected: Gemini CLI. aivault init

  memory   229, 12 this week
           claude 140 · codex 61 · opencode 28
  skills   219
  index    3 lists, 105 entries
  rules    6 files, 22 KB, loaded by every tool each session
  synced   2 h ago, autosync on

  saved    ▁▁▃▁▂▁▁▂▁▁▁▁▁▅▁▁▂▇▂▁█▁▆▁▁▁▁▅▁▂▃▁▂▁▁▁▃▁▁▂▁▁▅▃▁▂▂▆▄▁█▁▁▄▁▁
           sep 3                                              today
  latest   2 h    claude  the deploy script needs node 24, not 26
           5 h    codex   school files are in ~/Documents/school
           1 d    claude  short commit titles, no trailers

  aivault help   aivault search <words>   aivault doctor
```

the bar chart is memories saved per day over the last 8 weeks. it only reads local files, so it shows up right away.

### doctor

```
$ aivault doctor
aivault
  · version           0.2.0
  ✓ binary            /Users/me/.local/bin/aivault
  ✓ update            latest

vault
  ✓ folder            /Users/me/ai-vault
  ✓ rules             6 files, 22 KB: AGENTS.md, me.md, dont.md, mistakes.md, portfolio.md, remember.md
  · contents          229 memories, 219 skills, 3 index lists
  ✓ secrets           nothing that looks like a key or password

mcp
  ✓ server            starts, lists both tools, search works (28 ms)

tools
  ✓ Claude Code       rules and mcp in place
  ✗ Codex             ~/.codex/AGENTS.md has an old copy of the vault rules. run aivault sync
  · Gemini CLI        installed but not connected. aivault init connects it
  ✓ skill links       543 links

sync
  · last sync         2 h ago
  · autosync          off. aivault autosync on keeps every tool current by itself

8 ok · 1 problem

aivault doctor --fix reconnects tools and refreshes copies
```

the mcp check is real: doctor starts the server the same way your tools do, lists its tools and runs a search. `--json` prints the same report for scripts, `--quick` skips the mcp test and the update check. it exits non-zero when something is broken.

### autosync

`aivault autosync on` installs a small background job: a launchd agent on mac, a systemd user service on linux, a logon task on windows. it runs `aivault watch --sync 1h`: an edit to a rules file reaches every tool within a few seconds, and once an hour a full sync pulls in what the tools saved in their own memories. the log is `~/.aivault/watch.log` (`journalctl --user -u aivault` on linux). `aivault autosync off` removes it.

### shell completion

```sh
aivault completion zsh > "${fpath[1]}/_aivault"       # zsh
aivault completion bash > ~/.local/share/bash-completion/completions/aivault
aivault completion fish > ~/.config/fish/completions/aivault.fish
aivault completion powershell >> $PROFILE              # powershell
```

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
- tools that get a copy of the rules (codex, pi, hermes, kimi, dsh and others marked "copy" above) only see your own edits after `aivault sync`, unless autosync is on. a line added with `vault_remember` and `file` updates their copies right away
- every top-level vault file goes into every tool's context. a big one costs tokens in every session and can push a tool over its size limit. Hermes and DeepSeek Harness have one (see below), and `doctor` warns before either cuts the rules
- tools load their rules when a session starts. restart them after `init` or `sync`
- a tool you install after setup isn't connected until you run `aivault init` again
- a memory a tool saves in its own memory reaches the vault at the next sync, not right away. with autosync on that's within the hour. without it, nothing runs sync for you
- search is plain keyword matching, not semantic search. a memory worded differently from the question can be missed
- `vault_remember` refuses text that looks like a password or key, but that check only catches obvious shapes. the vault is plain markdown, and it syncs wherever you put it (icloud, dropbox, git), so don't keep keys or passwords in it

**if you already have a setup**
- check whether another installer or dotfiles script manages the same files (`~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md` and so on). aivault adds its own marked block, and a script that guards those files may refuse its next run or overwrite the block. retire it or exclude those files first
- back up first. `aivault remove` undoes aivault's own changes, but it can't know what another tool did before or after
- `init --import` moves rules text and skill folders. it leaves skills alone when they're links to somewhere else, or when a different skill already has the same name. read what it prints

**per tool**
- **Claude Code** also gets two hooks in `~/.claude/settings.json`. when your message flags a mistake, reports a bug or says "remember", one puts a reminder right next to it. the other won't let the turn end until `vault_remember` ran with the right file. it asks once, never twice in a row, and an explicit "don't call any tools" still wins. your own hooks stay as they are, and `aivault remove` takes ours out. other tools only have the rules text
- **OpenCode**: if your `opencode.json` has a skill allowlist (`"permission": {"skill": {"*": "deny", ...}}`), vault skills not on that list stay blocked. aivault doesn't edit your permission rules: `init` and `doctor` say how many vault skills are blocked, name some, and show the line to add. OpenCode also reads `~/.claude/skills` and `~/.agents/skills`, so it can list a skill twice
- **Codex**: it blocks mcp calls nobody approved, and a headless run can't ask, so aivault pre-approves its own two tools (`default_tools_approval_mode = "approve"`). Codex reads `~/.codex/AGENTS.md` whole. `project_doc_max_bytes` only caps the AGENTS.md files in your projects
- **Gemini CLI**: every mcp server is off in a folder you haven't trusted, user-level ones included. the vault rules still load, but `vault_search` only shows up once you trust the folder. if you route it through a proxy, Gemini CLI also calls its own built-in model names, which the proxy has to know
- **Qwen Code**: "remember this" tends to land in Qwen's own memory (`~/.qwen/memories/`) instead of the vault, even when the rules say otherwise. `aivault sync` copies those memories into the vault, so they reach your other tools after the next sync, not right away
- **Hermes Agent**: `SOUL.md` is its only global file, so the vault rules go there next to its persona. Hermes drops the middle of `SOUL.md` past `context_file_max_chars`, which is 20,000 characters on small models. when the rules get near that, `sync` sets `context_file_max_chars` at the top of `~/.hermes/config.yaml`, and sets it again if something removes it. a value you set yourself stays, and `doctor` says if it's too small
- **ZCode** only loads mcp servers when `features.mcp` is on. aivault turns it on only if you never set it. your own `false` stays, and `doctor` tells you
- **Neovate** also reads `~/.claude/CLAUDE.md` by itself, so with Claude Code connected it sees the rules twice
- **DeepSeek Harness**: `~/.dsh/cordis.patch.yml` must stay a yaml list. aivault refuses to edit it otherwise. dsh fits `~/.dsh/AGENTS.md` and every project AGENTS.md into 64 KB together, so big vault rules leave less room for project files
- **Crush**'s license (FSL) is source-available, not open source
- **Gemini CLI** stopped serving free accounts on 2026-06-18. it still works with an api key
- a vault path with spaces (like icloud's `Mobile Documents`) works for Claude Code's imports. Gemini and Qwen get a copy instead of an import, so run `sync` after edits there too
- on windows, skill links fall back to copies when links aren't allowed. run `sync` after changing skills
- on windows, autosync starts through a hidden powershell window at logon. it can flash for a moment

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
