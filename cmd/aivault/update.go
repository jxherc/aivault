package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

const repo = "jxherc/aivault"

// version from ldflags, or from the module when built with `go install ...@v1.2.3`
func curVersion() string {
	if version != "dev" {
		return strings.TrimPrefix(version, "v")
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "dev"
}

// a local build: no ldflags version, or a go pseudo-version like
// 0.1.1-0.20261026141233-c16c2b511288+dirty
var pseudoRe = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

func isDev(v string) bool {
	return v == "dev" || strings.Contains(v, "+dirty") || pseudoRe.MatchString(v)
}

type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

var errNoRelease = errors.New("no release published yet")

func latestRelease(ctx context.Context) (*release, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return nil, errNoRelease
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("github said %s", res.Status)
	}
	var r release
	return &r, json.NewDecoder(res.Body).Decode(&r)
}

// newer compares dotted versions like 0.10.1 > 0.9.3. anything odd = not newer
func newer(latest, cur string) bool {
	a, b := strings.Split(strings.TrimPrefix(latest, "v"), "."), strings.Split(strings.TrimPrefix(cur, "v"), ".")
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			fmt.Sscan(strings.SplitN(a[i], "-", 2)[0], &x)
		}
		if i < len(b) {
			fmt.Sscan(strings.SplitN(b[i], "-", 2)[0], &y)
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// a package manager owns the binary, so it should do the update
func managedBy(exe string) string {
	p := strings.ReplaceAll(strings.ToLower(exe), `\`, "/")
	switch {
	case strings.Contains(p, "/cellar/") || strings.Contains(p, "/caskroom/") || strings.Contains(p, "/homebrew/"):
		return "brew upgrade --cask aivault"
	case strings.Contains(p, "/scoop/"):
		return "scoop update aivault"
	case strings.HasPrefix(p, "/usr/bin/"):
		return "your package manager (or the install script again)"
	}
	return ""
}

func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	check := fs.Bool("check", false, "only say whether there's a newer version")
	force := fs.Bool("force", false, "update even a dev build or a package-manager install")
	fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	set, stop := spin("asking github for the latest release")
	defer stop()
	r, err := latestRelease(ctx)
	if err != nil {
		return err
	}
	cur, latest := curVersion(), strings.TrimPrefix(r.Tag, "v")
	if !isDev(cur) && !newer(latest, cur) && !*force {
		stop()
		fmt.Println(mark("ok") + " up to date " + dim.Render("("+cur+")"))
		return nil
	}
	if *check {
		stop()
		if isDev(cur) {
			fmt.Println(mark("info") + " this is a dev build. the latest release is " + latest)
		} else {
			fmt.Println(mark("warn") + " " + latest + " is out, you have " + cur + ". " + accent.Render("aivault update") + " gets it")
		}
		return nil
	}
	if *check {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	if how := managedBy(exe); how != "" && !*force {
		return fmt.Errorf("%s was installed by a package manager. update with %s", exe, how)
	}
	if isDev(cur) && !*force {
		return errors.New("this is a dev build. `aivault update --force` replaces it with the release")
	}

	name := assetName(runtime.GOOS, runtime.GOARCH)
	var archURL, sumURL string
	for _, a := range r.Assets {
		switch a.Name {
		case name:
			archURL = a.URL
		case "checksums.txt":
			sumURL = a.URL
		}
	}
	if archURL == "" || sumURL == "" {
		return fmt.Errorf("release %s has no %s", r.Tag, name)
	}
	set("downloading " + name)
	arch, err := fetch(ctx, archURL)
	if err != nil {
		return err
	}
	sums, err := fetch(ctx, sumURL)
	if err != nil {
		return err
	}
	if err := checkSum(arch, sums, name); err != nil {
		return err
	}
	bin, err := extract(arch, name)
	if err != nil {
		return err
	}
	if err := replaceExe(exe, bin); err != nil {
		return err
	}
	stop()
	fmt.Printf("%s updated %s → %s\n", mark("ok"), cur, bold.Render(latest))
	fmt.Println("  " + dim.Render("restart your ai tools so they launch the new mcp server"))
	return nil
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: %s", url, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 200<<20))
}

func checkSum(data, sums []byte, name string) error {
	h := sha256.Sum256(data)
	got := hex.EncodeToString(h[:])
	for _, l := range strings.Split(string(sums), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && f[1] == name {
			if f[0] != got {
				return fmt.Errorf("checksum mismatch for %s, not installing it", name)
			}
			return nil
		}
	}
	return fmt.Errorf("checksums.txt doesn't list %s", name)
}

// matches the archive names in .goreleaser.yaml
func assetName(goos, goarch string) string {
	os, arch, ext := goos, goarch, ".tar.gz"
	switch {
	case goos == "darwin" && goarch == "arm64":
		os, arch = "macos", "apple-silicon"
	case goos == "darwin":
		os, arch = "macos", "intel"
	case goarch == "amd64":
		arch = "x64"
	}
	if goos == "windows" {
		ext = ".zip"
	}
	return "aivault-" + os + "-" + arch + ext
}

// the aivault binary out of a release archive
func extract(arch []byte, name string) ([]byte, error) {
	want := "aivault"
	if strings.HasSuffix(name, ".zip") {
		want += ".exe"
		zr, err := zip.NewReader(bytes.NewReader(arch), int64(len(arch)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == want {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("%s has no %s", name, want)
	}
	gz, err := gzip.NewReader(bytes.NewReader(arch))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("%s has no %s", name, want)
		}
		if filepath.Base(h.Name) == want && h.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
}

// write next to the old one, then swap. windows won't delete a running exe
// but lets it be renamed, so the old one moves aside and goes on the next run
func replaceExe(exe string, bin []byte) error {
	fi, err := os.Stat(exe)
	if err != nil {
		return err
	}
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, bin, fi.Mode().Perm()|0o111); err != nil {
		return fmt.Errorf("can't write next to %s: %w", exe, err)
	}
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Rename(old, exe) // put it back
		return err
	}
	if runtime.GOOS != "windows" {
		os.Remove(old)
	}
	return nil
}

// leftovers from an update on windows
func cleanOld() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
	}
}
