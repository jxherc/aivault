# conventions

- only text between `ai-vault:start` / `ai-vault:end` markers is ours. never rewrite the rest of a user's file
- every first write to a file goes through State.write (backup + sha)
- writes are temp file + rename
- lowercase user-facing messages, short
- tests use a fake Env with a temp home. never touch the real home in tests
