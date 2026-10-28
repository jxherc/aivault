package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"

	"github.com/jxherc/aivault/internal/vault"
	"github.com/jxherc/aivault/internal/wire"
)

var version = "dev"

// the help screen. each group is a heading and its commands
var helpGroups = []struct {
	title string
	cmds  [][2]string
}{
	{"setup", [][2]string{
		{"init", "set up the vault and connect your tools"},
		{"remove [tool...]", "disconnect tools (all if none given)"},
	}},
	{"health", [][2]string{
		{"doctor", "check everything: vault, mcp server, every tool. --fix repairs, --json for scripts"},
		{"update", "update aivault itself. --check only looks"},
	}},
	{"keeping in sync", [][2]string{
		{"sync", "refresh the tools and pull in anything new from them"},
		{"autosync on|off", "keep everything in sync in the background"},
		{"watch", "keep everything in sync while this terminal is open"},
		{"import", "move existing rules and skills into the vault"},
	}},
	{"using the vault", [][2]string{
		{"search <query>", "search memories, skills and index lists"},
		{"add [--file f] text", "save a memory. --file remember adds it to remember.md too"},
		{"open [file]", "open the vault, or one of its files"},
	}},
	{"other", [][2]string{
		{"completion <shell>", "shell completion: bash, zsh, fish, powershell"},
		{"mcp", "the mcp server your tools launch"},
		{"version", ""},
	}},
}

func printHelp() {
	w := width()
	fmt.Println(bold.Render(accent.Render("aivault")) + "  one folder of rules, skills and memory for all your ai tools")
	fmt.Println()
	fmt.Println("  " + accent.Render("aivault") + dim.Render(" on its own shows what's connected and what's in the vault"))
	for _, g := range helpGroups {
		fmt.Println()
		fmt.Println(bold.Render(g.title))
		for _, c := range g.cmds {
			fmt.Println(strings.TrimRight("  "+accent.Render(fmt.Sprintf("%-20s", c[0]))+" "+wrap(c[1], w-23, 23), " "))
		}
	}
	fmt.Println()
	fmt.Println(bold.Render("tools"))
	fmt.Println("  " + wrap(strings.Join(wire.Names(), ", "), w-2, 2))
}

func main() {
	cleanOld()
	if len(os.Args) < 2 {
		if err := cmdHome(); err != nil {
			fmt.Fprintln(os.Stderr, bad.Render("error:"), err)
			os.Exit(1)
		}
		return
	}
	var err error
	args := os.Args[2:]
	switch os.Args[1] {
	case "init":
		err = cmdInit(args)
	case "doctor", "status", "health", "check":
		err = cmdDoctor(args)
	case "sync":
		if rest := plain("sync", args); len(rest) > 0 {
			err = fmt.Errorf("sync takes no arguments, got %q", rest[0])
		} else {
			err = cmdSync()
		}
	case "import":
		err = cmdImport(args)
	case "remove":
		err = cmdRemove(plain("remove", args))
	case "search":
		err = cmdSearch(args)
	case "mcp":
		err = cmdMCP(args)
	case "add":
		err = cmdAdd(args)
	case "open":
		err = cmdOpen(plain("open", args))
	case "watch":
		err = cmdWatch(args)
	case "autosync":
		err = cmdAutosync(plain("autosync", args))
	case "update", "upgrade":
		err = cmdUpdate(args)
	case "completion":
		err = cmdCompletion(plain("completion", args))
	case "hook":
		err = cmdHook(args)
	case "version", "--version", "-v":
		plain("version", args)
		fmt.Println(curVersion())
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "no command %q.", os.Args[1])
		if m := didYouMean(os.Args[1], commands); m != "" {
			fmt.Fprintf(os.Stderr, " did you mean %s?", accent.Render("aivault "+m))
		}
		fmt.Fprintln(os.Stderr, " "+dim.Render("aivault help lists them all"))
		os.Exit(2)
	}
	if err == errQuiet {
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, bad.Render("error:"), codes(err.Error(), out.NewStyle()))
		os.Exit(1)
	}
}

