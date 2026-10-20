# components

- `wire.Tool`: Name, Detect, Skills dir, Apply, Remove, Check. add a tool = add one of these to `wire.All`
- `wire.marks`: set/strip/body for marked blocks (markdown and toml comments). keeps crlf files crlf
- `wire.State`: write (with backup) and undo (restore or strip)
- `vault.Search`: word match, tags weigh 3x, prefix match for words over 3 letters
- `wire.Scan` / `wire.Import`: find and move existing rules text and skill folders. FileRec.Moved holds moved text for undo, State.Imported holds moved skill folders
- `wire.spec` + `build`: one table entry per harness (harnesses.go). kinds: rules import/copy, json list, yaml list, json/toml/yaml mcp, raw yaml block (dsh)
- `jsonApply/jsonUndo/jsonMissing`: own one key or a few list strings in a json(c) file. IfMissing ops are only undone if we added them
- `yamlSet/yamlStrip`: marker block under a top-level key, or appended to a top-level list
