# ai vault v1 spec

one folder of markdown that claude code, codex and opencode all read. a small go cli sets it up, wires each tool to it, and can undo everything.

the user runs `aivault init`, answers a few prompts, restarts their tools. after that a preference saved from claude code shows up in codex, and the reverse.

## v1 scope

in:
- claude code, codex, opencode
- macos, linux, windows
- shared rules file, shared skills, shared memory (search + save)
- clean undo

out (later, maybe):
- cursor, gemini cli, others
- searching past chat transcripts across tools
- a desktop gui (the go core can sit behind a tauri app later)
- any sync service. the vault is a folder; people put it in icloud, dropbox, syncthing or git if they want it on more machines
- accounts, telemetry, embeddings

## vault layout

```
ai-vault/
  AGENTS.md          rules every tool loads
  me.md              the user's preferences, imported by AGENTS.md
  memory/
    <id>.md          one memory per file
  skills/
    <name>/SKILL.md
```

plain markdown so it opens fine in obsidian or any editor. default location is `~/ai-vault`; init lets the user pick another folder.

a memory file:

```md
---
id: 3f9c2a
created: 2026-10-19T14:02:00Z
from: claude-code
tags: [git, commits]
---
lowercase commit titles. no co-author lines.
```

`AGENTS.md` starts with a short default (kept under ~2 KB) that tells the agent two things: call `vault_search` at the start of a task, and call `vault_remember` when the user states a lasting preference or correction. the user owns the file after that; the cli never rewrites it.

## commands

| command | does |
| --- | --- |
| `aivault init` | wizard: pick vault folder, detect tools, pick which to wire, optional import, apply |
| `aivault doctor` | checks each tool still points at the vault and reports what's broken |
| `aivault sync` | refreshes copied content (codex block, windows skill copies) |
| `aivault remove [tool]` | takes the vault out of one tool or all of them |
| `aivault search <query>` | same search the agents get, from the terminal |
| `aivault mcp` | runs the mcp server over stdio. tools launch this, people don't |

init also takes flags so it works in scripts: `aivault init --yes --vault ~/ai-vault --tools claude,codex,opencode`.

the wizard uses charmbracelet/huh. arrow keys and enter, nothing else to learn.

## how each tool gets wired

checked against current docs on 2026-10-19. these locations move, so doctor should re-check them and the table should be re-verified before each release.

| | claude code | codex | opencode |
| --- | --- | --- | --- |
| global rules | `~/.claude/CLAUDE.md` | `$CODEX_HOME/AGENTS.md` (default `~/.codex`) | `~/.config/opencode/opencode.json` |
| how | managed block with an `@` import of the vault's `AGENTS.md` | managed block holding a copy of `AGENTS.md` + `me.md` | add the vault's `AGENTS.md` to the `instructions` array |
| skills | link each skill into `~/.claude/skills/` | link into `~/.agents/skills/` | reads `~/.agents/skills/` already |
| mcp | `claude mcp add --scope user` | `[mcp_servers.ai-vault]` table between `# ai-vault:start` / `# ai-vault:end` in config.toml | add an `mcp` entry in opencode.json |

why codex gets a copy: codex doesn't expand `@path` imports in AGENTS.md, so the text has to be there. `aivault sync` refreshes it. codex also stops reading instructions past `project_doc_max_bytes` (32 KiB default), so sync warns when the copy gets near that.

for claude code, a vault path with spaces needs a backslash before each space in the `@` line. a quoted path doesn't import at all.

claude code's `~/.claude.json` is big and claude writes to it all the time, so that one goes through `claude mcp add`. codex gets a marked toml block instead of `codex mcp add`: undo is just cutting the block, and it works even when the codex cli isn't on PATH. opencode's config can be jsonc, so it's edited with a parser that keeps comments (tailscale/hujson), and only the new keys get formatted.

every mcp entry launches `aivault mcp --vault <path>`. with the path in the command, the server doesn't depend on which home folder or state file the tool starts it with.

### the managed block

```md
<!-- ai-vault:start -->
@/Users/me/ai-vault/AGENTS.md
<!-- ai-vault:end -->
```

rules:
- only text between the markers belongs to us. everything else in the file stays byte-for-byte the same.
- if the file doesn't exist, create it.
- before the first change to any file, copy the original to `~/.aivault/backups/<timestamp>/` and write a manifest.
- `remove` deletes the block. if the file was created by us and is now empty, delete it.
- running init twice changes nothing the second time.

## mcp server

built into the same binary, stdio only. v1 has two tools:

- `vault_search(query, limit?)`: searches `memory/`, `me.md` and skill descriptions. plain word matching with simple ranking, no embeddings. returns the matching text and file paths.
- `vault_remember(text, tags?)`: writes a new file to `memory/`. records which tool saved it.

no delete over mcp. an agent shouldn't be able to wipe memories on its own; deleting stays a cli or file-manager job.

## cross-platform

- resolve home with `os.UserHomeDir()`. on windows `~/.claude` means `%USERPROFILE%\.claude`.
- respect `CODEX_HOME` and `CLAUDE_CONFIG_DIR` when set.
- skill links: symlinks on mac and linux. on windows use directory junctions (no admin needed) and fall back to copying, which `sync` keeps fresh.
- write files with a temp file + rename so a crash never leaves half a config.
- keep the user's line endings when editing a file.

## state

`~/.aivault/config.json` holds the vault path, which tools are wired, and what was changed in each. backups sit next to it. that's all the cli remembers.

## shipping

goreleaser builds mac (arm64, amd64), linux (arm64, amd64) and windows (amd64, arm64). publish to github releases, a homebrew tap and scoop. `go install` works too.

## v1 is done when

- on a clean mac, linux and windows machine with all three tools, init finishes in under a minute
- each tool, asked "what's in my ai vault rules?", answers from the vault
- a memory saved from claude code is found by `vault_search` in codex and opencode
- `aivault remove` puts every touched file back exactly how it was, unless the user edited it outside the block since
- running init twice in a row makes no changes the second time
- doctor catches a missing block, a dead skill link, and a codex `AGENTS.override.md` that hides the global `AGENTS.md`

## open questions

- opencode reads skills from both `~/.claude/skills/` and `~/.agents/skills/`, so it may see each vault skill twice. needs a test: if it breaks, skip the claude link for opencode users or set opencode's claude-compat env var.
- import on first run: pull the user's existing `CLAUDE.md` / `AGENTS.md` into `me.md` as-is, or ask per section? as-is is simpler; i'd start there.
- `aivault` is a working command name. pick the real one before the first release.
