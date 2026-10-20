package wire

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"aivault/internal/vault"
)

// syncLinks makes every vault skill show up in each skills folder a wired
// tool reads, and removes links we made that aren't needed anymore.
// returns warnings, not errors: one bad skill shouldn't stop the rest
func syncLinks(e *Env, s *State) []string {
	var warn []string
	want := map[string]string{} // link path -> target
	skills, err := vault.Skills(s.Vault)
	if err != nil {
		return []string{"can't read vault skills: " + err.Error()}
	}
	for _, t := range All {
		if !s.HasTool(t.Name) {
			continue
		}
		dir := t.Skills(e)
		for _, sk := range skills {
			want[filepath.Join(dir, sk.Name)] = sk.Dir
		}
	}

	// drop links we own that aren't wanted
	for p, rec := range s.Links {
		if want[p] == rec.Target && !rec.Copied {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			warn = append(warn, "can't remove "+p+": "+err.Error())
			continue
		}
		delete(s.Links, p)
	}

	var taken []string
	for p, target := range want {
		if rec, ok := s.Links[p]; ok && rec.Target == target {
			continue
		}
		if _, err := os.Lstat(p); err == nil {
			taken = append(taken, p)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			warn = append(warn, err.Error())
			continue
		}
		copied, err := link(target, p)
		if err != nil {
			warn = append(warn, "can't link skill "+filepath.Base(p)+": "+err.Error())
			continue
		}
		s.Links[p] = &LinkRec{Target: target, Copied: copied}
	}
	// something already sits there. fine if it's the user's own link that ends
	// up at the vault skill anyway (checked last, since it may go through a link we just made)
	for _, p := range taken {
		real, err := filepath.EvalSymlinks(p)
		goal, _ := filepath.EvalSymlinks(want[p])
		if err == nil && real == goal {
			continue
		}
		warn = append(warn, fmt.Sprintf("skipped skill %s: %s already exists and isn't ours", filepath.Base(p), p))
	}
	return warn
}

// symlink, then a windows junction (no admin needed), then a plain copy
func link(target, p string) (copied bool, err error) {
	if err = os.Symlink(target, p); err == nil {
		return false, nil
	}
	if runtime.GOOS != "windows" {
		return false, err
	}
	if exec.Command("cmd", "/c", "mklink", "/J", p, target).Run() == nil {
		return false, nil
	}
	return true, copyDir(target, p)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		f, err := os.Create(out)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, in)
		return err
	})
}

func checkLinks(s *State) []string {
	var bad []string
	for p, rec := range s.Links {
		if _, err := os.Stat(p); err != nil {
			bad = append(bad, "skill link is broken: "+p)
			continue
		}
		if _, err := os.Stat(rec.Target); err != nil {
			bad = append(bad, "skill was removed from the vault but still linked: "+p+". run `aivault sync`")
		}
	}
	return bad
}