// exit 1 without a message, when the output already explained it
var errQuiet = errors.New("failed")

// every command reads its args through one of these, so -h shows the
// command's help and a flag it doesn't have is an error, not a tool name
func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.SetOutput(os.Stdout)
	fs.Usage = func() { cmdHelp(fs) }
	return fs
}

// args of a command that has no flags of its own
func plain(name string, args []string) []string {
	fs := newFlags(name)
	fs.Parse(args)
	return fs.Args()
}

func cmdHelp(fs *flag.FlagSet) {
	use, desc := fs.Name(), ""
	for _, g := range helpGroups {
		for _, c := range g.cmds {
			if strings.Fields(c[0])[0] == fs.Name() {
				use, desc = c[0], c[1]
			}
		}
	}
	fmt.Println(accent.Render("aivault " + use))
	if desc != "" {
		fmt.Println("  " + wrap(desc, width()-2, 2))
	}
	fs.VisitAll(func(f *flag.Flag) {
		// go reads a `quoted` word in a usage as the flag's value name
		arg, usage := flag.UnquoteUsage(f)
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
			arg = ""
		}
		name := strings.TrimSpace("--" + f.Name + " " + arg)
		fmt.Println("  " + accent.Render(fmt.Sprintf("%-16s", name)) + " " + wrap(usage, width()-19, 19))
	})
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
	fs := newFlags("init")
	yes := fs.Bool("yes", false, "don't ask, use flags and defaults")
	dir := fs.String("vault", "", "vault folder (default ~/ai-vault)")
	tools := fs.String("tools", "", "comma list: claude,codex,opencode (default: the ones found)")
	imp := fs.Bool("import", false, "move the tools' existing rules and skills into the vault")
	auto := fs.Bool("autosync", false, "keep the tools in sync in the background, like aivault autosync on")
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
		if err := wizard(e, s, &vdir, &picked, found, imp, auto); err != nil {
			return err
		}
	}
	if len(picked) == 0 {
		return errors.New("no tools picked. install claude code, codex or opencode, or pass --tools")
	}

	vdir = expand(vdir, e.Home)
	fmt.Println(dim.Render("vault ") + tildePath(vdir))
	if err := runPainted(func(w io.Writer) error { return wire.Apply(e, s, vdir, picked, *imp, w) }); err != nil {
		return err
	}
	if *auto {
		if err := autosyncOn(e); err != nil {
			fmt.Println("  "+mark("fail")+" couldn't turn on autosync:", err)
		} else {
			fmt.Println("  " + mark("ok") + " autosync on: edits to the vault reach every tool by themselves")
		}
	}
	fmt.Println()
	fmt.Println(mark("ok") + " done. restart your ai tools so they load the vault. " + accent.Render("aivault doctor") + " checks everything")
	fmt.Println("  " + codes(newToolNote, dim))
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

