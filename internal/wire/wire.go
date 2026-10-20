package wire

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"aivault/internal/vault"
)

// Apply creates the vault (if needed) and wires each tool. safe to run again:
// that's also what sync does. warnings are things the user should know but
// that didn't stop the run
func Apply(e *Env, s *State, vaultDir string, tools []string, out io.Writer) error {
	abs, err := filepath.Abs(vaultDir)
	if err != nil {
		return err
	}
	if s.Vault != "" && filepath.Clean(s.Vault) != abs {
		return fmt.Errorf("already wired to %s. run `aivault remove` first to switch vaults", s.Vault)
	}
	if err := vault.Create(abs); err != nil {
		return err
	}
	s.Vault = abs

	var failed []string
	for _, name := range tools {
		t := Find(name)
		if t == nil {
			return fmt.Errorf("unknown tool %q (know: claude, codex, opencode)", name)
		}
		if !s.HasTool(name) {
			s.Tools = append(s.Tools, name)
			sort.Strings(s.Tools)
		}
		err := t.Apply(e, s)
		// save after every tool so a crash halfway can still be undone
		if serr := s.Save(); serr != nil {
			return serr
		}
		if err != nil {
			fmt.Fprintf(out, "  %-9s problem: %v\n", t.Name, err)
			failed = append(failed, t.Name)
			continue
		}
		fmt.Fprintf(out, "  %-9s ok\n", t.Name)
	}
	for _, w := range syncLinks(e, s) {
		fmt.Fprintln(out, "  warning:", w)
	}
	if err := s.Save(); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d tool(s) need attention: %v", len(failed), failed)
	}
	return nil
}

// Remove unwires the given tools, or all of them when tools is empty
func Remove(e *Env, s *State, tools []string, out io.Writer) error {
	if len(tools) == 0 {
		tools = append([]string{}, s.Tools...)
	}
	var errs []string
	for _, name := range tools {
		t := Find(name)
		if t == nil {
			return fmt.Errorf("unknown tool %q", name)
		}
		if !s.HasTool(name) {
			fmt.Fprintf(out, "  %-9s wasn't wired\n", name)
			continue
		}
		if err := t.Remove(e, s); err != nil {
			fmt.Fprintf(out, "  %-9s problem: %v\n", name, err)
			errs = append(errs, name)
		} else {
			fmt.Fprintf(out, "  %-9s removed\n", name)
		}
		// drop it from the list either way, the error tells the user what's left
		var keep []string
		for _, x := range s.Tools {
			if x != name {
				keep = append(keep, x)
			}
		}
		s.Tools = keep
	}
	for _, w := range syncLinks(e, s) {
		fmt.Fprintln(out, "  warning:", w)
	}
	if len(s.Tools) == 0 {
		s.Vault = ""
	}
	if err := s.Save(); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("couldn't fully remove: %v", errs)
	}
	return nil
}

// Doctor returns every problem it finds. empty = all good
func Doctor(e *Env, s *State) []string {
	if s.Vault == "" {
		return []string{"nothing is wired yet. run `aivault init`"}
	}
	var bad []string
	for _, f := range vault.RulesFiles(s.Vault) {
		if _, err := os.Stat(f); err != nil {
			bad = append(bad, "vault file missing: "+f)
		}
	}
	for _, name := range s.Tools {
		if t := Find(name); t != nil {
			for _, b := range t.Check(e, s) {
				bad = append(bad, name+": "+b)
			}
		}
	}
	return append(bad, checkLinks(s)...)
}

// Detected lists the tools that look installed
func Detected(e *Env) []string {
	var out []string
	for _, t := range All {
		if t.Detect(e) {
			out = append(out, t.Name)
		}
	}
	return out
}
