# architecture

```
cmd/aivault/   cli + init wizard (main.go), mcp server (mcp.go)
internal/vault the folder: create, memories, search. knows nothing about tools
internal/wire  everything that touches tool config: env paths, state, marked blocks, json/yaml/toml editors, the harness table (harnesses.go, spec.go), skill links, import
```

flow: init -> wire.Apply -> vault.Create, then each tool's Apply, then skill links. state saved after every tool so a crash halfway can still be undone.

state: `~/.aivault/config.json` (vault path, wired tools, files we wrote with sha + backup path, links we made). backups in `~/.aivault/backups/<time>/`.
