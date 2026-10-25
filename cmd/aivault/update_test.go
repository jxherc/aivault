package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxherc/aivault/internal/wire"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"0.10.0", "0.9.9", true}, {"v1.0.0", "1.0.0", false}, {"1.0.1", "1.0", true}, {"0.1.0", "0.2.0", false}, {"1.2.0", "1.2.0-rc1", false}} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestReleaseArchives(t *testing.T) {
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"README.md": "x", "aivault": "BIN"} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	if b, err := extract(tgz.Bytes(), "aivault_linux_amd64.tar.gz"); err != nil || string(b) != "BIN" {
		t.Fatalf("tar: %q %v", b, err)
	}

	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("aivault.exe")
	w.Write([]byte("EXE"))
	zw.Close()
	if b, err := extract(zb.Bytes(), "aivault_windows_amd64.zip"); err != nil || string(b) != "EXE" {
		t.Fatalf("zip: %q %v", b, err)
	}

	h := sha256.Sum256(tgz.Bytes())
	sums := hex.EncodeToString(h[:]) + "  aivault_linux_amd64.tar.gz\nabc  other.zip\n"
	if err := checkSum(tgz.Bytes(), []byte(sums), "aivault_linux_amd64.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if err := checkSum([]byte("tampered"), []byte(sums), "aivault_linux_amd64.tar.gz"); err == nil {
		t.Fatal("tampered archive passed")
	}
	if err := checkSum(tgz.Bytes(), []byte(sums), "aivault_darwin_arm64.tar.gz"); err == nil {
		t.Fatal("unlisted archive passed")
	}
}

func TestReplaceExe(t *testing.T) {
	p := filepath.Join(t.TempDir(), "aivault")
	os.WriteFile(p, []byte("old"), 0o755)
	if err := replaceExe(p, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" {
		t.Fatalf("got %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm()&0o100 == 0 && os.PathSeparator == '/' {
		t.Fatal("lost the exec bit")
	}
}

func TestManagedBy(t *testing.T) {
	for p, want := range map[string]bool{
		"/opt/homebrew/Caskroom/aivault/0.1.0/aivault":       true,
		`C:\Users\me\scoop\apps\aivault\current\aivault.exe`: true,
		"/usr/bin/aivault":                                       true,
		"/home/me/.local/bin/aivault":                            false,
		`C:\Users\me\AppData\Local\Programs\aivault\aivault.exe`: false,
	} {
		if got := managedBy(p) != ""; got != want {
			t.Errorf("%s: %v", p, got)
		}
	}
}

func TestServiceFiles(t *testing.T) {
	e := &wire.Env{Home: "/home/me", StateDir: "/home/me/.aivault", Bin: "/opt/a b/aivault"}
	p := plist(e)
	if !strings.Contains(p, "<string>/opt/a b/aivault</string>\n    <string>watch</string>") || !strings.Contains(p, filepath.Join("/home/me/.aivault", "watch.log")) {
		t.Fatalf("plist:\n%s", p)
	}
	if u := unit(e); !strings.Contains(u, `ExecStart="/opt/a b/aivault" "watch" "--sync" "1h"`) {
		t.Fatalf("unit:\n%s", u)
	}
}