func wizard(e *wire.Env, s *wire.State, vdir *string, picked *[]string, found []string, imp, auto *bool) error {
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
	if on, _ := autosyncState(e); !on {
		*auto = true
		fields = append(fields, huh.NewConfirm().
			Title("keep everything in sync in the background?").
			Description("starts at login. vault edits reach every tool within seconds, and the tools' own memories come in every hour.\n`aivault autosync off` stops it.").
			Affirmative("yes").
			Negative("no, i'll run sync").
			Value(auto))
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

func cmdSync() error {
	e, s, err := load()
	if err != nil {
		return err
	}
	if s.Vault == "" {
		return errors.New("nothing is wired yet. run `aivault init`")
	}
	// sync pulls in whatever the tools gained since: rules text written
	// outside our block, new skill folders, and their own memories
	start := time.Now()
	err = runPainted(func(w io.Writer) error {
		return wire.Apply(e, s, s.Vault, append([]string{}, s.Tools...), true, w)
	})
	if err == nil {
		fmt.Println(dim.Render(fmt.Sprintf("synced %d tool%s in %.1f s", len(s.Tools), plural(len(s.Tools)), time.Since(start).Seconds())))
	}
	return err
}

// runs a wire step with its report lines painted
func runPainted(f func(io.Writer) error) error {
	p := painted(os.Stdout)
	err := f(p)
	p.Flush()
	return err
}

func cmdImport(args []string) error {
	fs := newFlags("import")
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
	return runPainted(func(w io.Writer) error {
		return wire.Apply(e, s, s.Vault, append([]string{}, s.Tools...), true, w)
	})
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
	return runPainted(func(w io.Writer) error { return wire.Remove(e, s, args, w) })
}

// a path inside the vault as vault-relative, anything else with ~
func vaultRel(vd, p string) string {
	if rel, err := filepath.Rel(vd, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return tildePath(p)
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
	fs := newFlags("search")
	dir := fs.String("vault", "", "vault folder")
	limit := fs.Int("n", 8, "max results")
	q := strings.Join(parseAnywhere(fs, args), " ")
	vd, err := vaultDir(*dir)
	if err != nil {
		return err
	}
	hits, err := vault.Search(vd, q, *limit)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Println("nothing found for " + bold.Render(q))
		return nil
	}
	w := width()
	for _, h := range hits {
		meta := []string{accent.Render(h.Kind)}
		if h.From != "" {
			meta = append(meta, h.From)
		}
		if t, ok := parseCreated(h.Created); ok {
			meta = append(meta, short(time.Since(t))+" ago")
		}
		if len(h.Tags) > 0 && h.Kind != "index" {
			meta = append(meta, strings.Join(h.Tags, ", "))
		}
		lines := snippet(h.Text, q, 3)
		if h.Kind == "index" {
			// "files / school / cs (...): the entry". the headings go up top
			if head, body, ok := strings.Cut(h.Text, ": "); ok {
				meta = append(meta, cut(aliasRe.ReplaceAllString(head, ""), w-12))
				lines = strings.Split(wrap(tildeText(body), w-2, 0), "\n")
				lines = lines[:min(3, len(lines))]
			}
		}
		fmt.Println(strings.Join(meta, dim.Render(" · ")))
		for _, l := range lines {
			fmt.Println("  " + highlight(cut(tildeText(l), w-2), q))
		}
		fmt.Println("  " + dim.Render(vaultRel(vd, h.Path)))
		fmt.Println()
	}
	fmt.Println(dim.Render(fmt.Sprintf("%d result%s. ", len(hits), plural(len(hits)))) + accent.Render("aivault open <path>") + dim.Render(" opens one"))
	return nil
}

// the "(ics4u, cs, ...)" alias lists in index headings
var aliasRe = regexp.MustCompile(` \([^)]*\)`)

// flags can come after the words too: `aivault search cs -n 3`
func parseAnywhere(fs *flag.FlagSet, args []string) []string {
	var words []string
	for {
		fs.Parse(args)
		args = fs.Args()
		if len(args) == 0 {
			return words
		}
		words = append(words, args[0])
		args = args[1:]
	}
}

// up to n lines of text, the ones with the query's words first
func snippet(text, q string, n int) []string {
	words := strings.Fields(strings.ToLower(q))
	var hit, rest []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l == "---" || strings.HasPrefix(l, "… (cut") {
			continue
		}
		low := strings.ToLower(l)
		matched := false
		for _, w := range words {
			matched = matched || strings.Contains(low, w)
		}
		if matched {
			hit = append(hit, l)
		} else {
			rest = append(rest, l)
		}
	}
	out := append(hit, rest...)
	return out[:min(n, len(out))]
}

// the query's words in bold accent. matching is case-insensitive
func highlight(s, q string) string {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return s
	}
	low := strings.ToLower(s)
	var b strings.Builder
	for i := 0; i < len(s); {
		n := 0
		for _, w := range words {
			if strings.HasPrefix(low[i:], w) && len(w) > n {
				n = len(w)
			}
		}
		if n > 0 {
			b.WriteString(bold.Inherit(accent).Render(s[i : i+n]))
			i += n
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func cmdMCP(args []string) error {
	fs := newFlags("mcp")
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
