// Command tray is the system-tray UI for identity-tray. It lets the user pick
// an active git/ssh identity, reflects the current selection in its icon, can
// verify which GitHub account the active key authenticates as, warns on
// repo-owner mismatches surfaced by the ssh wrapper, and toggles run-on-login.
//
// This file is deliberately thin: all decision logic lives in the tested
// internal packages. The UI is verified manually.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"fyne.io/systray"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/toast.v1"

	"github.com/dougc95/identity-crisis/internal/icon"
	"github.com/dougc95/identity-crisis/internal/identity"
	"github.com/dougc95/identity-crisis/internal/setup"
	"github.com/dougc95/identity-crisis/internal/sshwrap"
)

const appID = "identity-tray"
const defaultSSH = `C:\Windows\System32\OpenSSH\ssh.exe`

func main() {
	// Optional one-shot install/uninstall from the command line.
	if len(os.Args) > 1 {
		home, _ := os.UserHomeDir()
		switch os.Args[1] {
		case "--install", "install":
			exe, _ := os.Executable()
			sshwrapPath := siblingExe(exe, "sshwrap.exe")
			if err := setup.Install(home, sshwrapPath, exe); err != nil {
				fmt.Fprintln(os.Stderr, "install failed:", err)
				os.Exit(1)
			}
			fmt.Println("identity-tray installed. core.sshCommand ->", sshwrapPath)
			return
		case "--uninstall", "uninstall":
			if err := setup.Uninstall(home); err != nil {
				fmt.Fprintln(os.Stderr, "uninstall failed:", err)
				os.Exit(1)
			}
			fmt.Println("identity-tray uninstalled.")
			return
		}
	}
	systray.Run(onReady, func() {})
}

