package identity

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigFind(t *testing.T) {
	c := Config{Identities: []Identity{
		{Label: "work-account", Key: "~/.ssh/id_work"},
		{Label: "personal-account", Key: "~/.ssh/id_personal"},
	}}

	got, ok := c.Find("personal-account")
	if !ok {
		t.Fatal("Find(personal-account) not found")
	}
	if got.Key != "~/.ssh/id_personal" {
		t.Errorf("Find returned wrong identity: %+v", got)
	}

	if _, ok := c.Find("nope"); ok {
		t.Error("Find(nope) should not be found")
	}
}

func TestExpandHome(t *testing.T) {
	home := "/home/doug"
	cases := []struct {
		in   string
		want string
	}{
		{"~/.ssh/id_personal", "/home/doug/.ssh/id_personal"},
		{"~", "/home/doug"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}
	for _, c := range cases {
		if got := expandHome(c.in, home); got != c.want {
			t.Errorf("expandHome(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGitConfigActions(t *testing.T) {
	got := GitConfigActions("personal-account", "you@example.com", "/home/doug/.ssh/id_personal.pub", "/home/doug/.ssh/allowed_signers")
	want := [][]string{
		{"config", "--global", "user.name", "personal-account"},
		{"config", "--global", "user.email", "you@example.com"},
		{"config", "--global", "user.signingkey", "/home/doug/.ssh/id_personal.pub"},
		{"config", "--global", "gpg.format", "ssh"},
		{"config", "--global", "commit.gpgsign", "true"},
		{"config", "--global", "gpg.ssh.allowedSignersFile", "/home/doug/.ssh/allowed_signers"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GitConfigActions mismatch:\n got %v\nwant %v", got, want)
	}
}

func TestActiveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active")

	if _, err := ReadActive(path); err == nil {
		t.Error("ReadActive on missing file should error")
	}

	if err := WriteActive(path, "work-account"); err != nil {
		t.Fatalf("WriteActive: %v", err)
	}
	got, err := ReadActive(path)
	if err != nil {
		t.Fatalf("ReadActive: %v", err)
	}
	if got != "work-account" {
		t.Errorf("ReadActive = %q, want %q", got, "work-account")
	}

	// Overwriting must replace (atomic rename over existing file).
	if err := WriteActive(path, "personal-account"); err != nil {
		t.Fatalf("WriteActive overwrite: %v", err)
	}
	got, _ = ReadActive(path)
	if got != "personal-account" {
		t.Errorf("after overwrite ReadActive = %q, want %q", got, "personal-account")
	}

	// No leftover temp file.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind")
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identities.toml")
	content := `
[[identity]]
label       = "work-account"
key         = "~/.ssh/id_work"
name        = "work-account"
email       = "you@example.com"
signing_key = "~/.ssh/id_work.pub"
owners      = ["work-account", "some-org"]

[[identity]]
label = "personal-account"
key   = "~/.ssh/id_personal"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	c, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(c.Identities) != 2 {
		t.Fatalf("got %d identities, want 2", len(c.Identities))
	}
	first := c.Identities[0]
	if first.Label != "work-account" || first.Email != "you@example.com" {
		t.Errorf("first identity parsed wrong: %+v", first)
	}
	if !reflect.DeepEqual(first.Owners, []string{"work-account", "some-org"}) {
		t.Errorf("owners parsed wrong: %v", first.Owners)
	}
}

func TestConfigDirPaths(t *testing.T) {
	home := filepath.FromSlash("/home/doug")
	dir := ConfigDir(home)
	if dir != filepath.Join(home, ".identity-tray") {
		t.Errorf("ConfigDir = %q", dir)
	}
	if IdentitiesPath(home) != filepath.Join(dir, "identities.toml") {
		t.Errorf("IdentitiesPath wrong")
	}
	if ActivePath(home) != filepath.Join(dir, "active") {
		t.Errorf("ActivePath wrong")
	}
	if EventsPath(home) != filepath.Join(dir, "events.log") {
		t.Errorf("EventsPath wrong")
	}
}
