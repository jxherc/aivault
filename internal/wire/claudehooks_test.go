package wire

import (
	"strings"
	"testing"
)

func TestClaudeHooks(t *testing.T) {
	theirs := `{
  "model": "opus",
  "hooks": {
    "Stop": [
      {
        "hooks": [{"type": "command", "command": "clawdboard hook-sync", "async": true}]
      }
    ]
  }
}
`
	for _, orig := range []string{"", "{}\n", theirs} {
		out, err := hooksApply([]byte(orig), "/u/bin/aivault")
		if err != nil {
			t.Fatalf("%q: %v", orig, err)
		}
		s := string(out)
		if !strings.Contains(s, `"\"/u/bin/aivault\" hook prompt"`) || !strings.Contains(s, `"\"/u/bin/aivault\" hook stop"`) {
			t.Fatalf("%q: hooks missing:\n%s", orig, s)
		}
		if len(hooksMissing(out, "/u/bin/aivault")) != 0 {
			t.Fatalf("doctor unhappy:\n%s", s)
		}
		again, _ := hooksApply(out, "/u/bin/aivault")
		if string(again) != s {
			t.Fatalf("second apply changed it:\n%s", again)
		}
		// binary moved: same entries, new path, no duplicates
		moved, _ := hooksApply(out, "/opt/aivault")
		if strings.Contains(string(moved), "/u/bin") || strings.Count(string(moved), "hook stop") != 1 {
			t.Fatalf("move:\n%s", moved)
		}
		if len(hooksMissing(out, "/opt/aivault")) != 2 {
			t.Fatal("doctor should see a stale path")
		}
		back, err := hooksUndo(moved)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(back), "aivault") {
			t.Fatalf("undo left ours:\n%s", back)
		}
		if orig == theirs && (!strings.Contains(string(back), "clawdboard") || !strings.Contains(string(back), `"model": "opus"`)) {
			t.Fatalf("undo lost the user's stuff:\n%s", back)
		}
		if orig != theirs && strings.Contains(string(back), `"hooks"`) {
			t.Fatalf("undo left an empty hooks object:\n%s", back)
		}
	}
}
