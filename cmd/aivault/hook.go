package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jxherc/aivault/internal/vault"
)

// claude code hooks. the rules ask models to save mistakes and "remember
// this", but a busy model forgets. these make it hard to forget:
//   prompt:   when the user's message flags a mistake, reports a bug or says
//             remember, the reminder lands right next to the message. it
//             also hands over rules lines the session hasn't seen yet
//   stop:     a turn that had one can't end until vault_remember ran with the
//             right file. it blocks once, never twice in a row
//   subagent: hands a new worker the rules lines its copy is missing

var (
	// typos too: "mistka", "mistkae"
	mistakeRe = regexp.MustCompile(`(?i)\bmi\w{0,2}st?\w{0,2}k\w*|learn from (this|that|it|the|your)|(don'?t|never) (make|do) (that|this|it) again|why did you|you (broke|messed|screwed|forgot)|still (broken|not working)|(isn'?t|not) fixed|错误|搞错|别再`)
	// a bug report is a mistake too, just not named one: "x is broken"
	bugRe      = regexp.MustCompile(`(?i)\bbugs?\b|\bbuggy\b|\bbroken\b|\bcrash(es|ed|ing)?\b|(doesn'?t|does not|won'?t|didn'?t) work\b|(isn'?t|is not|stopped|not) working\b|\b(throws?|threw|throwing|getting|got|gives?|giving|shows?|showing) (an? |this |that )?(error|exception)\b|\bfail(s|ed|ing)?\b|\bregress(ion|ed)?\b|\bglitch|报错|坏了|崩了|崩溃|用不了|不能用|不工作`)
	rememberRe = regexp.MustCompile(`(?i)\bremember\b|from now on|记住|以后都`)
	// naming a file isn't flagging a mistake or asking to remember:
	// "check mistakes.md", "the last line of remember.md"
	fileRe = regexp.MustCompile(`(?i)\b(mistakes?|remember)\.md\b`)
	// the claude desktop app puts these above a message. the user's words come after
	reminderRe = regexp.MustCompile(`^\s*(<system-reminder>[\s\S]*?</system-reminder>\s*)+`)
	// a file's part in what the refresh hands over: "[mistakes.md]"
	sectionRe = regexp.MustCompile(`^\[([^\[\]/\\]+\.md)\]$`)
)

// how the claude desktop app starts a message nobody typed: the brief a
// session gets from the one that started it, and the summary after compaction
var appOpeners = []string{
	"Background (from the session that started this one)",
	"Task (from the session that started this one)",
	"This session is being continued from a previous conversation",
	// a background worker's report. seen 2026-11-05: its "corrections" and
	// "wrong" set off the bug check twice
	"<task-notification>",
	"[SYSTEM NOTIFICATION - NOT USER INPUT]",
}

func flagsMistake(s string) bool { return mistakeRe.MatchString(fileRe.ReplaceAllString(s, "")) }

func reportsBug(s string) bool { return bugRe.MatchString(s) }

func wantsRemembered(s string) bool { return rememberRe.MatchString(fileRe.ReplaceAllString(s, "")) }

// what the user wrote in a prompt, and whether it starts a new turn. tagged
// text (<cross-session-message>, <task-notification>) arrives inside a turn and
// isn't theirs. a brief from another session or a compaction summary starts a
// turn, but nobody in it is the user
func userWords(p string) (string, bool) {
	p = reminderRe.ReplaceAllString(p, "")
	t := strings.TrimSpace(p)
	for _, o := range appOpeners {
		if strings.HasPrefix(t, o) {
			return "", true
		}
	}
	if t == "" || strings.HasPrefix(t, "<") {
		return "", false
	}
	return p, true
}

