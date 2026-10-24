package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// looks like a password, key or token. those stay out of the vault, which
// syncs everywhere and gets read by every tool
var secretRe = regexp.MustCompile(`(?i)(\b(password|passwd|api[_ -]?key|secret|token|bearer)\b\s*[:=]\s*\S{6,})|(\bpassword\s+[A-Za-z0-9_\-!@#$%^&*]{6,})|(\bsk-[A-Za-z0-9_\-]{8,}[0-9][A-Za-z0-9_\-]{4,})|(gh[pousr]_[A-Za-z0-9]{20,})|(AKIA[0-9A-Z]{16})|(-----BEGIN [A-Z ]*PRIVATE KEY-----)|(xox[baprs]-[A-Za-z0-9-]{10,})`)

func LooksSecret(s string) bool { return secretRe.MatchString(s) }

var noteName = regexp.MustCompile(`^[a-z0-9_-]+$`)

// NoteFile finds the vault file a note goes to: name.md at the top (loaded by
// every tool) or index/name.md (searched). it has to exist already, so a
// typo can't start a new file that every tool then loads
func NoteFile(dir, name string) (string, error) {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".md")
	if !noteName.MatchString(name) {
		return "", fmt.Errorf("%q isn't a vault file name", name)
	}
	for _, p := range []string{filepath.Join(dir, name+".md"), filepath.Join(dir, "index", name+".md")} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("no %s.md in the vault. files you can add to: %s", name, strings.Join(NoteFiles(dir), ", "))
}

// NoteFiles lists the names NoteFile accepts
func NoteFiles(dir string) []string {
	var out []string
	for _, pat := range []string{filepath.Join(dir, "*.md"), filepath.Join(dir, "index", "*.md")} {
		m, _ := filepath.Glob(pat)
		for _, p := range m {
			if n := strings.TrimSuffix(filepath.Base(p), ".md"); noteName.MatchString(n) {
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// AddNote appends "- text (date, from)" to a vault file and returns its path
func AddNote(dir, name, text, from string) (string, error) {
	text = strings.Join(strings.Fields(text), " ") // one line, so it stays one entry
	if text == "" {
		return "", errors.New("note text is empty")
	}
	if LooksSecret(text) {
		return "", errors.New("that looks like a password or key. the vault doesn't keep those")
	}
	p, err := NoteFile(dir, name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	stamp := time.Now().Format("2006-01-02")
	if f := tagOK.ReplaceAllString(strings.ToLower(from), "-"); f != "" {
		stamp += ", " + f
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	add := "- " + text + " (" + stamp + ")\n"
	if len(b) > 0 && b[len(b)-1] != '\n' {
		add = "\n" + add
	}
	_, err = f.WriteString(add)
	return p, err
}
