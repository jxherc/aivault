package main

import (
	"strings"
	"testing"
)

func jl(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

const (
	prompt     = `{"type":"user","message":{"role":"user","content":"%s"}}`
	toolResult = `{"type":"user","message":{"content":[{"type":"tool_result","content":"mistakes.md: mistakes already made once"},{"type":"text","text":"<system-reminder>mistake rules</system-reminder>"}]}}`
	saveMist   = `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__ai-vault__vault_remember","input":{"text":"x -> y","file":"mistakes"}}]}}`
	saveRem    = `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__ai-vault__vault_remember","input":{"text":"remember: tabs in go"}}]}}`
	queued     = `{"type":"attachment","attachment":{"type":"queued_command","prompt":"why did you make thi mistka, fix it"}}`
)

func p(s string) string { return strings.Replace(prompt, "%s", s, 1) }

func TestStopHook(t *testing.T) {
	for _, c := range []struct {
		name, transcript string
		block            bool
	}{
		{"mistake not saved", jl(p("you made a mistake, the font is too small"), toolResult), true},
		{"mistake saved", jl(p("you made a mistake"), saveMist), false},
		{"saved in an earlier turn only", jl(p("you made a mistake"), saveMist, p("ok now the readme")), false},
		{"queued mid-turn", jl(p("fix the readme"), toolResult, queued), true},
		{"queued then saved", jl(p("fix the readme"), queued, saveMist), false},
		{"tool output mentions mistakes", jl(p("fix the readme"), toolResult), false},
		{"remember not saved", jl(p("remember that i use tabs")), true},
		{"remember saved by prefix", jl(p("remember that i use tabs"), saveRem), false},
		{"other session relays one", jl(p("go on"), `{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"},{"type":"text","text":"<cross-session-message from=\"x\">you made both mistakes</cross-session-message>"}]}}`), true},
	} {
		if got := unsaved(strings.NewReader(c.transcript)) != ""; got != c.block {
			t.Errorf("%s: block = %v", c.name, got)
		}
	}
}

func TestPromptNudge(t *testing.T) {
	for in, want := range map[string]string{
		"find why you made that mistake":  "mistakes",
		"oh right why did you do that":    "mistakes",
		"remember this: i use pnpm":       `"remember"`,
		"from now on use tabs":            `"remember"`,
		"make the font bigger please":     "",
		"the misty mountains, keep going": "",
	} {
		got := promptNudge(in)
		if (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("%q: %q", in, got)
		}
	}
}
