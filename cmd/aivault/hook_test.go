package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// seen 2026-11-05: a unit worker's report ("teacher key corrections", "wrong")
// blocked turns the user never reported a bug in. shapes copied from real
// desktop transcripts
const workerReport = "<task-notification>\n<task-id>a2e071a898cb45691</task-id>\n<tool-use-id>toolu_013rSwCEJ6zMq5ZaFn8sG2bV</tool-use-id>\n<output-file>/private/tmp/claude-501/x/tasks/a2e071a898cb45691.output</output-file>\n<status>completed</status>\n<summary>unit 2 worker done</summary>\n<result>teacher key corrections: q4 was wrong, the key is broken on q7</result>\n</task-notification>"

func TestStopHookNotifications(t *testing.T) {
	qn := `{"type":"attachment","attachment":{"type":"queued_command","commandMode":"task-notification","origin":{"kind":"task-notification","producer":"session-task"},"prompt":` + str(workerReport) + `}}`
	qh := `{"type":"attachment","attachment":{"type":"queued_command","commandMode":"prompt","origin":{"kind":"human"},"humanTurn":true,"prompt":"the export button is broken"}}`
	qp := `{"type":"attachment","attachment":{"type":"queued_command","commandMode":"prompt","origin":{"kind":"peer","from":"local_1"},"prompt":"<cross-session-message from=\"x\">the build is broken</cross-session-message>"}}`
	for _, c := range []struct {
		name, transcript string
		block            bool
	}{
		{"report starts a turn", jl(dp("task_notification", workerReport), toolResult), false},
		{"report queued mid-turn", jl(dp("human", "fix the readme"), toolResult, qn), false},
		{"report under a reminder", jl(dp("task_notification", linked+workerReport)), false},
		{"system notice", jl(dp("task_notification", "[SYSTEM NOTIFICATION - NOT USER INPUT]\nthe build failed")), false},
		{"report ends an earlier flagged turn", jl(dp("human", "the export is broken"), dp("task_notification", workerReport)), false},
		{"peer message queued", jl(dp("human", "fix the readme"), qp), false},
		{"owner's queued message", jl(dp("human", "fix the readme"), toolResult, qh), true},
		{"owner's real message under a reminder", jl(dp("human", linked+"the export button is broken"), toolResult), true},
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

// runs `aivault hook <name>` the way claude code does: json on stdin, json
// (or nothing) on stdout. returns the additionalContext it handed over
func runHook(t *testing.T, name string, in map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	b, _ := json.Marshal(in)
	inF := filepath.Join(dir, "in")
	os.WriteFile(inF, b, 0o644)
	stdin, _ := os.Open(inF)
	stdout, _ := os.Create(filepath.Join(dir, "out"))
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = stdin, stdout
	err := cmdHook([]string{name})
	os.Stdin, os.Stdout = oldIn, oldOut
	stdin.Close()
	stdout.Close()
	if err != nil {
		t.Fatalf("hook %s: %v", name, err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "out"))
	if len(out) == 0 {
		return ""
	}
	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("hook %s printed %q", name, out)
	}
	want := map[string]string{"prompt": "UserPromptSubmit", "subagent": "SubagentStart"}[name]
	if got.HookSpecificOutput.HookEventName != want {
		t.Fatalf("hook %s answered for %q", name, got.HookSpecificOutput.HookEventName)
	}
	return got.HookSpecificOutput.AdditionalContext
}

// claude code reads the vault rules once per session and gives every worker
// the same copy. seen 2026-11-02: a 15 hour lead and its 46 workers never
// got 36 lessons saved during the run
// a fake home with a vault set up, and a session transcript in it
type rulesHome struct {
	t           *testing.T
	home, v, tr string
}

func newRulesHome(t *testing.T) *rulesHome {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	h := &rulesHome{t, home, filepath.Join(home, "vault"), filepath.Join(home, "session.jsonl")}
	os.MkdirAll(h.v, 0o755)
	os.MkdirAll(filepath.Join(home, ".aivault"), 0o755)
	cfg, _ := json.Marshal(map[string]string{"vault": h.v})
	os.WriteFile(filepath.Join(home, ".aivault", "config.json"), cfg, 0o644)
	for n, c := range map[string]string{
		"AGENTS.md":   "# ai vault\n\n- search first\n",
		"me.md":       "# me\n\n- lowercase\n",
		"mistakes.md": "# mistakes\n\n- pkill took down a server. -> stop only your own pid\n",
		"remember.md": "# remember\n\n- no em dashes\n",
	} {
		os.WriteFile(filepath.Join(h.v, n), []byte(c), 0o644)
	}
	return h
}

var allRules = []string{"AGENTS.md", "me.md", "mistakes.md", "remember.md"}

// what claude code attaches when it loads the rules: each file without its last newline
func (h *rulesHome) loaded(names ...string) string {
	var fs []map[string]string
	for _, n := range names {
		c, _ := os.ReadFile(filepath.Join(h.v, n))
		fs = append(fs, map[string]string{"path": filepath.Join(h.v, n), "type": "User", "content": strings.TrimRight(string(c), "\n")})
	}
	fs = append(fs, map[string]string{"path": filepath.Join(h.home, "proj", "CLAUDE.md"), "type": "Project", "content": "- project rule"})
	b, _ := json.Marshal(map[string]any{"type": "attachment", "attachment": map[string]any{"type": "instructions", "files": fs}})
	return string(b)
}

// how claude code keeps what a hook said
func handed(ctx string) string {
	b, _ := json.Marshal(map[string]any{"type": "attachment", "attachment": map[string]any{"type": "hook_additional_context", "hookEvent": "UserPromptSubmit", "content": []string{ctx}}})
	return string(b)
}

func (h *rulesHome) write(lines ...string) { os.WriteFile(h.tr, []byte(jl(lines...)), 0o644) }

func (h *rulesHome) add(n, line string) {
	f, _ := os.OpenFile(filepath.Join(h.v, n), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	f.WriteString(line + "\n")
	f.Close()
}

func (h *rulesHome) prompt(s string) string {
	return runHook(h.t, "prompt", map[string]any{"prompt": s, "transcript_path": h.tr, "hook_event_name": "UserPromptSubmit"})
}

func TestRulesRefresh(t *testing.T) {
	h := newRulesHome(t)
	v, loaded, write, appendTo, prompt, all := h.v, h.loaded, h.write, h.add, h.prompt, allRules

	// nothing loaded yet: claude code attaches the rules after the first prompt hook
	write(p("hi"))
	if got := prompt("hi"); got != "" {
		t.Fatalf("no copy loaded yet, still handed over:\n%s", got)
	}
	// the copy this session loaded when it started
	first := loaded(all...)
	write(p("hi"), first)
	if got := prompt("<task-notification>done</task-notification>"); got != "" {
		t.Fatalf("nothing changed, still handed over:\n%s", got)
	}

	tracker := "- the tracker went stale for 2.5 hours. -> write it after every merge (2026-11-01, claude-code)"
	tabs := "- tabs in go"
	appendTo("mistakes.md", tracker)
	appendTo("remember.md", tabs)
	got := prompt("<task-notification>worker finished</task-notification>")
	for _, want := range []string{tracker, tabs, "[mistakes.md]", "[remember.md]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "pkill") || strings.Contains(got, "em dashes") || strings.Contains(got, "project rule") {
		t.Fatalf("old or non-vault lines handed over again:\n%s", got)
	}

	// once per change: claude code keeps what the hook said in the transcript
	write(p("hi"), first, handed(got))
	if again := prompt("go on"); again != "" {
		t.Fatalf("same lines handed over twice:\n%s", again)
	}
	pkill := "- a worker used pkill. -> never pkill"
	appendTo("mistakes.md", pkill)
	next := prompt("go on")
	if !strings.Contains(next, pkill) || strings.Contains(next, tracker) {
		t.Fatalf("want only the newest line:\n%s", next)
	}

	// a mistake nudge and new lines come out together, and both get read back
	write(p("hi"), first, handed(got))
	both := prompt("you made a mistake there")
	if !strings.Contains(both, `file "mistakes"`) || !strings.Contains(both, pkill) {
		t.Fatalf("want the nudge and the new line:\n%s", both)
	}
	write(p("hi"), first, handed(got), handed(both))
	if again := prompt("go on"); again != "" {
		t.Fatalf("lines handed over next to a nudge came back:\n%s", again)
	}

	// compaction or a restart: claude code attaches a fresh copy, that's the new base
	write(p("hi"), first, handed(got), loaded("mistakes.md", "remember.md"))
	if again := prompt("go on"); again != "" {
		t.Fatalf("fresh copy loaded, still handed over:\n%s", again)
	}

	// a worker gets claude code's copy, never what the hook told its parent
	write(p("hi"), first, handed(got))
	w := runHook(t, "subagent", map[string]any{"transcript_path": h.tr, "agent_id": "a1", "agent_type": "general-purpose", "hook_event_name": "SubagentStart"})
	for _, want := range []string{tracker, tabs, pkill} {
		if !strings.Contains(w, want) {
			t.Fatalf("worker missing %q in:\n%s", want, w)
		}
	}

	// a brand new top-level file comes over whole
	os.WriteFile(filepath.Join(v, "dont.md"), []byte("# dont\n\n- no badges\n"), 0o644)
	write(p("hi"), first, handed(got), handed(next))
	if nf := prompt("go on"); !strings.Contains(nf, "[dont.md]") || !strings.Contains(nf, "- no badges") {
		t.Fatalf("new file missing:\n%s", nf)
	}

	// a session that never loaded the vault gets nothing
	write(p("hi"), `{"type":"attachment","attachment":{"type":"instructions","files":[{"path":"/p/CLAUDE.md","type":"Project","content":"x"}]}}`)
	if got := prompt("go on"); got != "" {
		t.Fatalf("vault never loaded, still handed over:\n%s", got)
	}
}

// a long run piles up lessons. claude code may cut a long hook context, so
// they go over a few at a time, the rest next turn, and it says where they are
func TestRulesRefreshFits(t *testing.T) {
	h := newRulesHome(t)
	first := h.loaded(allRules...)
	for i := 0; i < 60; i++ {
		h.add("mistakes.md", fmt.Sprintf("- lesson %02d %s", i, strings.Repeat("x", 300)))
	}
	lines := []string{p("hi"), first}
	h.write(lines...)
	got := map[string]bool{}
	for round := 0; ; round++ {
		out := h.prompt("go on")
		if out == "" {
			break
		}
		if round > 20 {
			t.Fatal("never ran out")
		}
		if len(out) > 8000 {
			t.Fatalf("round %d: %d chars, claude code may cut it", round, len(out))
		}
		n := 0
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "- lesson") {
				got[l[:11]] = true
				n++
			}
		}
		if n == 0 {
			t.Fatalf("round %d handed over no lesson:\n%s", round, out)
		}
		if len(got) < 60 && !strings.Contains(out, filepath.Join(h.v, "mistakes.md")) {
			t.Fatalf("round %d cut it short without saying where the rest is:\n%s", round, out)
		}
		lines = append(lines, handed(out))
		h.write(lines...)
	}
	if len(got) != 60 {
		t.Fatalf("handed over %d of 60 lessons", len(got))
	}
}
