package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jxherc/aivault/internal/vault"
	"github.com/jxherc/aivault/internal/wire"
)

// one line of the health report
type check struct {
	Section string `json:"section"`
	Name    string `json:"name"`
	Status  string `json:"status"` // ok, warn, fail, info
	Detail  string `json:"detail,omitempty"`
}

type report struct{ Checks []check }

func (r *report) add(sec, name, status, detail string) {
	r.Checks = append(r.Checks, check{sec, name, status, detail})
}

func (r *report) count(status string) int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == status {
			n++
		}
	}
	return n
}

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fix := fs.Bool("fix", false, "fix what can be fixed (reconnects tools, refreshes copies), then check again")
	asJSON := fs.Bool("json", false, "print the report as json")
	quick := fs.Bool("quick", false, "skip the live mcp test and the update check")
	fs.Parse(args)

	e, s, err := load()
	if err != nil {
		return err
	}
	r := health(e, s, *quick)
	if *fix && r.count("fail") > 0 && s.Vault != "" {
		if !*asJSON {
			printReport(r, os.Stdout)
			fmt.Println("\nfixing...")
		}
		out := io.Discard
		if !*asJSON {
			out = os.Stdout
		}
		ferr := wire.Apply(e, s, s.Vault, append([]string{}, s.Tools...), false, out)
		if ferr != nil && !*asJSON {
			fmt.Println("  still broken:", ferr)
		}
		r = health(e, s, *quick)
		if !*asJSON {
			fmt.Println()
		}
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(r.Checks)
	} else {
		printReport(r, os.Stdout)
	}
	if n := r.count("fail"); n > 0 {
		if !*fix && !*asJSON {
			fmt.Println("\n`aivault doctor --fix` reconnects tools and refreshes copies.")
		}
		return fmt.Errorf("%d problem(s)", n)
	}
	return nil
}

func health(e *wire.Env, s *wire.State, quick bool) *report {
	r := &report{}

	// the binary and updates
	r.add("aivault", "version", "info", curVersion())
	if _, err := os.Stat(e.Bin); err != nil {
		r.add("aivault", "binary", "fail", e.Bin+" is missing, and that's what your tools launch")
	} else {
		r.add("aivault", "binary", "ok", e.Bin)
	}
	if !quick {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		rel, err := latestRelease(ctx)
		cancel()
		switch {
		case err == errNoRelease:
			r.add("aivault", "update", "info", "no release published yet")
		case err != nil:
			r.add("aivault", "update check", "info", "couldn't reach github")
		case curVersion() != "dev" && newer(rel.Tag, curVersion()):
			r.add("aivault", "update", "warn", strings.TrimPrefix(rel.Tag, "v")+" is out. run `aivault update`")
		default:
			r.add("aivault", "update", "ok", "latest")
		}
	}

	if s.Vault == "" {
		r.add("vault", "setup", "fail", "nothing is set up yet. run `aivault init`")
		return r
	}

	// the vault folder
	if fi, err := os.Stat(s.Vault); err != nil || !fi.IsDir() {
		r.add("vault", "folder", "fail", s.Vault+" is missing")
		return r
	}
	if f, err := os.CreateTemp(s.Vault, ".aivault-check-*"); err != nil {
		r.add("vault", "folder", "fail", "can't write to "+s.Vault)
	} else {
		f.Close()
		os.Remove(f.Name())
		r.add("vault", "folder", "ok", s.Vault)
	}
	size := 0
	var names []string
	for _, p := range vault.RulesFiles(s.Vault) {
		fi, err := os.Stat(p)
		if err != nil {
			r.add("vault", "rules", "fail", filepath.Base(p)+" is missing")
			continue
		}
		size += int(fi.Size())
		names = append(names, filepath.Base(p))
	}
	st := "ok"
	detail := fmt.Sprintf("%d files, %s: %s", len(names), kb(size), strings.Join(names, ", "))
	if size > 48<<10 {
		st = "warn"
		detail += ". every tool loads all of this every session, consider moving some to index/"
	}
	r.add("vault", "rules", st, detail)
	mems, _ := vault.Memories(s.Vault)
	skills, _ := vault.Skills(s.Vault)
	idx, _ := filepath.Glob(filepath.Join(s.Vault, "index", "*.md"))
	r.add("vault", "contents", "info", fmt.Sprintf("%d memories, %d skills, %d index lists", len(mems), len(skills), len(idx)))
	if leaks := secretFiles(s.Vault, mems); len(leaks) > 0 {
		more := ""
		if len(leaks) > 3 {
			more = fmt.Sprintf(" and %d more", len(leaks)-3)
			leaks = leaks[:3]
		}
		r.add("vault", "secrets", "warn", "these look like they hold a password or key: "+strings.Join(leaks, ", ")+more)
	} else {
		r.add("vault", "secrets", "ok", "nothing that looks like a key or password")
	}

	// the mcp server, for real: start it the way the tools do and ask it things
	if !quick {
		if d, err := mcpProbe(e.Bin, s.Vault); err != nil {
			r.add("mcp", "server", "fail", err.Error())
		} else {
			r.add("mcp", "server", "ok", d)
		}
	}

	// every connected tool
	for _, name := range s.Tools {
		t := wire.Find(name)
		if t == nil {
			r.add("tools", name, "fail", "unknown tool in ~/.aivault/config.json")
			continue
		}
		if bad := t.Check(e, s); len(bad) > 0 {
			r.add("tools", t.Label, "fail", strings.Join(bad, "; "))
		} else {
			r.add("tools", t.Label, "ok", "rules and mcp in place")
		}
	}
	for _, h := range wire.Hints(e, s) {
		name, msg, _ := strings.Cut(h, ":")
		if t := wire.Find(name); t != nil {
			name = t.Label
		}
		r.add("tools", name, "info", strings.TrimSpace(msg))
	}
	for _, n := range unwired(e, s) {
		r.add("tools", n, "info", "installed but not connected. `aivault init` connects it")
	}
	if bad := wire.LinkProblems(s); len(bad) > 0 {
		r.add("tools", "skill links", "fail", strings.Join(bad, "; "))
	} else {
		r.add("tools", "skill links", "ok", fmt.Sprintf("%d links", len(s.Links)))
	}

	// keeping up to date
	if s.Synced.IsZero() {
		r.add("sync", "last sync", "info", "unknown")
	} else {
		r.add("sync", "last sync", "info", ago(time.Since(s.Synced)))
	}
	if on, d := autosyncState(e); on {
		r.add("sync", "autosync", "ok", d)
	} else {
		r.add("sync", "autosync", "info", "off. `aivault autosync on` keeps every tool current by itself")
	}
	return r
}

