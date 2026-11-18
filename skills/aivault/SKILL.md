---
name: aivault
description: set up and use aivault, one folder of rules, skills and memory shared by claude code, codex, opencode, gemini cli, qwen code and 16 more coding agents. use when the user wants the same rules, skills or memories in every ai tool, asks to "remember" something for all their tools, wants to share a skill across tools, or has aivault installed and something in the vault needs saving, finding or fixing.
---

# aivault

aivault keeps one plain markdown folder (default `~/ai-vault`) and wires it into every coding agent on the machine: rules files, skills folders and an mcp server with two tools, `vault_search` and `vault_remember`.

source and docs: https://github.com/jxherc/aivault (MIT, free)

## check first

run `aivault doctor --quick`. if the command is missing, aivault isn't installed: offer to install it. if it prints problems, run `aivault doctor --fix`, then `aivault doctor` again.

## install

ask before running an installer. mac or linux:

```sh
curl -fsSL https://raw.githubusercontent.com/jxherc/aivault/main/install.sh | sh
```

windows (powershell):

```powershell
irm https://raw.githubusercontent.com/jxherc/aivault/main/install.ps1 | iex
```

or `go install github.com/jxherc/aivault/cmd/aivault@latest`.

then set it up. interactive: `aivault init`. non-interactive (you're running it, so use this one):

```sh
aivault init --yes --vault ~/ai-vault --tools claude,codex,opencode --import --autosync
```

pick `--tools` from what the user actually uses. `--import` moves their existing rules and skills into the vault. `aivault remove` undoes everything. tell the user to restart their tools afterwards: rules load when a session starts.

## the vault

```
ai-vault/
  AGENTS.md     rules every tool loads
  me.md         the user's preferences
  mistakes.md   mistakes an ai already made, each with the rule that stops a repeat
  *.md          every other top-level .md loads too
  index/        long lookup lists, searched line by line, not loaded
  memory/       one .md file per memory
  skills/       <name>/SKILL.md, linked into every tool
```

every top-level `.md` goes into every tool's context each session. keep them short. long lists (file locations, nicknames, servers) belong in `index/`.

## saving things

- the user says "remember ...": call `vault_remember`. if a vault file fits (`remember`, `mistakes`, `me`), pass it as `file` so the line also goes into that file. the file must already exist
- the user points out a mistake: fix the root cause everywhere it applies, then save the lesson with `file: mistakes`
- no mcp in this session: `aivault add --file remember "the deploy script needs node 24"`
- never save passwords, keys or tokens. the vault syncs wherever the user keeps it

## finding things

before answering anything about the user, their projects, servers or preferences, call `vault_search` (or `aivault search <words>`). it's keyword search, not semantic: try a second wording if the first finds nothing.

## sharing a skill

put it at `~/ai-vault/skills/<name>/SKILL.md` with `name` and `description` in the frontmatter, then run `aivault sync`. every connected tool gets a link to it.

## after editing the vault by hand

run `aivault sync`. tools that import the vault (claude code, gemini cli, qwen code) see edits next session anyway. tools that get a copy (codex, pi, hermes, kimi and others) only see them after a sync, unless `aivault autosync on` is set.

## don't

- don't edit text between `ai-vault:start` and `ai-vault:end` markers in a tool's own rules file. edit the vault and sync
- don't delete memory files without the user asking. there's no delete over mcp on purpose