func cmdHook(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aivault hook prompt|stop|subagent (claude code runs these)")
	}
	b, _ := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	var in struct {
		Prompt         string `json:"prompt"`
		TranscriptPath string `json:"transcript_path"`
		StopHookActive bool   `json:"stop_hook_active"`
		AgentID        string `json:"agent_id"`
	}
	json.Unmarshal(b, &in)
	say := func(event string, msgs ...string) error {
		var ok []string
		for _, m := range msgs {
			if m != "" {
				ok = append(ok, m)
			}
		}
		if len(ok) == 0 {
			return nil
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": strings.Join(ok, "\n\n")},
		})
	}
	switch args[0] {
	case "prompt":
		words, _ := userWords(in.Prompt)
		news := ""
		// transcript_path is always the main session's, so a worker's own
		// prompt can't be told apart from its parent's here
		if in.AgentID == "" {
			news = newRules(in.TranscriptPath, true)
		}
		// nudge first: reading it back later starts at the refresh's header
		return say("UserPromptSubmit", promptNudge(words), news)
	case "subagent":
		// transcript_path is the parent's. the worker starts from claude
		// code's copy of the rules, not from what the parent was handed
		return say("SubagentStart", newRules(in.TranscriptPath, false))
	case "stop":
		if in.StopHookActive || in.TranscriptPath == "" {
			return nil // already blocked once this turn, let it end
		}
		f, err := os.Open(in.TranscriptPath)
		if err != nil {
			return nil // never break a session over a missing file
		}
		defer f.Close()
		if why := unsaved(f); why != "" {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"decision": "block", "reason": why})
		}
	default:
		return fmt.Errorf("unknown hook %q", args[0])
	}
	return nil
}

func promptNudge(p string) string {
	switch {
	case flagsMistake(p):
		return `aivault: the user is pointing out a mistake. before you go on, call vault_remember with file "mistakes": what went wrong, then "->" and the rule that stops it happening again. if you don't know the cause yet, save what happened now and save the cause when you find it. don't write a date, the tool adds one.`
	case reportsBug(p):
		return `aivault: this looks like a bug report. find the root cause, then fix every place with that cause, not just the one the user saw. once you know the cause, call vault_remember with file "mistakes": what went wrong, then "->" and the rule that stops it happening again, written for the cause so it covers every case like this one. don't write a date, the tool adds one.`
	case wantsRemembered(p):
		return `aivault: the user wants this remembered. call vault_remember with file "remember" now, in one short line, before you answer. saying "noted" saves nothing.`
	}
	return ""
}

// what the user said this turn, and which vault files got saved to since
type turn struct {
	said  []string
	saved map[string]bool
}

