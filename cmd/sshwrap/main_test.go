package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestMain lets this test binary double as a stub "real ssh": when SSHWRAP_STUB
// is set, it prints its args (pipe-joined) and exits with SSHWRAP_STUB_EXIT.
func TestMain(m *testing.M) {
	if os.Getenv("SSHWRAP_STUB") == "1" {
		_, _ = os.Stdout.WriteString(strings.Join(os.Args[1:], "|"))
		code, _ := strconv.Atoi(os.Getenv("SSHWRAP_STUB_EXIT"))
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// writeHome creates a temp home with a config + active identity and returns it.
func writeHome(t *testing.T, activeLabel string, keyExists bool) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".identity-tray")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(home, ".ssh", "id_work")
	if keyExists {
		if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyPath, []byte("KEY"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := `
[[identity]]
label       = "work-account"
key         = "` + filepath.ToSlash(keyPath) + `"
name        = "work-account"
email       = "you@example.com"
signing_key = "` + filepath.ToSlash(keyPath) + `.pub"
owners      = ["work-account"]
`
	if err := os.WriteFile(filepath.Join(dir, "identities.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte(activeLabel+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestDecideInjectsKeyForManagedHost(t *testing.T) {
	home := writeHome(t, "work-account", true)
	args := []string{"github.com", "git-upload-pack 'work-account/Dual-Website.git'"}

	final, mismatch, keyMissing := decide(args, home)

	if keyMissing != "" {
		t.Fatalf("unexpected keyMissing: %q", keyMissing)
	}
	if mismatch != "" {
		t.Errorf("unexpected mismatch: %q", mismatch)
	}
	joined := strings.Join(final, " ")
	if !strings.Contains(joined, "-i ") || !strings.Contains(joined, "IdentitiesOnly=yes") {
		t.Errorf("expected key injection, got %v", final)
	}
	if final[len(final)-2] != "github.com" {
		t.Errorf("original args not preserved after injection: %v", final)
	}
}

func TestDecidePassthroughForAliasHost(t *testing.T) {
	home := writeHome(t, "work-account", true)
	args := []string{"github-work", "git-upload-pack 'work-account/Dual-Website.git'"}

	final, _, _ := decide(args, home)

	if strings.Contains(strings.Join(final, " "), "-i ") {
		t.Errorf("alias host should pass through untouched, got %v", final)
	}
	if len(final) != len(args) {
		t.Errorf("alias args changed: %v", final)
	}
}

func TestDecideWarnsOnMismatch(t *testing.T) {
	home := writeHome(t, "work-account", true)
	// active identity owns work-account, but we contact personal-account's repo
	args := []string{"github.com", "git-upload-pack 'personal-account/secret.git'"}

	_, mismatch, _ := decide(args, home)

	if mismatch != "personal-account" {
		t.Errorf("expected mismatch owner personal-account, got %q", mismatch)
	}
}

func TestDecideFailSafeWhenNoActive(t *testing.T) {
	home := t.TempDir() // no .identity-tray at all
	args := []string{"github.com", "git-upload-pack 'x/y.git'"}

	final, mismatch, keyMissing := decide(args, home)

	if !equal(final, args) {
		t.Errorf("expected passthrough when state missing, got %v", final)
	}
	if mismatch != "" || keyMissing != "" {
		t.Errorf("expected no warnings on fail-safe passthrough")
	}
}

func TestDecideSignalsMissingKey(t *testing.T) {
	home := writeHome(t, "work-account", false) // key file absent
	args := []string{"github.com", "git-upload-pack 'work-account/r.git'"}

	final, _, keyMissing := decide(args, home)

	if keyMissing == "" {
		t.Error("expected keyMissing to be set when key file is absent")
	}
	if !equal(final, args) {
		t.Errorf("expected passthrough (no -i) when key missing, got %v", final)
	}
}

func TestRunProxiesExitCodeAndArgs(t *testing.T) {
	home := writeHome(t, "work-account", true)
	t.Setenv("SSHWRAP_STUB", "1")
	t.Setenv("SSHWRAP_STUB_EXIT", "7")

	var out, errBuf bytes.Buffer
	e := env{
		realSSH: os.Args[0], // this test binary, acting as stub ssh
		home:    home,
		stdin:   strings.NewReader(""),
		stdout:  &out,
		stderr:  &errBuf,
	}
	args := []string{"github.com", "git-upload-pack 'work-account/Dual-Website.git'"}

	code := run(args, e)

	if code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
	stubSaw := out.String()
	if !strings.Contains(stubSaw, "-i|") || !strings.Contains(stubSaw, "IdentitiesOnly=yes") {
		t.Errorf("stub ssh did not receive injected key args: %q", stubSaw)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
