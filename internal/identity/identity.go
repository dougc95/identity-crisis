// Package identity models the set of switchable git/ssh identities, the
// persisted "active" selection, and the git-config actions that applying an
// identity entails. It is shared by the tray (which writes state and applies
// git config) and the ssh wrapper (which reads state to pick a key).
package identity

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Identity is one switchable git/ssh identity.
type Identity struct {
	Label      string   `toml:"label"`       // menu label + active-state token
	Key        string   `toml:"key"`         // ssh private key path (may start with ~)
	Name       string   `toml:"name"`        // git user.name
	Email      string   `toml:"email"`       // git user.email
	SigningKey string   `toml:"signing_key"` // ssh public key path for signing (may start with ~)
	Owners     []string `toml:"owners"`      // GitHub owners this identity is expected for; empty disables mismatch check
}

// Config is the parsed identities.toml.
type Config struct {
	Identities []Identity `toml:"identity"`
}

// Find returns the identity with the given label.
func (c Config) Find(label string) (Identity, bool) {
	for _, id := range c.Identities {
		if id.Label == label {
			return id, true
		}
	}
	return Identity{}, false
}

// LoadConfig reads and parses identities.toml.
func LoadConfig(path string) (Config, error) {
	var c Config
	_, err := toml.DecodeFile(path, &c)
	return c, err
}

// GitConfigActions returns the argument lists to pass to `git` (one per action)
// that apply an identity's author and signing configuration globally.
func GitConfigActions(name, email, signingKey, allowedSigners string) [][]string {
	return [][]string{
		{"config", "--global", "user.name", name},
		{"config", "--global", "user.email", email},
		{"config", "--global", "user.signingkey", signingKey},
		{"config", "--global", "gpg.format", "ssh"},
		{"config", "--global", "commit.gpgsign", "true"},
		{"config", "--global", "gpg.ssh.allowedSignersFile", allowedSigners},
	}
}

// Apply persists the identity as active and applies its git author/signing
// configuration by invoking runGit for each action. It stops at the first error.
// runGit is injected so the effect can be tested without a real git.
func Apply(id Identity, home string, runGit func(args ...string) error) error {
	if err := WriteActive(ActivePath(home), id.Label); err != nil {
		return err
	}
	signingKey := ExpandHome(id.SigningKey, home)
	allowedSigners := ExpandHome("~/.ssh/allowed_signers", home)
	for _, action := range GitConfigActions(id.Name, id.Email, signingKey, allowedSigners) {
		if err := runGit(action...); err != nil {
			return err
		}
	}
	return nil
}

// WriteActive atomically records label as the active identity.
func WriteActive(path, label string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(label+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadActive returns the currently active identity label.
func ReadActive(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// ConfigDir returns the identity-tray config directory for a given home.
func ConfigDir(home string) string { return filepath.Join(home, ".identity-tray") }

// IdentitiesPath returns the identities.toml path.
func IdentitiesPath(home string) string { return filepath.Join(ConfigDir(home), "identities.toml") }

// ActivePath returns the active-state file path.
func ActivePath(home string) string { return filepath.Join(ConfigDir(home), "active") }

// EventsPath returns the mismatch event-log path.
func EventsPath(home string) string { return filepath.Join(ConfigDir(home), "events.log") }

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return expandHome(p, home)
}

// ExpandHome expands a leading ~ in p using the provided home directory.
func ExpandHome(p, home string) string { return expandHome(p, home) }

// expandHome expands a leading ~ in p using the provided home directory.
func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return home + p[1:]
	}
	return p
}