// reads a claude code transcript (jsonl) and returns the last turn: from the
// last real prompt to the end, plus messages the user queued mid-turn. other
// sessions' messages don't count: a peer reporting its own mistake isn't the
// user flagging one here
func lastTurn(r io.Reader) turn {
	start := func(s string, app bool) (turn, bool) {
		w, ok := userWords(s)
		if !ok {
			return turn{}, false
		}
		t := turn{saved: map[string]bool{}}
		if w != "" && !app {
			t.said = []string{w}
		}
		return t, true
	}
	t := turn{saved: map[string]bool{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var e struct {
			Type       string `json:"type"`
			IsMeta     bool   `json:"isMeta"`
			TurnOrigin string `json:"turnOrigin"`
			Entrypoint string `json:"entrypoint"`
			Attachment struct {
				Type        string `json:"type"`
				Prompt      string `json:"prompt"`
				CommandMode string `json:"commandMode"`
				Origin      struct {
					Kind string `json:"kind"`
				} `json:"origin"`
			} `json:"attachment"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		switch e.Type {
		case "user":
			if e.IsMeta {
				continue
			}
			// the desktop app marks what it sent itself: briefs and messages
			// from other sessions, task notifications, scheduled runs
			app := e.Entrypoint == "claude-desktop" && e.TurnOrigin != "" && e.TurnOrigin != "human"
			var s string
			if json.Unmarshal(e.Message.Content, &s) == nil {
				if nt, ok := start(s, app); ok {
					t = nt // a new prompt starts a new turn
				}
				continue
			}
			var parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			json.Unmarshal(e.Message.Content, &parts)
			tool := false
			for _, p := range parts {
				tool = tool || p.Type == "tool_result"
			}
			for _, p := range parts {
				if p.Type != "text" {
					continue
				}
				if !tool {
					if nt, ok := start(p.Text, app); ok {
						t = nt
					}
					continue
				}
				if strings.Contains(p.Text, "The user sent a new message while you were working:") {
					t.said = append(t.said, p.Text)
				}
			}
		case "attachment":
			// queued while a turn ran. only what the user typed counts:
			// worker reports (commandMode task-notification) and peer
			// messages queue the same way
			a := e.Attachment
			if a.Type != "queued_command" || a.CommandMode == "task-notification" || (a.Origin.Kind != "" && a.Origin.Kind != "human") {
				continue
			}
			if w, ok := userWords(a.Prompt); ok && w != "" {
				t.said = append(t.said, w)
			}
		case "assistant":
			var parts []struct {
				Type  string `json:"type"`
				Name  string `json:"name"`
				Input struct {
					File string `json:"file"`
					Text string `json:"text"`
				} `json:"input"`
			}
			json.Unmarshal(e.Message.Content, &parts)
			for _, p := range parts {
				if p.Type != "tool_use" || !strings.HasSuffix(p.Name, "vault_remember") {
					continue
				}
				file := strings.ToLower(p.Input.File)
				if file == "" {
					if name, _, ok := strings.Cut(p.Input.Text, ":"); ok && !strings.Contains(name, " ") {
						file = strings.ToLower(name)
					}
				}
				t.saved[file] = true
				t.saved["*"] = true
			}
		}
	}
	return t
}

func unsaved(r io.Reader) string {
	t := lastTurn(r)
	said := strings.Join(t.said, "\n")
	switch {
	case flagsMistake(said) && !t.saved["mistakes"] && !t.saved["dont"]:
		return `aivault: the user pointed out a mistake this turn and it isn't in the vault yet. call vault_remember with file "mistakes" now: what went wrong -> the rule that stops it happening again (no date, the tool adds it). if the cause is still unknown, save what happened and say so. if the user wasn't talking about a mistake at all, just end.`
	case reportsBug(said) && !t.saved["mistakes"] && !t.saved["dont"]:
		return `aivault: the user reported a bug this turn and no lesson is in the vault yet. if you found the cause, call vault_remember with file "mistakes" now: what went wrong -> the rule that stops it happening again, for the cause, not just this case. if the cause is still unknown, save what happened and say so. if it wasn't a bug report, or nothing an ai did caused it, just end.`
	case wantsRemembered(said) && !t.saved["*"]:
		return `aivault: the user asked you to remember something this turn and nothing was saved. call vault_remember with file "remember" now, one short line. if they didn't mean a lasting rule, just end.`
	}
	return ""
}

// claude code reads the vault rules (@imports in CLAUDE.md) when a session
// starts and gives every worker that same copy. it reads them again only after
// compaction or a restart, so a lesson saved during a long run never reaches
// it. checked in claude code 2.1.314: memory files are cached per session and
// only re-read on compaction, a settings or account change, or a new process;
// each load is kept in the transcript as an "instructions" attachment

// how the refresh starts. also how it finds its own words in the transcript
const refreshHead = "aivault: new lines in the vault rules since this session loaded them"

// what fits in one hand-over. claude code may cut a hook's context at 8000
// characters, and a cut line would count as handed over
const refreshBudget = 6000

// lines added to the vault's rules files since the session in this transcript
// loaded them, ready for the model. "" when there's nothing new or no way to tell
func newRules(transcript string, ours bool) string {
	if transcript == "" {
		return ""
	}
	_, s, err := load()
	if err != nil || s.Vault == "" {
		return ""
	}
	f, err := os.Open(transcript)
	if err != nil {
		return "" // never break a session over a missing file
	}
	defer f.Close()
	return rulesNews(s.Vault, loadedRules(f, s.Vault, ours))
}

// the lines of each vault rules file (by name) a session has: the newest copy
// claude code attached, plus, with ours, what this hook handed over since
func loadedRules(r io.Reader, dir string, ours bool) map[string]map[string]bool {
	seen := map[string]map[string]bool{}
	inVault := func(p string) bool {
		d := filepath.Dir(p)
		if filepath.Clean(d) == filepath.Clean(dir) {
			return true
		}
		a, err1 := filepath.EvalSymlinks(d)
		b, err2 := filepath.EvalSymlinks(dir)
		return err1 == nil && err2 == nil && a == b
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Bytes()
		// most lines are neither. skip them before parsing a big transcript
		if !bytes.Contains(line, []byte(`"instructions"`)) && !(ours && bytes.Contains(line, []byte(refreshHead))) {
			continue
		}
		var e struct {
			IsSidechain bool `json:"isSidechain"`
			Attachment  struct {
				Type  string `json:"type"`
				Files []struct {
					Path    string `json:"path"`
					Content string `json:"content"`
				} `json:"files"`
				Content json.RawMessage `json:"content"`
			} `json:"attachment"`
		}
		if json.Unmarshal(line, &e) != nil || e.IsSidechain {
			continue
		}
		switch e.Attachment.Type {
		case "instructions":
			// after compaction or a restart only the files that changed come
			// again, so each file keeps its own newest copy
			for _, f := range e.Attachment.Files {
				if inVault(f.Path) {
					seen[filepath.Base(f.Path)] = lineSet(f.Content)
				}
			}
		case "hook_additional_context":
			if !ours {
				continue
			}
			var texts []string
			if json.Unmarshal(e.Attachment.Content, &texts) != nil {
				var one string
				json.Unmarshal(e.Attachment.Content, &one)
				texts = []string{one}
			}
			for _, t := range texts {
				handedBack(t, seen)
			}
		}
	}
	return seen
}

func lineSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			m[l] = true
		}
	}
	return m
}

