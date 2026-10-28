package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// claude code hooks. the rules ask models to save mistakes and "remember
// this", but a busy model forgets. these make it hard to forget:
//   prompt: when the user's message flags a mistake, reports a bug or says
//           remember, the reminder lands right next to the message
//   stop:   a turn that had one can't end until vault_remember ran with the
//           right file. it blocks once, never twice in a row

var (
	// typos too: "mistka", "mistkae"
	mistakeRe = regexp.MustCompile(`(?i)\bmi\w{0,2}st?\w{0,2}k\w*|learn from (this|that|it|the|your)|(don'?t|never) (make|do) (that|this|it) again|why did you|you (broke|messed|screwed|forgot)|still (broken|not working)|(isn'?t|not) fixed|错误|搞错|别再`)
	// a bug report is a mistake too, just not named one: "x is broken"
	bugRe      = regexp.MustCompile(`(?i)\bbugs?\b|\bbuggy\b|\bbroken\b|\bcrash(es|ed|ing)?\b|(doesn'?t|does not|won'?t|didn'?t) work\b|(isn'?t|is not|stopped|not) working\b|\b(throws?|threw|throwing|getting|got|gives?|giving|shows?|showing) (an? |this |that )?(error|exception)\b|\bfail(s|ed|ing)?\b|\bregress(ion|ed)?\b|\bglitch|报错|坏了|崩了|崩溃|用不了|不能用|不工作`)
	rememberRe = regexp.MustCompile(`(?i)\bremember\b|from now on|记住|以后都`)
	// naming the file isn't flagging a mistake: "check mistakes.md"
	fileRe = regexp.MustCompile(`(?i)\bmistakes?\.md\b`)
)

func flagsMistake(s string) bool { return mistakeRe.MatchString(fileRe.ReplaceAllString(s, "")) }

func reportsBug(s string) bool { return bugRe.MatchString(s) }

func cmdHook(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aivault hook prompt|stop (claude code runs these)")
	}
	b, _ := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	var in struct {
		Prompt         string `json:"prompt"`
		TranscriptPath string `json:"transcript_path"`
		StopHookActive bool   `json:"stop_hook_active"`
	}
	json.Unmarshal(b, &in)
	switch args[0] {
	case "prompt":
		if msg := promptNudge(in.Prompt); msg != "" {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"hookSpecificOutput": map[string]any{"hookEventName": "UserPromptSubmit", "additionalContext": msg},
			})
		}
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
	case rememberRe.MatchString(p):
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
	t := turn{saved: map[string]bool{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var e struct {
			Type       string `json:"type"`
			IsMeta     bool   `json:"isMeta"`
			Attachment struct {
				Type   string `json:"type"`
				Prompt string `json:"prompt"`
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
			var s string
			if json.Unmarshal(e.Message.Content, &s) == nil {
				if !strings.HasPrefix(s, "<") {
					t = turn{said: []string{s}, saved: map[string]bool{}} // a new prompt starts a new turn
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
				switch {
				case p.Type != "text":
				case !tool && !strings.HasPrefix(p.Text, "<"):
					t = turn{said: []string{p.Text}, saved: map[string]bool{}}
				case strings.Contains(p.Text, "The user sent a new message while you were working:"):
					t.said = append(t.said, p.Text)
				}
			}
		case "attachment":
			if e.Attachment.Type == "queued_command" && e.Attachment.Prompt != "" {
				t.said = append(t.said, e.Attachment.Prompt)
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
	case rememberRe.MatchString(said) && !t.saved["*"]:
		return `aivault: the user asked you to remember something this turn and nothing was saved. call vault_remember with file "remember" now, one short line. if they didn't mean a lasting rule, just end.`
	}
	return ""
}
