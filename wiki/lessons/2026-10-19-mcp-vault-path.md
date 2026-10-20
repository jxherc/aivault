# put the vault path in the mcp command

opencode launched `aivault mcp` with a different HOME than init used, so the server couldn't find ~/.aivault/config.json and died ("connection closed"). fix: every mcp entry passes `--vault <path>`.
