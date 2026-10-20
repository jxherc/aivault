package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// native memories: what tools save in their own memory files (claude code's
// auto memory, codex's MEMORY.md, qwen's memories/, hermes, dsh lessons).
// sync copies each entry into the vault so every tool can find it. copies,
// never moves: the tool keeps its own memory working

const (
	splitFile    = iota // the whole file is one memory (frontmatter dropped)
	splitHeading        // one memory per heading that starts with `prefix`
	splitSep            // entries separated by a line that is just `prefix`
	splitBullets        // one memory per top-level "- " bullet
)

type memSrc struct {
	files  func(e *Env) []string
	split  int
	prefix string
}

type nativeMem struct {
	tool, key, path, text string
	mod                   time.Time
}

// files matching a glob, skipping index files the tools rebuild themselves
func memGlob(pattern func(e *Env) string) func(e *Env) []string {
	return func(e *Env) []string {
		m, _ := filepath.Glob(pattern(e))
		var out []string
		for _, p := range m {
			if filepath.Base(p) != "MEMORY.md" {
				out = append(out, p)
			}
		}
		return out
	}
}

// every .md under a folder, any depth
func memTree(dir func(e *Env) string) func(e *Env) []string {
	return func(e *Env) []string {
		var out []string
		filepath.WalkDir(dir(e), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") && d.Name() != "MEMORY.md" {
				out = append(out, p)
			}
			return nil
		})
		return out
	}
}

func memFile(path func(e *Env) string) func(e *Env) []string {
	return func(e *Env) []string { return []string{path(e)} }
}

func stripFront(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if strings.HasPrefix(s, "---\n") {
		if i := strings.Index(s[4:], "\n---"); i >= 0 {
			s = s[4+i+4:]
		}
	}
	return strings.TrimSpace(s)
}

func splitMem(src memSrc, text string) [][2]string { // key, text
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out [][2]string
	add := func(key, body string) {
		if body = strings.TrimSpace(body); body != "" {
			out = append(out, [2]string{strings.TrimSpace(key), body})
		}
	}
	switch src.split {
	case splitFile:
		add("", stripFront(text))
	case splitHeading:
		var key, cur string
		started := false
		for _, l := range strings.Split(text, "\n") {
			if strings.HasPrefix(l, src.prefix) {
				if started {
					add(key, cur)
				}
				key, cur, started = l, l+"\n", true
				continue
			}
			if started {
				cur += l + "\n"
			}
		}
		if started {
			add(key, cur)
		}
	case splitSep:
		for _, part := range strings.Split(text, "\n"+src.prefix+"\n") {
			part = strings.TrimSpace(part)
			key := part
			if len(key) > 80 {
				key = key[:80]
			}
			add(key, part)
		}
	case splitBullets:
		var cur string
		flush := func() {
			k := cur
			if len(k) > 80 {
				k = k[:80]
			}
			add(k, cur)
			cur = ""
		}
		for _, l := range strings.Split(text, "\n") {
			switch {
			case strings.HasPrefix(l, "- "):
				flush()
				cur = strings.TrimPrefix(l, "- ")
			case cur != "" && strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "#"):
				cur += "\n" + l // a wrapped line still belongs to the bullet
			default:
				flush()
			}
		}
		flush()
	}
	return out
}

// looks like a password, key or token. those stay out of the vault, which
// syncs everywhere and gets read by every tool
var secretRe = regexp.MustCompile(`(?i)(\b(password|passwd|api[_ -]?key|secret|token|bearer)\b\s*[:=]\s*\S{6,})|(\bpassword\s+[A-Za-z0-9_\-!@#$%^&*]{6,})|(\bsk-[A-Za-z0-9_\-]{8,}[0-9][A-Za-z0-9_\-]{4,})|(gh[pousr]_[A-Za-z0-9]{20,})|(AKIA[0-9A-Z]{16})|(-----BEGIN [A-Z ]*PRIVATE KEY-----)|(xox[baprs]-[A-Za-z0-9-]{10,})`)

func looksSecret(s string) bool { return secretRe.MatchString(s) }

var tagClean = regexp.MustCompile(`[^a-z0-9_-]+`)

