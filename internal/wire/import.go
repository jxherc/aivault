package wire

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jxherc/aivault/internal/vault"
)

// import moves what a user already has into the vault: the text of each
// tool's global rules file, and skill folders. the originals get backed up
// and `remove` puts them back

type RuleSrc struct {
	Tools []string
	Paths []string // same text can live in more than one tool's file
	Text  string
}

type SkillSrc struct {
	Name string
	Path string // the real folder
	Tool string
	Dup  bool // same files as another one being imported. moved aside, not copied
}

type Found struct {
	Rules    []RuleSrc
	Skills   []SkillSrc
	Skipped  []string // things we saw but won't touch, with why
	Linked   int      // skills that are links to somewhere else, left alone
	Memories int      // tool memories not in the vault yet (copied, not moved)
	tools    []string
}

func (f Found) Empty() bool { return len(f.Rules) == 0 && len(f.Skills) == 0 && f.Memories == 0 }

// Summary is one short line per thing, for the wizard and the cli
func (f Found) Summary(home string) []string {
	var out []string
	for _, r := range f.Rules {
		for _, p := range r.Paths {
			out = append(out, fmt.Sprintf("%s (%s)", tilde(p, home), size(len(r.Text))))
		}
	}
	if n := f.unique(); n > 0 {
		out = append(out, fmt.Sprintf("%d skill%s", n, plural(n)))
	}
	if f.Memories > 0 {
		out = append(out, fmt.Sprintf("%d memor%s from the tools' own memory", f.Memories, map[bool]string{true: "y", false: "ies"}[f.Memories == 1]))
	}
	return out
}

func (f Found) unique() int {
	n := 0
	for _, sk := range f.Skills {
		if !sk.Dup {
			n++
		}
	}
	return n
}

// same files with the same bytes
func sameDir(a, b string) bool {
	count := 0
	err := filepath.WalkDir(a, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(a, p)
		x, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		y, err := os.ReadFile(filepath.Join(b, rel))
		if err != nil || string(x) != string(y) {
			return fs.ErrNotExist
		}
		count++
		return nil
	})
	if err != nil {
		return false
	}
	n := 0
	filepath.WalkDir(b, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n == count
}

func tilde(p, home string) string {
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return p
}

func size(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d bytes", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func rulesFile(e *Env, tool string) string {
	if t := Find(tool); t != nil && t.RulesFile != nil {
		return t.RulesFile(e)
	}
	return ""
}

func skillDirs(e *Env, tool string) []string {
	if t := Find(tool); t != nil && t.SkillFrom != nil {
		return t.SkillFrom(e)
	}
	return nil
}

// text outside our block, trimmed
func userText(s *State, p string) (string, error) {
	b, err := read(p)
	if err != nil || b == nil {
		return "", err
	}
	sep := ""
	if rec := s.Files[p]; rec != nil {
		sep = rec.Sep
	}
	return strings.TrimSpace(string(mdMarks.strip(b, sep))), nil
}

func toolLabel(name string) string {
	if t := Find(name); t != nil {
		return t.Label
	}
	return name
}

