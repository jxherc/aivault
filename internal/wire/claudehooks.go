package wire

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// claude code hooks in ~/.claude/settings.json: `aivault hook prompt` on
// UserPromptSubmit and `aivault hook stop` on Stop. they nudge, then insist,
// that a mistake or a "remember this" gets saved. other hooks stay untouched

var claudeHookEvents = [][2]string{{"UserPromptSubmit", "prompt"}, {"Stop", "stop"}}

func claudeSettings(e *Env) string { return filepath.Join(e.ClaudeDir, "settings.json") }

func hookCmd(bin, arg string) string { return `"` + bin + `" hook ` + arg }

// a command we put there, from this binary or an older path of it
func ourHook(cmd, arg string) bool {
	return strings.Contains(cmd, "aivault") && strings.HasSuffix(cmd, " hook "+arg)
}

// index of the group holding our hook, and its command
func findHook(groups []any, arg string) (int, string) {
	for i, g := range groups {
		m, _ := g.(map[string]any)
		hs, _ := m["hooks"].([]any)
		for _, h := range hs {
			hm, _ := h.(map[string]any)
			if c, _ := hm["command"].(string); ourHook(c, arg) {
				return i, c
			}
		}
	}
	return -1, ""
}

func hooksApply(b []byte, bin string) ([]byte, error) {
	v, view, err := parseJSON(b)
	if err != nil {
		return nil, err
	}
	for _, ev := range claudeHookEvents {
		cur, have := walk(view, []string{"hooks", ev[0]})
		groups, _ := cur.([]any)
		want := hookCmd(bin, ev[1])
		i, c := findHook(groups, ev[1])
		if c == want {
			continue
		}
		group, _ := json.Marshal(map[string]any{"hooks": []any{map[string]any{"type": "command", "command": want, "timeout": 10}}})
		switch {
		case i >= 0: // the binary moved: point the old entry at the new one
			err = patchOne(&v, "replace", fmt.Sprintf("/hooks/%s/%d", ev[0], i), string(group))
		case have == 2:
			err = patchOne(&v, "add", "/hooks/"+ev[0]+"/-", string(group))
		case have == 1:
			err = patchOne(&v, "add", "/hooks/"+ev[0], "["+string(group)+"]")
		default:
			err = patchOne(&v, "add", "/hooks", `{"`+ev[0]+`":[`+string(group)+`]}`)
		}
		if err != nil {
			return nil, err
		}
		fixPath(&v, []string{"hooks", ev[0]}, "  ")
		_, view, _ = parseJSON(v.Pack())
	}
	return v.Pack(), nil
}

func hooksUndo(b []byte) ([]byte, error) {
	v, view, err := parseJSON(b)
	if err != nil {
		return nil, err
	}
	for _, ev := range claudeHookEvents {
		cur, _ := walk(view, []string{"hooks", ev[0]})
		groups, _ := cur.([]any)
		for i, _ := findHook(groups, ev[1]); i >= 0; i, _ = findHook(groups, ev[1]) {
			if err := patchOne(&v, "remove", fmt.Sprintf("/hooks/%s/%d", ev[0], i), ""); err != nil {
				return nil, err
			}
			groups = append(groups[:i:i], groups[i+1:]...)
		}
		if cur != nil && len(groups) == 0 {
			patchOne(&v, "remove", "/hooks/"+ev[0], "")
		}
		_, view, _ = parseJSON(v.Pack())
	}
	if h, ok := view["hooks"].(map[string]any); ok && len(h) == 0 {
		patchOne(&v, "remove", "/hooks", "")
	}
	return v.Pack(), nil
}

func hooksMissing(b []byte, bin string) []string {
	_, view, err := parseJSON(b)
	if err != nil {
		return []string{"settings.json can't be parsed: " + err.Error()}
	}
	var bad []string
	for _, ev := range claudeHookEvents {
		cur, _ := walk(view, []string{"hooks", ev[0]})
		groups, _ := cur.([]any)
		if _, c := findHook(groups, ev[1]); c != hookCmd(bin, ev[1]) {
			bad = append(bad, "settings.json has no up-to-date "+ev[0]+" hook. run `aivault sync`")
		}
	}
	return bad
}
