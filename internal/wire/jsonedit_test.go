package wire

import (
	"strings"
	"testing"
)

func TestJSONEdit(t *testing.T) {
	ops := []jsonOp{
		{Path: []string{"mcpServers", "ai-vault"}, Val: map[string]any{"command": "/bin/aivault", "args": []string{"mcp"}}},
		{Path: []string{"context", "fileName"}, List: []string{"/v/AGENTS.md"}},
	}
	cases := []string{
		"",
		"{}",
		"{\n  // keep me\n  \"theme\": \"dark\"\n}\n",
		"{\n\t\"mcpServers\": {\n\t\t\"other\": {\"command\": \"x\"}\n\t},\n\t\"context\": {\"fileName\": [\"GEMINI.md\"]},\n}\n",
	}
	for _, orig := range cases {
		out, _, err := jsonApply([]byte(orig), ops)
		if err != nil {
			t.Fatalf("%q: %v", orig, err)
		}
		if bad, err := jsonMissing(out, ops); err != nil || len(bad) != 0 {
			t.Fatalf("%q: missing after apply %v %v\n%s", orig, bad, err, out)
		}
		if strings.Contains(orig, "keep me") && !strings.Contains(string(out), "// keep me") {
			t.Fatalf("comment lost:\n%s", out)
		}
		if strings.Contains(orig, "GEMINI.md") && !strings.Contains(string(out), "GEMINI.md") {
			t.Fatalf("existing list item lost:\n%s", out)
		}
		if strings.Contains(string(out), `":{`) || strings.Contains(string(out), `,"`) || strings.Contains(string(out), "}}") {
			t.Fatalf("new keys squashed onto one line:\n%s", out)
		}
		again, _, _ := jsonApply(out, ops)
		if string(again) != string(out) {
			t.Fatalf("%q: second apply changed it:\n%s\n---\n%s", orig, out, again)
		}
		back, err := jsonUndo(out, ops, nil)
		if err != nil {
			t.Fatal(err)
		}
		if bad, _ := jsonMissing(back, ops); len(bad) != 2 && !strings.Contains(orig, "GEMINI") {
			t.Fatalf("%q: undo left our stuff: %v\n%s", orig, bad, back)
		}
		if strings.Contains(string(back), "ai-vault") || strings.Contains(string(back), "/v/AGENTS.md") {
			t.Fatalf("%q: undo left our stuff:\n%s", orig, back)
		}
	}

	// a changed value gets replaced, not duplicated
	out, added, _ := jsonApply([]byte(`{"mcpServers": {"ai-vault": {"command": "old"}}}`), ops)
	if strings.Count(string(out), "ai-vault") != 1 || !strings.Contains(string(out), "/bin/aivault") || len(added) != 0 {
		t.Fatalf("replace:\n%s", out)
	}

	// IfMissing: set when absent and undone; a user's own value is never touched
	feat := []jsonOp{{Path: []string{"features", "mcp"}, Val: true, IfMissing: true}}
	out, added, _ = jsonApply([]byte(`{}`), feat)
	if len(added) != 1 || !strings.Contains(string(out), "true") {
		t.Fatalf("not added: %s", out)
	}
	if back, _ := jsonUndo(out, feat, added); strings.Contains(string(back), "mcp") {
		t.Fatalf("not undone: %s", back)
	}
	mine := []byte(`{"features": {"mcp": false}}`)
	out, added, _ = jsonApply(mine, feat)
	if string(out) != string(mine) || len(added) != 0 {
		t.Fatalf("user's false got overwritten: %s", out)
	}
	if bad, _ := jsonMissing(mine, feat); len(bad) != 1 {
		t.Fatal("doctor should flag mcp turned off")
	}
	if back, _ := jsonUndo([]byte(`{"features": {"mcp": true}}`), feat, nil); !strings.Contains(string(back), "true") {
		t.Fatal("undo removed a value the user set")
	}
}