// Scan finds what import would move, without changing anything
func Scan(e *Env, s *State, vaultDir string, tools []string) (Found, error) {
	f := Found{tools: tools}
	f.Memories = countNewNative(e, vaultDir, tools)
	byText := map[string]int{}
	for _, tool := range tools {
		p := rulesFile(e, tool)
		// a file linked to another tool's file has the same text, so it
		// lands in the same group below and both get moved
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		txt, err := userText(s, p)
		if err != nil {
			return f, err
		}
		if txt == "" {
			continue
		}
		if i, ok := byText[txt]; ok {
			f.Rules[i].Tools = append(f.Rules[i].Tools, tool)
			f.Rules[i].Paths = append(f.Rules[i].Paths, p)
			continue
		}
		byText[txt] = len(f.Rules)
		f.Rules = append(f.Rules, RuleSrc{Tools: []string{tool}, Paths: []string{p}, Text: txt})
	}

	// skills: only real folders with a SKILL.md. links get followed so the
	// same skill isn't imported twice; links to anywhere else stay alone
	seen := map[string]bool{}
	byName := map[string]string{}
	scanned := map[string]bool{}
	var dirs []string
	var dirTool []string
	for _, tool := range tools {
		for _, d := range skillDirs(e, tool) {
			if !scanned[d] {
				scanned[d] = true
				dirs = append(dirs, d)
				dirTool = append(dirTool, tool)
			}
		}
	}
	vaultSkills := filepath.Join(vaultDir, "skills")
	if r, err := filepath.EvalSymlinks(vaultSkills); err == nil {
		vaultSkills = r // macos: /var is really /private/var
	}
	inScanned := func(p string) bool {
		for d := range scanned {
			if rd, err := filepath.EvalSymlinks(d); err == nil && filepath.Dir(p) == rd {
				return true
			}
		}
		return false
	}
	for i, d := range dirs {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, ent := range ents {
			p := filepath.Join(d, ent.Name())
			if _, ours := s.Links[p]; ours {
				continue
			}
			real, err := filepath.EvalSymlinks(p)
			if err != nil {
				continue // dangling link
			}
			fi, err := os.Stat(real)
			if err != nil || !fi.IsDir() || !exists(filepath.Join(real, "SKILL.md")) {
				continue
			}
			if seen[real] {
				continue
			}
			if strings.HasPrefix(real, vaultSkills+string(filepath.Separator)) {
				continue // already in the vault
			}
			if ent.Type()&fs.ModeSymlink != 0 && !inScanned(real) {
				seen[real] = true
				f.Linked++
				continue
			}
			seen[real] = true
			if prev, ok := byName[ent.Name()]; ok && sameDir(prev, real) {
				f.Skills = append(f.Skills, SkillSrc{Name: ent.Name(), Path: real, Tool: dirTool[i], Dup: true})
				continue
			}
			if prev, ok := byName[ent.Name()]; ok {
				f.Skipped = append(f.Skipped, fmt.Sprintf("%s: a different skill with this name is in %s, kept that one", tilde(real, e.Home), tilde(prev, e.Home)))
				continue
			}
			if exists(filepath.Join(vaultDir, "skills", ent.Name())) {
				f.Skipped = append(f.Skipped, tilde(real, e.Home)+": the vault already has a skill with this name")
				continue
			}
			byName[ent.Name()] = real
			f.Skills = append(f.Skills, SkillSrc{Name: ent.Name(), Path: real, Tool: dirTool[i]})
		}
	}
	return f, nil
}

// claude resolves `@relative` imports from the file they're in. once the text
// lives in the vault they'd break, so point the ones that exist at absolute paths
var importRe = regexp.MustCompile(`(^|[\s(])@((?:\\ |[^\s` + "`" + `)])+)`)

func absImports(text, base string) string {
	lines := strings.Split(text, "\n")
	fence := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		lines[i] = importRe.ReplaceAllStringFunc(l, func(m string) string {
			sub := importRe.FindStringSubmatch(m)
			pre, raw := sub[1], sub[2]
			if strings.HasPrefix(raw, "~") || filepath.IsAbs(strings.ReplaceAll(raw, `\ `, " ")) {
				return m
			}
			abs := filepath.Join(base, strings.ReplaceAll(raw, `\ `, " "))
			if !exists(abs) {
				return m // probably an @mention, not a file
			}
			return pre + claudeImport(abs)
		})
	}
	return strings.Join(lines, "\n")
}