// collect reads every native memory of the given tools
func collectNative(e *Env, tools []string) []nativeMem {
	var out []nativeMem
	for _, name := range tools {
		t := Find(name)
		if t == nil || t.Memories == nil {
			continue
		}
		for _, src := range t.Memories(e) {
			for _, p := range src.files(e) {
				fi, err := os.Stat(p)
				if err != nil || fi.IsDir() {
					continue
				}
				b, err := os.ReadFile(p)
				if err != nil {
					continue
				}
				for _, kv := range splitMem(src, string(b)) {
					out = append(out, nativeMem{tool: name, key: kv[0], path: p, text: kv[1], mod: fi.ModTime()})
				}
			}
		}
	}
	return out
}

func nativeID(m nativeMem) string {
	h := sha256.Sum256([]byte(m.tool + "\x00" + m.path + "\x00" + m.key))
	return "n" + hex.EncodeToString(h[:])[:15]
}

// claude code keeps memories per project: ~/.claude/projects/<project>/memory
func projectTag(p string) string {
	dir := filepath.Base(filepath.Dir(filepath.Dir(p)))
	if filepath.Base(filepath.Dir(p)) != "memory" || dir == "." {
		return ""
	}
	parts := strings.Split(strings.Trim(dir, "-"), "-")
	return parts[len(parts)-1] // the last path segment is enough to tell projects apart
}

type syncReport struct {
	added, updated, same int
	skipped              []string // files
	skippedN             int      // entries
}

// syncNative copies native memories into the vault. stable ids, so a memory
// the tool rewrites gets updated in place instead of piling up copies
func syncNative(e *Env, s *State, tools []string, out io.Writer) syncReport {
	var r syncReport
	mems := collectNative(e, tools)
	byTool := map[string]int{}
	skipSeen := map[string]bool{}
	for _, m := range mems {
		if looksSecret(m.text) {
			r.skippedN++
			if !skipSeen[m.path] {
				r.skipped = append(r.skipped, m.path)
				skipSeen[m.path] = true
			}
			continue
		}
		id := nativeID(m)
		tags := []string{m.tool, "native"}
		if pt := projectTag(m.path); pt != "" {
			tags = append(tags, tagClean.ReplaceAllString(strings.ToLower(pt), "-"))
		}
		body := fmt.Sprintf("---\nid: %s\ncreated: %s\nfrom: %s\ntags: [%s]\nsynced_from: %s\n---\n%s\n",
			id, m.mod.UTC().Format(time.RFC3339), m.tool, strings.Join(tags, ", "), filepath.ToSlash(m.path), m.text)
		p := filepath.Join(s.Vault, "memory", id+".md")
		cur, err := os.ReadFile(p)
		switch {
		case err == nil && stripFront(string(cur)) == strings.TrimSpace(m.text):
			r.same++
			continue
		case err == nil:
			r.updated++
		default:
			r.added++
		}
		if err := writeAtomic(p, []byte(body)); err != nil {
			fmt.Fprintf(out, "  warning: can't write %s: %v\n", p, err)
			continue
		}
		byTool[m.tool]++
	}
	if r.added+r.updated > 0 {
		var names []string
		for t := range byTool {
			names = append(names, t)
		}
		sort.Strings(names)
		fmt.Fprintf(out, "  synced memories from %s: %d new, %d updated\n", strings.Join(names, ", "), r.added, r.updated)
	}
	if len(r.skipped) > 0 {
		fmt.Fprintf(out, "  left out %d memor%s that look like they hold a password or key. copy what you want by hand from:\n", r.skippedN, map[bool]string{true: "y", false: "ies"}[r.skippedN == 1])
		for _, p := range r.skipped {
			fmt.Fprintln(out, "    "+tilde(p, e.Home))
		}
	}
	return r
}

// for import's summary: how many native memories aren't in the vault yet
func countNewNative(e *Env, vaultDir string, tools []string) int {
	n := 0
	for _, m := range collectNative(e, tools) {
		if looksSecret(m.text) {
			continue
		}
		if _, err := os.Stat(filepath.Join(vaultDir, "memory", nativeID(m)+".md")); err != nil {
			n++
		}
	}
	return n
}
