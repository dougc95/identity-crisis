// Package setup installs and removes identity-tray's integration points: the
// git core.sshCommand hook, the seeded identities.toml, and the run-on-login
// registry value under HKCU. All registry writes are user-scoped (no admin).
package setup

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dougc95/identity-crisis/internal/identity"
	"golang.org/x/sys/windows/registry"
)

// RunKeyName is the HKCU Run value name for the tray's run-on-login entry.
const RunKeyName = "identity-tray"

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

const sharedEmail = "you@example.com"

// QuoteSSHCommand renders an executable path as a git core.sshCommand value:
// forward-slashed and double-quoted so git parses it as a single token.
func QuoteSSHCommand(path string) string {
	return `"` + strings.ReplaceAll(path, `\`, `/`) + `"`
}

// DefaultIdentities returns the seed identities for this machine's known accounts.
func DefaultIdentities() []identity.Identity {
	return []identity.Identity{
		{Label: "work-account", Key: "~/.ssh/id_work", Name: "work-account", Email: sharedEmail, SigningKey: "~/.ssh/id_work.pub", Owners: []string{"work-account"}},
		{Label: "personal-account", Key: "~/.ssh/id_personal", Name: "personal-account", Email: sharedEmail, SigningKey: "~/.ssh/id_personal.pub", Owners: []string{"personal-account"}},
		{Label: "side-project", Key: "~/.ssh/id_side", Name: "side-project", Email: sharedEmail, SigningKey: "~/.ssh/id_side.pub", Owners: []string{"side-project"}},
	}
}

// SeedConfig ensures the config directory and a default identities.toml exist.
// It never overwrites an existing file. created reports whether a file was written.
func SeedConfig(home string) (created bool, err error) {
	dir := identity.ConfigDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	path := identity.IdentitiesPath(home)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	var buf bytes.Buffer
	buf.WriteString("# identity-tray identities — edit freely.\n")
	buf.WriteString("# 'owners' lists the GitHub owners each identity is expected for;\n")
	buf.WriteString("# an empty list disables the mismatch warning for that identity.\n\n")
	if err := toml.NewEncoder(&buf).Encode(identity.Config{Identities: DefaultIdentities()}); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// SetGitSSHCommand points git's global core.sshCommand at the wrapper.
func SetGitSSHCommand(sshwrapPath string) error {
	return runGit("config", "--global", "core.sshCommand", QuoteSSHCommand(sshwrapPath))
}

// GetGitSSHCommand returns the current global core.sshCommand ("" if unset).
func GetGitSSHCommand() (string, error) {
	out, err := exec.Command("git", "config", "--global", "core.sshCommand").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "", nil // key not set
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// UnsetGitSSHCommand removes the global core.sshCommand setting.
func UnsetGitSSHCommand() error {
	out, err := exec.Command("git", "config", "--global", "--unset", "core.sshCommand").CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 5 {
			return nil // wasn't set
		}
		return fmt.Errorf("git --unset core.sshCommand: %v: %s", err, out)
	}
	return nil
}

// SetRunKey writes a HKCU Run value (run-on-login). It creates the Run key if
// it doesn't already exist (as on a fresh user profile).
func SetRunKey(name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

// GetRunKey reads a HKCU Run value; ok is false if the value (or the Run key
// itself) is absent.
func GetRunKey(name string) (value string, ok bool, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err == registry.ErrNotExist {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// DeleteRunKey removes a HKCU Run value (no error if the value or Run key is
// already absent).
func DeleteRunKey(name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// Install wires up the full integration: seed config, capture the prior
// core.sshCommand (once), point it at the wrapper, and register run-on-login.
func Install(home, sshwrapPath, trayPath string) error {
	if _, err := SeedConfig(home); err != nil {
		return err
	}
	prevPath := filepath.Join(identity.ConfigDir(home), "previous_sshcommand")
	if _, err := os.Stat(prevPath); os.IsNotExist(err) {
		prev, _ := GetGitSSHCommand()
		_ = os.WriteFile(prevPath, []byte(prev), 0o644)
	}
	if err := SetGitSSHCommand(sshwrapPath); err != nil {
		return err
	}
	return SetRunKey(RunKeyName, QuoteSSHCommand(trayPath))
}

// Uninstall reverts core.sshCommand to its captured prior value (or unsets it)
// and removes the run-on-login entry.
func Uninstall(home string) error {
	prevPath := filepath.Join(identity.ConfigDir(home), "previous_sshcommand")
	if b, err := os.ReadFile(prevPath); err == nil {
		if prev := strings.TrimSpace(string(b)); prev != "" {
			_ = runGit("config", "--global", "core.sshCommand", prev)
		} else {
			_ = UnsetGitSSHCommand()
		}
		_ = os.Remove(prevPath)
	} else {
		_ = UnsetGitSSHCommand()
	}
	return DeleteRunKey(RunKeyName)
}

func runGit(args ...string) error {
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return nil
}
