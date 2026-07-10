package identity

import (
	"os"
	"reflect"
	"testing"
)

func TestApplyWritesActiveAndGitConfig(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(ConfigDir(home), 0o755); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	runGit := func(args ...string) error {
		calls = append(calls, args)
		return nil
	}

	id := Identity{
		Label:      "personal-account",
		Name:       "personal-account",
		Email:      "you@example.com",
		SigningKey: "~/.ssh/id_personal.pub",
	}
	if err := Apply(id, home, runGit); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// active state persisted
	got, err := ReadActive(ActivePath(home))
	if err != nil || got != "personal-account" {
		t.Fatalf("active = %q (err %v), want personal-account", got, err)
	}

	// git config user.name set
	wantName := []string{"config", "--global", "user.name", "personal-account"}
	if !containsCall(calls, wantName) {
		t.Errorf("missing git call %v in %v", wantName, calls)
	}

	// signing key expanded to an absolute path (no leading ~)
	wantSigning := []string{"config", "--global", "user.signingkey", ExpandHome("~/.ssh/id_personal.pub", home)}
	if !containsCall(calls, wantSigning) {
		t.Errorf("missing expanded signingkey call %v in %v", wantSigning, calls)
	}
}

func TestApplyStopsOnGitError(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(ConfigDir(home), 0o755)
	runGit := func(args ...string) error { return os.ErrPermission }
	err := Apply(Identity{Label: "x"}, home, runGit)
	if err == nil {
		t.Error("Apply should return the git runner's error")
	}
}

func containsCall(calls [][]string, want []string) bool {
	for _, c := range calls {
		if reflect.DeepEqual(c, want) {
			return true
		}
	}
	return false
}