func mcpProbe(bin, dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := time.Now()
	c := mcp.NewClient(&mcp.Implementation{Name: "aivault-doctor", Version: curVersion()}, nil)
	sess, err := c.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(bin, "mcp", "--vault", dir)}, nil)
	if err != nil {
		return "", fmt.Errorf("doesn't start: %v", err)
	}
	defer sess.Close()
	tl, err := sess.ListTools(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("started but doesn't answer: %v", err)
	}
	have := map[string]bool{}
	for _, t := range tl.Tools {
		have[t.Name] = true
	}
	for _, n := range []string{"vault_search", "vault_remember"} {
		if !have[n] {
			return "", fmt.Errorf("no %s tool", n)
		}
	}
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "vault_search", Arguments: map[string]any{"query": "", "limit": 1}})
	if err != nil || res.IsError {
		return "", fmt.Errorf("vault_search failed: %v", err)
	}
	return fmt.Sprintf("starts, lists both tools, search works (%d ms)", time.Since(start).Milliseconds()), nil
}

func secretFiles(dir string, mems []vault.Memory) []string {
	var out []string
	for _, m := range mems {
		if vault.LooksSecret(m.Text) {
			out = append(out, "memory/"+filepath.Base(m.Path))
		}
	}
	idx, _ := filepath.Glob(filepath.Join(dir, "index", "*.md"))
	for _, p := range append(vault.RulesFiles(dir), idx...) {
		if b, err := os.ReadFile(p); err == nil && vault.LooksSecret(string(b)) {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
	}
	return out
}

func kb(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%d KB", (n+512)/1024)
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

func printReport(r *report, w io.Writer) {
	color := isatty.IsTerminal(os.Stdout.Fd()) && os.Getenv("NO_COLOR") == ""
	mark := map[string]string{"ok": "✓", "warn": "!", "fail": "✗", "info": "·"}
	paint := map[string]string{"ok": "32", "warn": "33", "fail": "31", "info": "90"}
	width := 0
	for _, c := range r.Checks {
		width = max(width, len(c.Name))
	}
	sec := ""
	for _, c := range r.Checks {
		if c.Section != sec {
			if sec != "" {
				fmt.Fprintln(w)
			}
			sec = c.Section
			fmt.Fprintln(w, sec)
		}
		m := mark[c.Status]
		if color {
			m = "\x1b[" + paint[c.Status] + "m" + m + "\x1b[0m"
		}
		fmt.Fprintf(w, "  %s %-*s  %s\n", m, width, c.Name, c.Detail)
	}
	fmt.Fprintf(w, "\n%d ok, %d warnings, %d problems\n", r.count("ok"), r.count("warn"), r.count("fail"))
}
