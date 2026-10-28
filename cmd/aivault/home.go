package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jxherc/aivault/internal/vault"
	"github.com/jxherc/aivault/internal/wire"
)

// `aivault` on its own: what's connected, what's in the vault, what got
// saved lately. all local reads, no network, so it's quick
func cmdHome() error {
	e, s, err := load()
	if err != nil {
		return err
	}
	ver := curVersion()
	if isDev(ver) {
		ver = "dev build"
	}
	fmt.Println(bold.Render(accent.Render("aivault")) + "  " + dim.Render(ver))
	fmt.Println()
	if s.Vault == "" {
		fmt.Println("  one folder of rules, skills and memory for all your ai tools.")
		fmt.Println("  nothing is set up yet. " + accent.Render("aivault init") + " connects your tools.")
		fmt.Println()
		fmt.Println(dim.Render("  aivault help lists every command"))
		return nil
	}
	if fi, err := os.Stat(s.Vault); err != nil || !fi.IsDir() {
		fmt.Println("  " + mark("fail") + " the vault folder is missing: " + tildePath(s.Vault))
		fmt.Println("    " + accent.Render("aivault doctor") + " shows what's wrong")
		return nil
	}

	_, stop := spin("reading the vault")
	h := gather(e, s)
	stop()

	w := width()
	// label column, then the value as given. values are cut or wrapped
	// before they get here
	row := func(label, val string) {
		fmt.Println(strings.TrimRight("  "+dim.Render(fmt.Sprintf("%-8s", label))+" "+val, " "))
	}
	row("vault", cut(tildePath(s.Vault), w-11))

	var names []string
	for _, t := range h.tools {
		names = append(names, mark(t.status)+" "+t.label)
	}
	row("tools", wrapItems(names, "  ", w-11, 11))
	for _, t := range h.tools {
		if t.status == "fail" {
			row("", bad.Render(t.label+": ")+cut(tildeText(t.problem), w-13-len(t.label)))
		}
	}
	if h.failed > 0 {
		row("", accent.Render("aivault doctor --fix")+" repairs what it can")
	}
	if len(h.unwired) > 0 {
		items := append([]string{"not connected: " + h.unwired[0]}, h.unwired[1:]...)
		row("", paint(dim, wrapItems(items, ", ", w-11, 11))+dim.Render(". ")+accent.Render("aivault init"))
	}
	fmt.Println()

	mem := fmt.Sprintf("%d", len(h.mems))
	if h.week > 0 {
		mem += fmt.Sprintf(", %d this week", h.week)
	}
	row("memory", mem)
	if src := sources(h.mems, 3); len(src) > 0 {
		row("", paint(dim, wrapItems(src, " · ", w-11, 11)))
	}
	row("skills", fmt.Sprint(h.skills))
	row("index", fmt.Sprintf("%d lists, %d entries", h.lists, h.entries))
	row("rules", wrap(fmt.Sprintf("%d files, %s, loaded by every tool each session", h.rules, kb(h.rulesSize)), w-11, 11))
	syncd := "never"
	if !s.Synced.IsZero() {
		syncd = ago(time.Since(s.Synced))
	}
	if h.auto {
		syncd += ", autosync on"
	} else {
		syncd += ", autosync off"
	}
	row("synced", syncd)
	fmt.Println()

	days := min(56, w-13)
	if days >= 14 && len(h.mems) > 0 {
		row("saved", sparkline(perDay(h.mems, days, time.Now())))
		from := strings.ToLower(time.Now().AddDate(0, 0, -(days - 1)).Format("Jan 2"))
		row("", dim.Render(from+strings.Repeat(" ", max(1, days-len(from)-5))+"today"))
	}
	recent := latest(h.mems, 3)
	fromW := 0
	for _, m := range recent {
		fromW = max(fromW, len(m.From))
	}
	for i, m := range recent {
		label := ""
		if i == 0 {
			label = "latest"
		}
		t, _ := parseCreated(m.Created)
		meta := fmt.Sprintf("%-6s %-*s  ", short(time.Since(t)), fromW, m.From)
		row(label, dim.Render(meta)+cut(firstLine(m.Text), w-11-len(meta)))
	}
	fmt.Println()
	fmt.Println("  " + accent.Render("aivault help") + "   " + accent.Render("aivault search <words>") + "   " + accent.Render("aivault doctor"))
	return nil
}

type toolState struct{ label, status, problem string }

type homeData struct {
	tools          []toolState
	failed         int
	unwired        []string
	mems           []vault.Memory
	week           int
	skills         int
	lists, entries int
	rules          int
	rulesSize      int
	auto           bool
}

func gather(e *wire.Env, s *wire.State) homeData {
	var h homeData
	for _, name := range s.Tools {
		t := wire.Find(name)
		if t == nil {
			continue
		}
		ts := toolState{label: t.Label, status: "ok"}
		if bad := t.Check(e, s); len(bad) > 0 {
			ts.status, ts.problem = "fail", strings.Join(bad, "; ")
			h.failed++
		}
		h.tools = append(h.tools, ts)
	}
	if bad := wire.LinkProblems(s); len(bad) > 0 {
		h.tools = append(h.tools, toolState{"skill links", "fail", strings.Join(bad, "; ")})
		h.failed++
	}
	h.unwired = unwired(e, s)
	h.mems, _ = vault.Memories(s.Vault)
	for _, c := range perDay(h.mems, 7, time.Now()) {
		h.week += c
	}
	sk, _ := vault.Skills(s.Vault)
	h.skills = len(sk)
	idx, _ := filepath.Glob(filepath.Join(s.Vault, "index", "*.md"))
	h.lists = len(idx)
	for _, p := range idx {
		h.entries += bullets(p)
	}
	for _, p := range vault.RulesFiles(s.Vault) {
		if fi, err := os.Stat(p); err == nil {
			h.rules++
			h.rulesSize += int(fi.Size())
		}
	}
	h.auto, _ = autosyncState(e)
	return h
}

func parseCreated(s string) (time.Time, bool) {
	for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// memories saved per day, oldest first, the last entry is today
func perDay(mems []vault.Memory, days int, now time.Time) []int {
	out := make([]int, days)
	y, mo, d := now.Date()
	today := time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
	for _, m := range mems {
		t, ok := parseCreated(m.Created)
		if !ok {
			continue
		}
		t = t.In(now.Location())
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
		i := days - 1 - int(today.Sub(day).Hours()/24)
		if i >= 0 && i < days {
			out[i]++
		}
	}
	return out
}

func latest(mems []vault.Memory, n int) []vault.Memory {
	ms := append([]vault.Memory{}, mems...)
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Created > ms[j].Created })
	return ms[:min(n, len(ms))]
}

// "claude 180", "codex 70", "+3 more"
func sources(mems []vault.Memory, n int) []string {
	count := map[string]int{}
	for _, m := range mems {
		if m.From != "" {
			count[m.From]++
		}
	}
	var keys []string
	for k := range count {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return count[keys[i]] > count[keys[j]] || count[keys[i]] == count[keys[j]] && keys[i] < keys[j]
	})
	var parts []string
	for i, k := range keys {
		if i == n {
			parts = append(parts, fmt.Sprintf("+%d more", len(keys)-n))
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", k, count[k]))
	}
	return parts
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "#-*> "))
		if l != "" && l != "---" {
			return strings.ReplaceAll(l, "**", "")
		}
	}
	return ""
}

func short(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
	return fmt.Sprintf("%d d", int(d.Hours()/24))
}
