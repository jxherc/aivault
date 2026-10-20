# ai vault

one folder of rules, skills and memory that claude code, codex and opencode all read.

save a preference in claude code, and codex knows it the next time you open it. write a skill once and every tool has it.

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
aivault init --yes --vault ~/ai-vault --tools claude,codex,opencode
```

## bringing in what you already have

if your tools already have rules or skills, init offers to move them into the vault (`--import` in scripts, or `aivault import` later):

- the text of `~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md` and `~/.config/opencode/AGENTS.md` goes into `me.md`, one section per tool. identical text is only added once. relative `@file` imports get turned into full paths so they still work from the vault
- skill folders from `~/.claude/skills/`, `~/.agents/skills/` and `~/.config/opencode/skills/` move to `skills/`, and a link takes their old place. identical copies count as one skill
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

## what it changes

only text it marks as its own, and it backs up every file first (`~/.aivault/backups/`).

- claude code: a marked block in `~/.claude/CLAUDE.md` that imports the vault files, plus `claude mcp add --scope user ai-vault`
- codex: a marked block in `~/.codex/AGENTS.md` holding a copy of the rules, and a marked `[mcp_servers.ai-vault]` table in `~/.codex/config.toml`
- opencode: the vault files in `instructions` and an `mcp.ai-vault` entry in `~/.config/opencode/opencode.json(c)`, comments kept
- skills: linked into `~/.claude/skills/` and `~/.agents/skills/` (codex and opencode both read the second)

`aivault remove` undoes all of it. if you haven't touched a file since, you get the original back byte for byte. if you have, only the ai vault parts come out. the vault folder itself is never deleted.

`CLAUDE_CONFIG_DIR`, `CODEX_HOME` and `XDG_CONFIG_HOME` are respected.

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
