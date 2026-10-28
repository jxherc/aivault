package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jxherc/aivault/internal/vault"
	"github.com/jxherc/aivault/internal/wire"
)

// aivault add [--file remember] text: what vault_remember does, from a terminal
func cmdAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	file := fs.String("file", "", "also add the line to this vault file, like remember or mistakes")
	tags := fs.String("tags", "", "comma list of topic words")
	text := strings.TrimSpace(strings.Join(parseAnywhere(fs, args), " "))
	if text == "" {
		return errors.New(`usage: aivault add [--file remember] "text"`)
	}
	_, s, err := load()
	if err != nil {
		return err
	}
	if s.Vault == "" {
		return errors.New("no vault set up. run `aivault init`")
	}
	if vault.LooksSecret(text) {
		return errors.New("that looks like a password or key. the vault doesn't keep those")
	}
	var tl []string
	for _, t := range strings.Split(*tags, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tl = append(tl, t)
		}
	}
	if *file != "" {
		p, err := vault.AddNote(s.Vault, *file, text, "cli")
		if err != nil {
			return err
		}
		fmt.Println(mark("ok") + " added to " + vaultRel(s.Vault, p))
		tl = append(tl, *file)
		if filepath.Dir(p) == filepath.Clean(s.Vault) {
			refresh(s.Vault)
		}
	}
	m, err := vault.Remember(s.Vault, text, "cli", tl)
	if err != nil {
		return err
	}
	fmt.Println(mark("ok") + " saved " + vaultRel(s.Vault, m.Path))
	return nil
}

// aivault open [name]: the vault, or one of its files, in the default app
func cmdOpen(args []string) error {
	_, s, err := load()
	if err != nil {
		return err
	}
	if s.Vault == "" {
		return errors.New("no vault set up. run `aivault init`")
	}
	target := s.Vault
	if len(args) > 0 {
		switch args[0] {
		case "memory", "skills", "index", "library":
			target = filepath.Join(s.Vault, args[0])
		default:
			if target, err = vault.NoteFile(s.Vault, args[0]); err != nil {
				return err
			}
		}
	}
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", target)
	case "windows":
		c = exec.Command("explorer", target)
	default:
		c = exec.Command("xdg-open", target)
	}
	fmt.Println(target)
	// explorer exits 1 even when it worked
	if err := c.Run(); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("couldn't open it: %w", err)
	}
	return nil
}

// aivault watch: refresh the tools' copies whenever a rules file changes,
// and do a full sync every so often so the tools' own memories come in too
func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	every := fs.Duration("sync", 0, "also run a full sync this often, like 1h. 0 = never")
	logTo := fs.String("log", "", "append output to this file instead of printing it")
	fs.Parse(args)
	if *logTo != "" {
		f, err := os.OpenFile(*logTo, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		os.Stdout, os.Stderr = f, f
	}
	e, s, err := load()
	if err != nil {
		return err
	}
	if s.Vault == "" {
		return errors.New("no vault set up. run `aivault init`")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	logf := func(f string, a ...any) { fmt.Printf(time.Now().Format("2006-01-02 15:04:05")+" "+f+"\n", a...) }
	logf("watching %s", s.Vault)

	last := rulesStamp(s.Vault)
	lastSync := time.Now()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		// reload each time: init or remove may have changed things meanwhile
		if s2, err := wire.LoadState(e); err == nil {
			s = s2
		}
		if s.Vault == "" {
			logf("vault was removed, stopping")
			return nil
		}
		if now := rulesStamp(s.Vault); now != last {
			last = now
			if err := wire.RefreshRules(e, s); err != nil {
				logf("refresh: %v", err)
			} else {
				logf("rules changed, tools refreshed")
			}
		}
		if *every > 0 && time.Since(lastSync) >= *every {
			lastSync = time.Now()
			var out strings.Builder
			if err := wire.Apply(e, s, s.Vault, append([]string{}, s.Tools...), true, &out); err != nil {
				logf("sync: %v", err)
			}
			// only say something when the sync brought something in
			for _, l := range strings.Split(out.String(), "\n") {
				if l = strings.TrimSpace(l); l != "" && !strings.HasSuffix(l, " ok") && !strings.HasPrefix(l, "note:") {
					logf("sync: %s", l)
				}
			}
		}
	}
}

// names, sizes and times of the rules files. any change means refresh
func rulesStamp(dir string) string {
	var b strings.Builder
	for _, p := range vault.RulesFiles(dir) {
		if fi, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%s %d %d\n", filepath.Base(p), fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return b.String()
}
