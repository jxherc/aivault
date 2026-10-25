package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/jxherc/aivault/internal/wire"
)

// autosync runs `aivault watch --sync 1h` at login: launchd on mac, a
// systemd user service on linux, a logon task on windows

const agentLabel = "com.jxherc.aivault"

func cmdAutosync(args []string) error {
	e, s, err := load()
	if err != nil {
		return err
	}
	what := "status"
	if len(args) > 0 {
		what = args[0]
	}
	switch what {
	case "on":
		if s.Vault == "" {
			return errors.New("no vault set up. run `aivault init` first")
		}
		if err := autosyncOn(e); err != nil {
			return err
		}
		_, d := autosyncState(e)
		fmt.Println("autosync on: rules reach every tool within seconds of an edit, and a full sync runs every hour.", d)
	case "off":
		if err := autosyncOff(e); err != nil {
			return err
		}
		fmt.Println("autosync off")
	case "status":
		on, d := autosyncState(e)
		if !on {
			fmt.Println("off. `aivault autosync on` to start it")
			return nil
		}
		fmt.Println("on.", d)
	default:
		return errors.New("usage: aivault autosync on|off|status")
	}
	return nil
}

func watchLog(e *wire.Env) string { return filepath.Join(e.StateDir, "watch.log") }

func watchArgs(e *wire.Env) []string { return []string{e.Bin, "watch", "--sync", "1h"} }

func plistPath(e *wire.Env) string {
	return filepath.Join(e.Home, "Library", "LaunchAgents", agentLabel+".plist")
}

func unitPath(e *wire.Env) string {
	return filepath.Join(e.Home, ".config", "systemd", "user", "aivault.service")
}

func xmlEsc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func plist(e *wire.Env) string {
	var args strings.Builder
	for _, a := range watchArgs(e) {
		args.WriteString("    <string>" + xmlEsc(a) + "</string>\n")
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + agentLabel + `</string>
  <key>ProgramArguments</key>
  <array>
` + args.String() + `  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>` + xmlEsc(watchLog(e)) + `</string>
  <key>StandardErrorPath</key>
  <string>` + xmlEsc(watchLog(e)) + `</string>
</dict>
</plist>
`
}

func unit(e *wire.Env) string {
	var q []string
	for _, a := range watchArgs(e) {
		q = append(q, strconv.Quote(a))
	}
	return `[Unit]
Description=aivault: keep ai tools in sync with the vault

[Service]
ExecStart=` + strings.Join(q, " ") + `
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func autosyncOn(e *wire.Env) error {
	os.MkdirAll(e.StateDir, 0o755)
	switch runtime.GOOS {
	case "darwin":
		p := plistPath(e)
		autosyncOff(e) // reload cleanly if it was on
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(plist(e)), 0o644); err != nil {
			return err
		}
		return run("launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), p)
	case "windows":
		// powershell hidden, or a console window sits there all day
		q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		cmd := fmt.Sprintf(`powershell -NoProfile -WindowStyle Hidden -Command "& %s watch --sync 1h --log %s"`, q(e.Bin), q(watchLog(e)))
		if err := run("schtasks", "/Create", "/F", "/SC", "ONLOGON", "/TN", "aivault", "/TR", cmd); err != nil {
			return err
		}
		return run("schtasks", "/Run", "/TN", "aivault")
	default:
		if _, err := exec.LookPath("systemctl"); err != nil {
			return errors.New("no systemd here. run `aivault watch --sync 1h` from your login script instead")
		}
		p := unitPath(e)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(unit(e)), 0o644); err != nil {
			return err
		}
		if err := run("systemctl", "--user", "daemon-reload"); err != nil {
			return err
		}
		return run("systemctl", "--user", "enable", "--now", "aivault.service")
	}
}

func autosyncOff(e *wire.Env) error {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid())+"/"+agentLabel).Run() // fine if it wasn't loaded
		if err := os.Remove(plistPath(e)); err != nil && !os.IsNotExist(err) {
			return err
		}
	case "windows":
		exec.Command("schtasks", "/End", "/TN", "aivault").Run()
		if err := exec.Command("schtasks", "/Query", "/TN", "aivault").Run(); err == nil {
			return run("schtasks", "/Delete", "/F", "/TN", "aivault")
		}
	default:
		if _, err := os.Stat(unitPath(e)); err != nil {
			return nil
		}
		exec.Command("systemctl", "--user", "disable", "--now", "aivault.service").Run()
		if err := os.Remove(unitPath(e)); err != nil {
			return err
		}
		exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
	return nil
}

// on, and a line about it
func autosyncState(e *wire.Env) (bool, string) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := os.Stat(plistPath(e)); err != nil {
			return false, ""
		}
		out, err := exec.Command("launchctl", "print", "gui/"+strconv.Itoa(os.Getuid())+"/"+agentLabel).Output()
		if err != nil {
			return true, "installed but not loaded. `aivault autosync on` restarts it"
		}
		if strings.Contains(string(out), "state = running") {
			return true, "running. log: " + watchLog(e)
		}
		return true, "loaded, not running right now. log: " + watchLog(e)
	case "windows":
		if exec.Command("schtasks", "/Query", "/TN", "aivault").Run() != nil {
			return false, ""
		}
		return true, "logon task installed. log: " + watchLog(e)
	default:
		if _, err := os.Stat(unitPath(e)); err != nil {
			return false, ""
		}
		out, _ := exec.Command("systemctl", "--user", "is-active", "aivault.service").Output()
		return true, strings.TrimSpace(string(out)) + ". log: journalctl --user -u aivault"
	}
}
