package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State is everything we changed, so remove can put it back.
// lives at ~/.aivault/config.json
type State struct {
	Vault     string              `json:"vault"`
	Tools     []string            `json:"tools"`
	Files     map[string]*FileRec `json:"files"`
	Links     map[string]*LinkRec `json:"links"`
	ClaudeMCP bool                `json:"claude_mcp,omitempty"`
	// skills moved into the vault: original path -> where the original sits now
	Imported map[string]*ImportRec `json:"imported,omitempty"`

	path   string
	backup string // this run's backup folder, made on first use
}

type FileRec struct {
	Created bool     `json:"created"`          // file didn't exist before us
	Backup  string   `json:"backup,omitempty"` // copy of the original
	Sum     string   `json:"sum"`              // sha256 of what we last wrote
	Sep     string   `json:"sep,omitempty"`    // what we put before our block
	Moved   string   `json:"moved,omitempty"`  // text import moved into the vault, put back on undo
	Link    string   `json:"link,omitempty"`   // the file was a symlink to this
	Tool    string   `json:"tool,omitempty"`
	Line    string   `json:"line,omitempty"`  // yaml: the user's line we rewrote, like `mcp_servers: {}`
	Added   []string `json:"added,omitempty"` // json keys we set only because they were missing
}

type ImportRec struct {
	Vault  string `json:"vault"`  // copy in the vault
	Backup string `json:"backup"` // the original folder, moved aside
}

type LinkRec struct {
	Target string `json:"target"`
	Copied bool   `json:"copied,omitempty"` // windows fallback when links fail
}

func LoadState(e *Env) (*State, error) {
	s := &State{Files: map[string]*FileRec{}, Links: map[string]*LinkRec{}, Imported: map[string]*ImportRec{}, path: filepath.Join(e.StateDir, "config.json")}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("%s is broken: %w", s.path, err)
	}
	if s.Files == nil {
		s.Files = map[string]*FileRec{}
	}
	if s.Links == nil {
		s.Links = map[string]*LinkRec{}
	}
	if s.Imported == nil {
		s.Imported = map[string]*ImportRec{}
	}
	return s, nil
}

func (s *State) Save() error {
	if len(s.Tools) == 0 && len(s.Files) == 0 && len(s.Links) == 0 && len(s.Imported) == 0 && !s.ClaudeMCP {
		// nothing wired anymore. keep backups, drop the config
		err := os.Remove(s.path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path, append(b, '\n'))
}

func (s *State) HasTool(t string) bool {
	for _, x := range s.Tools {
		if x == t {
			return true
		}
	}
	return false
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// read returns the file, or nil if it's not there
func read(p string) ([]byte, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// write saves a file we're changing. first time we touch a file, the
// original gets copied to the backup folder
func (s *State) write(p string, data []byte) error {
	rec, ok := s.Files[p]
	if !ok {
		rec = &FileRec{}
		orig, err := os.ReadFile(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			rec.Created = true
		case err != nil:
			return err
		default:
			if fi, err := os.Lstat(p); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
				// we replace the link with a file. undo puts the link back
				rec.Link, _ = os.Readlink(p)
			}
			bdir, err := s.backupDir()
			if err != nil {
				return err
			}
			bp := filepath.Join(bdir, fmt.Sprintf("%d-%s", len(s.Files), filepath.Base(p)))
			if err := os.WriteFile(bp, orig, 0o600); err != nil {
				return err
			}
			rec.Backup = bp
		}
		s.Files[p] = rec
	}
	if err := writeAtomic(p, data); err != nil {
		return err
	}
	rec.Sum = sum(data)
	return nil
}

// this run's backup folder
func (s *State) backupDir() (string, error) {
	if s.backup == "" {
		s.backup = filepath.Join(filepath.Dir(s.path), "backups", time.Now().Format("20060102-150405"))
	}
	return s.backup, os.MkdirAll(s.backup, 0o755)
}

// undo takes our change out of p. if nobody touched the file since our last
// write, the original comes back byte for byte. otherwise strip cuts out only our part
func (s *State) undo(p string, strip func([]byte) ([]byte, error)) error {
	rec, ok := s.Files[p]
	if !ok {
		return nil
	}
	cur, err := read(p)
	if err != nil {
		return err
	}
	switch {
	case cur == nil:
		// already gone, nothing to do
	case sum(cur) == rec.Sum && rec.Created:
		if err := os.Remove(p); err != nil {
			return err
		}
	case sum(cur) == rec.Sum && rec.Link != "":
		if err := os.Remove(p); err != nil {
			return err
		}
		if err := os.Symlink(rec.Link, p); err != nil {
			return err
		}
	case sum(cur) == rec.Sum && rec.Backup != "":
		orig, err := os.ReadFile(rec.Backup)
		if err != nil {
			return fmt.Errorf("backup for %s is missing: %w", p, err)
		}
		if err := writeAtomic(p, orig); err != nil {
			return err
		}
	default:
		out, err := strip(cur)
		if err != nil {
			return err
		}
		if rec.Moved != "" {
			// the user edited the file since, so keep their edits and put the moved text back on top
			rest := strings.TrimLeft(string(out), "\r\n")
			out = []byte(rec.Moved)
			if rest != "" {
				out = append(out, []byte("\n\n"+rest)...)
			}
		}
		if rec.Created && len(out) == 0 {
			err = os.Remove(p)
		} else {
			err = writeAtomic(p, out)
		}
		if err != nil {
			return err
		}
	}
	delete(s.Files, p)
	return nil
}

// temp file + rename, so a crash never leaves half a config
func writeAtomic(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(p); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".aivault-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), mode)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), p)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
