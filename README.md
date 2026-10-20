# ai vault

one folder of rules, skills and memory that claude code, codex and opencode all read.

save a preference in claude code, and codex knows it the next time you open it. write a skill once and every tool has it.

## setup

```sh
go build -o aivault .
./aivault init
```

init asks where to keep the vault (default `~/ai-vault`) and which tools to connect, then restart those tools. for scripts:

```sh
aivault init --yes --vault ~/ai-vault --tools claude,codex,opencode
```

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
```

see [SPEC.md](SPEC.md) for the v1 plan.
