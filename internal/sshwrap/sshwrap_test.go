package sshwrap

import (
	"reflect"
	"testing"
)

func TestIsManaged(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"github.com", true},
		{"GitHub.com", true}, // case-insensitive
		{"github-work", false},
		{"github-side", false},
		{"gitlab.com", false},
		{"ssh.github.com", false},
		{"example.com", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsManaged(c.host); got != c.want {
			t.Errorf("IsManaged(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestFindHost(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantHost string
		wantIdx  int
		wantOK   bool
	}{
		{
			name:     "plain host and command",
			args:     []string{"github.com", "git-upload-pack 'owner/repo.git'"},
			wantHost: "github.com",
			wantIdx:  0,
			wantOK:   true,
		},
		{
			name:     "leading -o option before host",
			args:     []string{"-o", "SendEnv=GIT_PROTOCOL", "github.com", "git-upload-pack 'owner/repo.git'"},
			wantHost: "github.com",
			wantIdx:  2,
			wantOK:   true,
		},
		{
			name:     "flag with attached value and valueless flags",
			args:     []string{"-4", "-p", "22", "github-work", "git-receive-pack 'o/r.git'"},
			wantHost: "github-work",
			wantIdx:  3,
			wantOK:   true,
		},
		{
			name:     "user@host form",
			args:     []string{"git@github.com", "git-upload-pack 'owner/repo.git'"},
			wantHost: "git@github.com",
			wantIdx:  0,
			wantOK:   true,
		},
		{
			name:   "no host present",
			args:   []string{"-v", "-o", "BatchMode=yes"},
			wantOK: false,
		},
		{
			name:   "empty args",
			args:   []string{},
			wantOK: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host, idx, ok := FindHost(c.args)
			if ok != c.wantOK {
				t.Fatalf("FindHost(%v) ok = %v, want %v", c.args, ok, c.wantOK)
			}
			if !ok {
				return
			}
			if host != c.wantHost || idx != c.wantIdx {
				t.Errorf("FindHost(%v) = (%q, %d), want (%q, %d)", c.args, host, idx, c.wantHost, c.wantIdx)
			}
		})
	}
}

func TestHostname(t *testing.T) {
	// Hostname strips an optional user@ prefix so IsManaged can be applied.
	cases := []struct {
		in   string
		want string
	}{
		{"github.com", "github.com"},
		{"git@github.com", "github.com"},
		{"git@github-work", "github-work"},
	}
	for _, c := range cases {
		if got := Hostname(c.in); got != c.want {
			t.Errorf("Hostname(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseOwner(t *testing.T) {
	cases := []struct {
		name      string
		command   string
		wantOwner string
		wantOK    bool
	}{
		{"upload-pack single quotes", "git-upload-pack 'work-account/Dual-Website.git'", "work-account", true},
		{"receive-pack single quotes", "git-receive-pack 'personal-account/foo.git'", "personal-account", true},
		{"double quotes", `git-upload-pack "owner/repo.git"`, "owner", true},
		{"no quotes", "git-upload-pack owner/repo.git", "owner", true},
		{"leading slash", "git-upload-pack '/owner/repo.git'", "owner", true},
		{"nested path uses first segment", "git-upload-pack 'owner/group/repo.git'", "owner", true},
		{"no .git suffix", "git-upload-pack 'owner/repo'", "owner", true},
		{"unrecognized command", "echo hello", "", false},
		{"empty", "", "", false},
		{"pack command but no path", "git-upload-pack", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			owner, ok := ParseOwner(c.command)
			if ok != c.wantOK {
				t.Fatalf("ParseOwner(%q) ok = %v, want %v", c.command, ok, c.wantOK)
			}
			if owner != c.wantOwner {
				t.Errorf("ParseOwner(%q) = %q, want %q", c.command, owner, c.wantOwner)
			}
		})
	}
}

func TestInjectKey(t *testing.T) {
	orig := []string{"github.com", "git-upload-pack 'owner/repo.git'"}
	got := InjectKey(orig, `C:\Users\you\.ssh\id_work`)
	want := []string{
		"-i", `C:\Users\you\.ssh\id_work`,
		"-o", "IdentitiesOnly=yes",
		"github.com", "git-upload-pack 'owner/repo.git'",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("InjectKey() = %v, want %v", got, want)
	}
}

func TestInjectKeyDoesNotMutateInput(t *testing.T) {
	orig := []string{"github.com", "cmd"}
	_ = InjectKey(orig, "key")
	if !reflect.DeepEqual(orig, []string{"github.com", "cmd"}) {
		t.Errorf("InjectKey mutated its input: %v", orig)
	}
}

func TestOwnerAllowed(t *testing.T) {
	cases := []struct {
		name   string
		owner  string
		owners []string
		want   bool
	}{
		{"match", "work-account", []string{"work-account"}, true},
		{"case-insensitive match", "Work-Account", []string{"work-account"}, true},
		{"no match", "someoneelse", []string{"work-account"}, false},
		{"empty owners disables check (always allowed)", "anyone", []string{}, true},
		{"empty owners nil disables check", "anyone", nil, true},
		{"multiple owners", "second", []string{"first", "second"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OwnerAllowed(c.owner, c.owners); got != c.want {
				t.Errorf("OwnerAllowed(%q, %v) = %v, want %v", c.owner, c.owners, got, c.want)
			}
		})
	}
}
