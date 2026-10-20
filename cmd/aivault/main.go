package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"

	"github.com/jxherc/aivault/internal/vault"
	"github.com/jxherc/aivault/internal/wire"
)

var version = "dev"

const usage = `aivault: one folder of rules, skills and memory for all your ai tools

  aivault init              set up the vault and connect your tools
  aivault doctor            check everything is still connected
  aivault sync              refresh after editing the vault
  aivault import            move existing rules and skills into the vault
  aivault remove [tool...]  disconnect tools (all if none given)
  aivault search <query>    search memories and skills
  aivault mcp               the mcp server your tools launch
  aivault version

tools: claude, codex, opencode, gemini, kilo, crush, goose, pi, hermes,
       cline, zed, warp, interpreter, aider, qwen, kimi, dsh, zcode,
       minimax, kode, neovate
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit(os.Args[2:])
	case "doctor":
		err = cmdDoctor()
	case "sync":
		err = cmdSync()
	case "import":
		err = cmdImport(os.Args[2:])
	case "remove":
		err = cmdRemove(os.Args[2:])
	case "search":
		err = cmdSearch(os.Args[2:])
	case "mcp":
		err = cmdMCP(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func load() (*wire.Env, *wire.State, error) {
	e, err := wire.DefaultEnv()
	if err != nil {
		return nil, nil, err
	}
	s, err := wire.LoadState(e)
	return e, s, err
}

func expand(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	yes := fs.Bool("yes", false, "don't ask, use flags and defaults")
	dir := fs.String("vault", "", "vault folder (default ~/ai-vault)")
	tools := fs.String("tools", "", "comma list: claude,codex,opencode (default: the ones found)")
	imp := fs.Bool("import", false, "move the tools' existing rules and skills into the vault")
	fs.Parse(args)

	e, s, err := load()
	if err != nil {
		return err
	}
	vdir := *dir
	if vdir == "" {
		vdir = s.Vault
	}
	if vdir == "" {
		vdir = filepath.Join(e.Home, "ai-vault")
	}
	found := wire.Detected(e)
	var picked []string
	if *tools != "" {
		for _, t := range strings.Split(*tools, ",") {
			if t = strings.TrimSpace(t); t != "" {
				picked = append(picked, t)
			}
		}
	} else {
		// found now, plus whatever was wired before (it may not be detectable anymore)
		picked = append([]string{}, found...)
		for _, t := range s.Tools {
			if !contains(picked, t) {
				picked = append(picked, t)
			}
		}
	}

	if !*yes {
		if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
			return errors.New("not a terminal. use `aivault init --yes` with flags")
		}
		if err := wizard(e, s, &vdir, &picked, found, imp); err != nil {
			return err
		}
	}
	if len(picked) == 0 {
		return errors.New("no tools picked. install claude code, codex or opencode, or pass --tools")
	}

	vdir = expand(vdir, e.Home)
	fmt.Printf("vault: %s\n", vdir)
	if err := wire.Apply(e, s, vdir, picked, *imp, os.Stdout); err != nil {
		return err
	}
	fmt.Println("\ndone. restart your ai tools so they load the vault.")
	fmt.Println(newToolNote)
	return nil
}

const newToolNote = "installed another ai tool later? aivault won't notice on its own. run `aivault init` again to connect it."

// tools that look installed but aren't connected
func unwired(e *wire.Env, s *wire.State) []string {
	var out []string
	for _, name := range wire.Detected(e) {
		if !s.HasTool(name) {
			out = append(out, wire.Find(name).Label)
		}
	}
	return out
}

func wizard(e *wire.Env, s *wire.State, vdir *string, picked *[]string, found []string, imp *bool) error {
	isFound := map[string]bool{}
	for _, f := range found {
		isFound[f] = true
	}
	// only what's installed or already connected. the full list is 21 long
	var opts []huh.Option[string]
	for _, t := range wire.All {
		if !isFound[t.Name] && !contains(*picked, t.Name) && len(found) > 0 {
			continue
		}
		label := t.Label
		if !isFound[t.Name] {
			label += " (not found)"
		}
		opts = append(opts, huh.NewOption(label, t.Name).Selected(contains(*picked, t.Name)))
	}
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("vault folder").
			Description("plain markdown. put it in icloud, dropbox or git if you want it on more machines.").
			Value(vdir),
		huh.NewMultiSelect[string]().
			Title("connect these tools").
			Description("only tools found on this computer are listed. others: aivault init --tools <name>").
			Options(opts...).
			Value(picked),
	)).Run()
	if err != nil {
		return err
	}
	if len(*picked) == 0 {
		return nil
	}

	// second screen: offer import if the picked tools already have stuff
	abs, err := filepath.Abs(expand(*vdir, e.Home))
	if err != nil {
		return err
	}
	f, err := wire.Scan(e, s, abs, *picked)
	if err != nil {
		return err
	}
	var fields []huh.Field
	if !f.Empty() {
		*imp = true
		fields = append(fields, huh.NewConfirm().
			Title("move what you already have into the vault?").
			Description("found "+strings.Join(f.Summary(e.Home), ", ")+".\nyour rules go into me.md, skills into skills/. originals are backed up and `aivault remove` puts them back.").
			Affirmative("move them").
			Negative("leave them").
			Value(imp))
	}
	ok := true
	fields = append(fields, huh.NewConfirm().
		Title("apply?").
		Description("every file gets backed up first. `aivault remove` undoes it.").
		Affirmative("yes").
		Negative("no").
		Value(&ok))
	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		return err
	}
	if !ok {
		return errors.New("cancelled, nothing changed")
	}
	return nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func cmdDoctor() error {
	e, s, err := load()
	if err != nil {
		return err
	}
	bad := wire.Doctor(e, s)
	if s.Vault != "" {
		if n := unwired(e, s); len(n) > 0 {
			fmt.Printf("installed but not connected: %s. run `aivault init` to connect.\n", strings.Join(n, ", "))
		}
	}
	if len(bad) == 0 {
		fmt.Printf("all good. vault: %s, tools: %s\n", s.Vault, strings.Join(s.Tools, ", "))
		return nil
	}
	for _, b := range bad {
		fmt.Println("-", b)
	}
	return fmt.Errorf("%d problem(s)", len(bad))
}

func cmdSync() error {
	e, s, err := load()
	if err != nil {
		return err
	}
	if s.Vault == "" {
		return errors.New("nothing is wired yet. run `aivault init`")
	}
	return wire.Apply(e, s, s.Vault, append([]string{}, s.Tools...), false, os.Stdout)
}

func cmdImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	yes := fs.Bool("yes", false, "don't ask")
	fs.Parse(args)
	e, s, err := load()
	if err != nil {
		return err
	}
	if s.Vault == "" {
		return errors.New("nothing is wired yet. run `aivault init`")
	}
	f, err := wire.Scan(e, s, s.Vault, s.Tools)
	if err != nil {
		return err
	}
	if f.Empty() {
		fmt.Println("nothing to import.")
		return nil
	}
	fmt.Println("found", strings.Join(f.Summary(e.Home), ", "))
	if !*yes {
		ok := true
		err := huh.NewConfirm().
			Title("move them into the vault?").
			Description("originals are backed up and `aivault remove` puts them back.").
			Affirmative("move them").
			Negative("leave them").
			Value(&ok).Run()
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("cancelled, nothing changed")
		}
	}
	return wire.Apply(e, s, s.Vault, append([]string{}, s.Tools...), true, os.Stdout)
}

func cmdRemove(args []string) error {
	e, s, err := load()
	if err != nil {
		return err
	}
	if len(s.Tools) == 0 {
		fmt.Println("nothing is wired.")
		return nil
	}
	return wire.Remove(e, s, args, os.Stdout)
}

func vaultDir(flagVal string) (string, error) {
	if flagVal != "" {
		return filepath.Abs(flagVal)
	}
	_, s, err := load()
	if err != nil {
		return "", err
	}
	if s.Vault == "" {
		return "", errors.New("no vault set up. run `aivault init` or pass --vault")
	}
	return s.Vault, nil
}

func cmdSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	dir := fs.String("vault", "", "vault folder")
	limit := fs.Int("n", 8, "max results")
	fs.Parse(args)
	vd, err := vaultDir(*dir)
	if err != nil {
		return err
	}
	hits, err := vault.Search(vd, strings.Join(fs.Args(), " "), *limit)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Println("nothing found.")
	}
	for _, h := range hits {
		meta := h.Kind
		if len(h.Tags) > 0 {
			meta += " [" + strings.Join(h.Tags, ", ") + "]"
		}
		if h.From != "" {
			meta += " from " + h.From
		}
		fmt.Printf("%s\n  %s\n  %s\n\n", meta, strings.ReplaceAll(h.Text, "\n", "\n  "), h.Path)
	}
	return nil
}

func cmdMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	dir := fs.String("vault", "", "vault folder (default: the one from init)")
	fs.Parse(args)
	vd, err := vaultDir(*dir)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return serveMCP(ctx, vd)
}