// Import moves everything in f into the vault. run it before the tools get
// wired, so the rules files end up holding only our block
func Import(e *Env, s *State, f Found, tools []string, out io.Writer) error {
	if f.Empty() {
		return nil
	}
	if err := vault.Create(s.Vault); err != nil {
		return err
	}

	if len(f.Rules) > 0 {
		var parts []string
		for _, r := range f.Rules {
			var labels []string
			for _, t := range r.Tools {
				labels = append(labels, toolLabel(t))
			}
			parts = append(parts, "## from "+strings.Join(labels, ", ")+"\n\n"+absImports(r.Text, filepath.Dir(r.Paths[0])))
		}
		if err := vault.AddToMe(s.Vault, strings.Join(parts, "\n\n")); err != nil {
			return err
		}
		for _, r := range f.Rules {
			for j, p := range r.Paths {
				if err := moveOut(s, p, r.Tools[j], vault.RulesFiles(s.Vault)[1]); err != nil {
					return err
				}
				fmt.Fprintf(out, "  moved %s into me.md\n", tilde(p, e.Home))
			}
		}
	}

	for _, sk := range f.Skills {
		if err := moveSkill(s, sk); err != nil {
			fmt.Fprintf(out, "  warning: skill %s: %v\n", sk.Name, err)
			continue
		}
	}
	if n := f.unique(); n > 0 {
		fmt.Fprintf(out, "  moved %d skill%s into the vault\n", n, plural(n))
	}
	for _, sk := range f.Skipped {
		fmt.Fprintln(out, "  skipped:", sk)
	}
	if f.Linked > 0 {
		fmt.Fprintf(out, "  left %d skill%s alone: they're links to folders outside these tools\n", f.Linked, plural(f.Linked))
	}
	if lim := codexMax(func(e *Env) string { return e.CodexDir })(e); (s.HasTool("codex") || hasStr(tools, "codex")) && func() bool { r, err := vault.Rules(s.Vault); return err == nil && len(r) > lim-4096 }() {
		r, _ := vault.Rules(s.Vault)
		fmt.Fprintf(out, "  warning: the vault rules are %s now. codex stops reading at %s: trim me.md, or raise project_doc_max_bytes in ~/.codex/config.toml\n", size(len(r)), size(lim))
	}
	return s.Save()
}

// replace a rules file's text with a note block. the claude and codex
// wiring overwrite the note with their own block right after
func moveOut(s *State, p, tool, me string) error {
	txt, err := userText(s, p)
	if err != nil {
		return err
	}
	b, _ := read(p)
	body := "<!-- moved to " + filepath.ToSlash(me) + ". `aivault remove` puts it back -->"
	if s.Files[p] != nil && mdMarks.has(b) {
		body = mdMarks.body(b) // already wired, keep what's there
	}
	outB, _ := mdMarks.set(nil, body)
	if nl(b) == "\r\n" {
		outB = []byte(strings.ReplaceAll(string(outB), "\n", "\r\n"))
	}
	if err := s.write(p, outB); err != nil {
		return err
	}
	rec := s.Files[p]
	rec.Moved = txt
	rec.Sep = ""
	rec.Tool = tool
	return nil
}

// copy the folder into the vault, move the original aside. syncLinks then
// puts a link where it was
func moveSkill(s *State, sk SkillSrc) error {
	dst := filepath.Join(s.Vault, "skills", sk.Name)
	if !sk.Dup {
		if exists(dst) {
			return fmt.Errorf("%s already exists in the vault", sk.Name)
		}
		if err := copyDir(sk.Path, dst); err != nil {
			os.RemoveAll(dst)
			return err
		}
	}
	bdir, err := s.backupDir()
	if err != nil {
		return err
	}
	bp := filepath.Join(bdir, "skills", fmt.Sprintf("%d-%s", len(s.Imported), sk.Name))
	if err := os.MkdirAll(filepath.Dir(bp), 0o755); err != nil {
		return err
	}
	if err := os.Rename(sk.Path, bp); err != nil {
		// different disk: copy then delete
		if err := copyDir(sk.Path, bp); err != nil {
			return err
		}
		if err := os.RemoveAll(sk.Path); err != nil {
			return err
		}
	}
	s.Imported[sk.Path] = &ImportRec{Vault: dst, Backup: bp}
	return nil
}

// restoreSkills puts originals back wherever nothing of ours sits anymore
func restoreSkills(s *State) []string {
	var warn []string
	for orig, rec := range s.Imported {
		if _, err := os.Lstat(orig); err == nil {
			continue // our link (or something new) is still there
		}
		if err := os.Rename(rec.Backup, orig); err != nil {
			warn = append(warn, "can't put back "+orig+": "+err.Error()+". the original is at "+rec.Backup)
			continue
		}
		delete(s.Imported, orig)
	}
	return warn
}

func hasStr(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
