package main

import (
	"flag"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"testing"
	"time"

	"github.com/jxherc/aivault/internal/vault"
)

// tests run without a terminal, so every style renders as plain text

func TestSparkline(t *testing.T) {
	if got := sparkline([]int{0, 1, 4, 100}); got != "▁▂▂█" {
		t.Fatalf("got %q", got)
	}
	if got := sparkline([]int{0, 0}); got != "▁▁" {
		t.Fatalf("all zero: %q", got)
	}
}

func TestPerDay(t *testing.T) {
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	ms := []vault.Memory{
		{Created: "2026-10-28T01:00:00Z"},
		{Created: "2026-10-28T14:00:00Z"},
		{Created: "2026-10-26T09:00:00Z"},
		{Created: "2026-09-01T09:00:00Z"}, // too old
		{Created: "garbage"},
	}
	got := perDay(ms, 3, now)
	if got[0] != 1 || got[1] != 0 || got[2] != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestWrapItems(t *testing.T) {
	got := wrapItems([]string{"✓ Claude Code", "✓ DeepSeek Harness", "✓ Kimi Code"}, "  ", 40, 4)
	if got != "✓ Claude Code  ✓ DeepSeek Harness\n    ✓ Kimi Code" {
		t.Fatalf("got %q", got)
	}
}

func TestWrapAndCut(t *testing.T) {
	if got := wrap("one two three four five six seven eight nine ten eleven", 30, 2); got != "one two three four five six\n  seven eight nine ten eleven" {
		t.Fatalf("wrap: %q", got)
	}
	if got := cut("abcdefgh", 5); got != "abcd…" {
		t.Fatalf("cut: %q", got)
	}
	if got := cut("abc", 5); got != "abc" {
		t.Fatalf("cut short: %q", got)
	}
}

func TestDidYouMean(t *testing.T) {
	for in, want := range map[string]string{"docter": "doctor", "se": "search", "snyc": "sync", "xyzzy": ""} {
		if got := didYouMean(in, commands); got != want {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}

func TestCodes(t *testing.T) {
	if got := codes("run `aivault init` now", out.NewStyle()); got != "run aivault init now" {
		t.Fatalf("got %q", got)
	}
	if got := codes("a ` lone tick", out.NewStyle()); got != "a ` lone tick" {
		t.Fatalf("unpaired: %q", got)
	}
}

func TestSnippet(t *testing.T) {
	text := "# title\n\nfirst line\nthe worksheet is here\n---\nlast"
	got := snippet(text, "Worksheet", 2)
	if strings.Join(got, "|") != "the worksheet is here|# title" {
		t.Fatalf("got %q", got)
	}
}

func TestParseAnywhere(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	n := fs.Int("n", 8, "")
	words := parseAnywhere(fs, []string{"cs", "worksheet", "-n", "3", "five"})
	if *n != 3 || strings.Join(words, " ") != "cs worksheet five" {
		t.Fatalf("n=%d words=%q", *n, words)
	}
}

func TestSources(t *testing.T) {
	ms := []vault.Memory{{From: "codex"}, {From: "claude"}, {From: "claude"}, {From: "pi"}, {From: "cli"}, {}}
	if got := strings.Join(sources(ms, 2), ", "); got != "claude 2, cli 1, +2 more" {
		t.Fatalf("got %q", got)
	}
}

func TestPaintLine(t *testing.T) {
	if got := paintLine("  ✓ Codex"); got != "  ✓ Codex" {
		t.Fatalf("got %q", got)
	}
	if got := paintLine("  plain"); got != "  plain" {
		t.Fatalf("got %q", got)
	}
}

func TestSpinStopTwice(t *testing.T) {
	_, stop := spin("x")
	stop()
	stop()
}

func TestPaintNoPadding(t *testing.T) {
	st := lipgloss.NewStyle().Bold(true)
	for _, got := range []string{paint(st, "a long line\nshort"), codes("a long line\nshort `x`", st)} {
		for _, l := range strings.Split(got, "\n") {
			if strings.HasSuffix(l, " ") {
				t.Fatalf("trailing space in %q", got)
			}
		}
	}
}