// adds the lines an earlier refresh handed over, read back from its text
func handedBack(t string, seen map[string]map[string]bool) {
	i := strings.Index(t, refreshHead)
	if i < 0 {
		return
	}
	var cur map[string]bool
	for _, l := range strings.Split(t[i:], "\n") {
		l = strings.TrimSpace(l)
		if m := sectionRe.FindStringSubmatch(l); m != nil {
			if seen[m[1]] == nil {
				seen[m[1]] = map[string]bool{}
			}
			cur = seen[m[1]]
		} else if cur != nil && l != "" {
			cur[l] = true
		}
	}
}

// the hand-over: each rules file's new lines under its name. what doesn't fit
// comes next turn, and the model is told where to read it now
func rulesNews(dir string, seen map[string]map[string]bool) string {
	if len(seen) == 0 {
		return "" // the session never loaded the vault, or claude code hasn't attached it yet
	}
	var parts, rest []string
	size := 0
	for _, p := range vault.RulesFiles(dir) {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		name := filepath.Base(p)
		var add []string
		left := 0
		for _, l := range strings.Split(string(b), "\n") {
			l = strings.TrimRight(l, " \t\r")
			if strings.TrimSpace(l) == "" || seen[name][strings.TrimSpace(l)] {
				continue
			}
			// always at least one line, or a line too long to fit would never go
			if size > 0 && size+len(l)+1 > refreshBudget {
				left++
				continue
			}
			add = append(add, l)
			size += len(l) + 1
		}
		if len(add) > 0 {
			parts = append(parts, "["+name+"]\n"+strings.Join(add, "\n"))
		}
		if left > 0 {
			rest = append(rest, fmt.Sprintf("%d in %s", left, p))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	head := refreshHead + ". they count as much as every other line in those files, so follow them from now on."
	if len(rest) > 0 {
		// before the parts: anything under a [file.md] line reads back as handed over
		head += "\nmore new lines didn't fit here (" + strings.Join(rest, ", ") + "). read those files from disk now, they come over here bit by bit"
	}
	return head + "\n\n" + strings.Join(parts, "\n\n")
}