func onReady() {
	home, _ := os.UserHomeDir()
	_, _ = setup.SeedConfig(home) // ensure config exists on first run

	cfg, err := identity.LoadConfig(identity.IdentitiesPath(home))
	if err != nil || len(cfg.Identities) == 0 {
		systray.SetTitle("identity-tray")
		systray.SetTooltip("identity-tray — no identities configured")
		broken := systray.AddMenuItem("No identities configured", "Edit identities.toml")
		go func() {
			for range broken.ClickedCh {
				openConfig(home)
			}
		}()
		addQuit()
		return
	}

	// Resolve the active identity, defaulting to the first if unset/unknown.
	active, _ := identity.ReadActive(identity.ActivePath(home))
	if _, ok := cfg.Find(active); !ok {
		active = cfg.Identities[0].Label
	}
	// Re-apply on startup so git config always matches the tray's state.
	if id, ok := cfg.Find(active); ok {
		_ = identity.Apply(id, home, runGit)
	}
	refreshIcon(active)

	items := make(map[string]*systray.MenuItem)
	for _, id := range cfg.Identities {
		it := systray.AddMenuItemCheckbox(id.Label, "Switch to "+id.Label, id.Label == active)
		items[id.Label] = it
		go func(id identity.Identity, it *systray.MenuItem) {
			for range it.ClickedCh {
				switchTo(home, id, items)
			}
		}(id, it)
	}

	systray.AddSeparator()
	verify := systray.AddMenuItem("Verify connection", "Check which GitHub account the active key authenticates as")
	openCfg := systray.AddMenuItem("Open config", "Edit identities.toml")
	runLogin := systray.AddMenuItemCheckbox("Run at login", "Start identity-tray when you log in", runOnLoginEnabled())
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Exit identity-tray")

	go watchEvents(home)

	go func() {
		for {
			select {
			case <-verify.ClickedCh:
				doVerify(home)
			case <-openCfg.ClickedCh:
				openConfig(home)
			case <-runLogin.ClickedCh:
				toggleRunLogin(runLogin)
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func switchTo(home string, id identity.Identity, items map[string]*systray.MenuItem) {
	if err := identity.Apply(id, home, runGit); err != nil {
		notify("identity-tray", "Failed to switch: "+err.Error())
		return
	}
	for label, it := range items {
		if label == id.Label {
			it.Check()
		} else {
			it.Uncheck()
		}
	}
	refreshIcon(id.Label)
	notify("Switched identity", "Now acting as "+id.Label)
}

func doVerify(home string) {
	active, err := identity.ReadActive(identity.ActivePath(home))
	if err != nil {
		notify("Verify", "No active identity set")
		return
	}
	cfg, err := identity.LoadConfig(identity.IdentitiesPath(home))
	if err != nil {
		notify("Verify", "Could not read config")
		return
	}
	id, ok := cfg.Find(active)
	if !ok {
		notify("Verify", "Active identity not found in config")
		return
	}
	key := identity.ExpandPath(id.Key)
	// ssh -T exits non-zero even on success, so we parse the output regardless.
	out, _ := exec.Command(sshPath(),
		"-T", "-i", key,
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"git@github.com",
	).CombinedOutput()

	if acct, ok := sshwrap.ParseGreeting(string(out)); ok {
		if acct == active {
			notify("Verify: "+active+" ✓", "GitHub sees you as "+acct)
		} else {
			notify("Verify: mismatch", "Active is "+active+" but GitHub sees "+acct)
		}
		return
	}
	notify("Verify failed", firstLine(string(out)))
}

func watchEvents(home string) {
	path := identity.EventsPath(home)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer w.Close()
	// Watch the directory so we catch creation of events.log too.
	if err := w.Add(identity.ConfigDir(home)); err != nil {
		return
	}
	offset := fileSize(path)
	for ev := range w.Events {
		if ev.Name != path {
			continue
		}
		if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
			continue
		}
		lines, newOffset := readFrom(path, offset)
		offset = newOffset
		for _, line := range lines {
			var e struct {
				Type   string `json:"type"`
				Active string `json:"active"`
				Owner  string `json:"owner"`
			}
			if json.Unmarshal([]byte(line), &e) == nil && e.Type == "mismatch" {
				notify("⚠ Wrong identity", "Active "+e.Active+" does not own "+e.Owner)
			}
		}
	}
}

func toggleRunLogin(it *systray.MenuItem) {
	if it.Checked() {
		if err := setup.DeleteRunKey(setup.RunKeyName); err == nil {
			it.Uncheck()
		}
		return
	}
	exe, _ := os.Executable()
	if err := setup.SetRunKey(setup.RunKeyName, setup.QuoteSSHCommand(exe)); err == nil {
		it.Check()
	}
}

func runOnLoginEnabled() bool {
	_, ok, _ := setup.GetRunKey(setup.RunKeyName)
	return ok
}

func openConfig(home string) {
	_ = exec.Command("cmd", "/c", "start", "", identity.IdentitiesPath(home)).Start()
}

func refreshIcon(active string) {
	label := active
	if label == "" {
		label = "?"
	}
	systray.SetIcon(icon.RenderICO(label, 32))
	systray.SetTitle("")
	systray.SetTooltip("identity-tray — " + label)
}

func notify(title, message string) {
	_ = (&toast.Notification{AppID: appID, Title: title, Message: message}).Push()
}

func addQuit() {
	quit := systray.AddMenuItem("Quit", "Exit identity-tray")
	go func() {
		<-quit.ClickedCh
		systray.Quit()
	}()
}

func runGit(args ...string) error {
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

func sshPath() string {
	if p := os.Getenv("IDENTITY_TRAY_SSH"); p != "" {
		return p
	}
	return defaultSSH
}

func siblingExe(exe, name string) string {
	dir := exe
	if i := lastSlash(exe); i >= 0 {
		dir = exe[:i]
	}
	return dir + string(os.PathSeparator) + name
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\\' || s[i] == '/' {
			return i
		}
	}
	return -1
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func readFrom(path string, offset int64) ([]string, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, offset
	}
	if fi.Size() < offset {
		offset = 0 // file was truncated/rotated
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, offset
	}
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, fi.Size()
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			return s[:i]
		}
	}
	if s == "" {
		return "ssh returned no output"
	}
	return s
}
