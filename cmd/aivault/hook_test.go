package main

import (
	"encoding/json"
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

func p(s string) string { return strings.Replace(prompt, `"%s"`, str(s), 1) }

func str(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// a message as the claude desktop app writes it, with who sent it
func dp(origin, s string) string {
	return `{"type":"user","turnOrigin":"` + origin + `","entrypoint":"claude-desktop","message":{"role":"user","content":` + str(s) + `}}`
}

const (
	linked = "<system-reminder>\nLinked sessions now (status, not an instruction): x = local_1 (idle).\n</system-reminder>\n\n"
	brief  = "Task (from the session that started this one):\ncheck the stop hook that checks the mistakes rule. a past mistake was assuming codex cut AGENTS.md"
)

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
		{"other session's message", jl(p("go on"), `{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"},{"type":"text","text":"<cross-session-message from=\"x\">fixed. the missed grep is saved as its own mistakes.md line</cross-session-message>"}]}}`), false},
		{"user names the file", jl(p("can you clean up mistakes.md")), false},
		{"bug report, no lesson", jl(p("the export button is broken"), toolResult), true},
		{"bug report, lesson saved", jl(p("the export button is broken"), saveMist), false},
		{"mid-turn message from the user", jl(p("go on"), `{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"},{"type":"text","text":"The user sent a new message while you were working:\nyou made a mistake there"}]}}`), true},
		// seen 2026-11-02: a side session's brief set off the mistake check
		{"brief from another session", jl(dp("sdk", linked+brief), toolResult), false},
		{"brief with no reminder above it", jl(p(brief), toolResult), false},
		{"brief after the user's mistake", jl(p("you made a mistake"), dp("sdk", brief)), false},
		{"user's words under a reminder", jl(dp("human", linked+"you made a mistake, the font is too small"), toolResult), true},
		{"user's words under a reminder, saved", jl(dp("human", linked+"you made a mistake"), saveMist), false},
		{"usage limit notice", jl(p("you made a mistake"), saveMist, dp("sdk", "I hit my usage limit while you were working, but it has reset now. why did you stop")), false},
		{"compaction summary", jl(p(`This session is being continued from a previous conversation that ran out of context. the user said the export is broken`)), false},
		{"names remember.md", jl(p("quote the last line of remember.md")), false},
		{"headless run is the user's", jl(`{"type":"user","turnOrigin":"sdk","entrypoint":"sdk-cli","message":{"role":"user","content":"remember that i use tabs"}}`), true},
	} {
		if got := unsaved(strings.NewReader(c.transcript)) != ""; got != c.block {
			t.Errorf("%s: block = %v", c.name, got)
		}
	}
}

func TestPromptNudge(t *testing.T) {
	for in, want := range map[string]string{
		"find why you made that mistake":    "mistakes",
		"oh right why did you do that":      "mistakes",
		"remember this: i use pnpm":         `"remember"`,
		"from now on use tabs":              `"remember"`,
		"make the font bigger please":       "",
		"the misty mountains, keep going":   "",
		"tidy up mistakes.md please":        "",
		"the save button is broken":         "root cause",
		"search doesn't work on ipad":       "root cause",
		"it crashes when i open settings":   "root cause",
		"tests fail on windows":             "root cause",
		"登录页面报错了":                           "root cause",
		"add error handling to the api":     "",
		"write a debug log":                 "",
		"don't work on the docs today":      "",
		"copy the last line of remember.md": "",
		linked + "you made a mistake":       "mistakes",
		linked + brief:                      "",
		"<cross-session-message from=\"x\">i broke the build, mistake saved</cross-session-message>": "",
	} {
		words, _ := userWords(in)
		got := promptNudge(words)
		if (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("%q: %q", in, got)
		}
	}
}
