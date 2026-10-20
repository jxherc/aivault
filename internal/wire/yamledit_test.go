package wire

import (
	"strings"
	"testing"
)

func TestYAMLEdit(t *testing.T) {
	entry := yamlEntry("ai-vault", []ykv{{"command", "/bin/aivault"}, {"args", []string{"mcp", "--vault", "/v"}}})
	cases := []string{
		"",
		"model: x\n",
		"model: x\nmcp_servers: {}\napprovals:\n  a: false\n",
		"model: x\nmcp_servers:\n    other:\n        command: y\nlast: 1\n",
		"a: 1\r\nmcp_servers:\r\n  other: {command: y}\r\n",
		"mcp_servers: # my servers\n\n  # a comment\n  other:\n    command: y\n",
	}
	for _, orig := range cases {
		out, line, err := yamlSet([]byte(orig), "mcp_servers", entry)
		if err != nil {
			t.Fatalf("%q: %v", orig, err)
		}
		o := string(out)
		if !yamlHas(out) || !strings.Contains(o, `command: "/bin/aivault"`) {
			t.Fatalf("%q: no entry:\n%s", orig, o)
		}
		if strings.Count(o, "mcp_servers:") != 1 {
			t.Fatalf("%q: key duplicated:\n%s", orig, o)
		}
		if strings.Contains(orig, "\r\n") && strings.Count(o, "\n") != strings.Count(o, "\r\n") {
			t.Fatalf("crlf lost: %q", o)
		}
		if strings.Contains(orig, "    other:") && !strings.Contains(o, "    ai-vault:") {
			t.Fatalf("didn't follow the file's indent:\n%s", o)
		}
		again, _, _ := yamlSet(out, "mcp_servers", entry)
		if string(again) != o {
			t.Fatalf("%q: second set changed it:\n%s\n---\n%s", orig, o, again)
		}
		if back := yamlStrip(out, "mcp_servers", line); string(back) != orig {
			t.Fatalf("%q: strip gave %q", orig, back)
		}
	}
	if _, _, err := yamlSet([]byte("mcp_servers: {a: {command: b}}\n"), "mcp_servers", entry); err == nil {
		t.Fatal("non-empty flow map should be refused")
	}

	items := yamlItems([]string{"/v/AGENTS.md", "/v/me.md"})
	for _, orig := range []string{"", "model: x\n", "read: []\n", "read:\n- CONVENTIONS.md\nmodel: x\n", "read:\n  - a.md\n"} {
		out, line, err := yamlSet([]byte(orig), "read", items)
		if err != nil {
			t.Fatalf("%q: %v", orig, err)
		}
		if strings.Contains(orig, "\n- CONV") && !strings.Contains(string(out), "\n- \"/v/AGENTS.md\"") {
			t.Fatalf("list items not flush with the existing ones:\n%s", out)
		}
		if back := yamlStrip(out, "read", line); string(back) != orig {
			t.Fatalf("%q: strip gave %q", orig, back)
		}
	}
	if _, _, err := yamlSet([]byte("read: one.md\n"), "read", items); err == nil {
		t.Fatal("a single-string read: should be refused")
	}

	top := []string{"- insert:", "    - id: mcp-ai-vault", "      config:", "        serverName: ai-vault"}
	for _, orig := range []string{"", "[]\n", "# patches\n[]\n", "- insert:\n    - id: other\n"} {
		out, line, err := yamlSet([]byte(orig), "", top)
		if err != nil {
			t.Fatalf("%q: %v", orig, err)
		}
		if strings.Contains(string(out), "[]") {
			t.Fatalf("%q: empty list left in:\n%s", orig, out)
		}
		again, _, _ := yamlSet(out, "", top)
		if string(again) != string(out) {
			t.Fatalf("%q: second set changed it", orig)
		}
		if back := yamlStrip(out, "", line); string(back) != orig {
			t.Fatalf("%q: strip gave %q", orig, back)
		}
	}
	if _, _, err := yamlSet([]byte("a: 1\n"), "", top); err == nil {
		t.Fatal("a map file should be refused")
	}
}
