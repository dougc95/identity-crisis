package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dougc95/identity-crisis/internal/identity"
)

func TestQuoteSSHCommand(t *testing.T) {
	in := `C:\Users\you\Code\identity-tray\bin\sshwrap.exe`
	want := `"C:/Users/you/Code/identity-tray/bin/sshwrap.exe"`
	if got := QuoteSSHCommand(in); got != want {
		t.Errorf("QuoteSSHCommand(%q) = %q, want %q", in, got, want)
	}
}

func TestDefaultIdentities(t *testing.T) {
	ids := DefaultIdentities()
	if len(ids) != 3 {
		t.Fatalf("got %d default identities, want 3", len(ids))
	}
	byLabel := map[string]identity.Identity{}
	for _, id := range ids {
		byLabel[id.Label] = id
		if id.Email != "you@example.com" {
			t.Errorf("%s email = %q, want shared you@example.com", id.Label, id.Email)
		}
	}
	dual, ok := byLabel["work-account"]
	if !ok {
		t.Fatal("missing work-account identity")
	}
	if dual.Key != "~/.ssh/id_work" {
		t.Errorf("dual key = %q", dual.Key)
	}
	if len(dual.Owners) != 1 || dual.Owners[0] != "work-account" {
		t.Errorf("dual owners = %v", dual.Owners)
	}
}

func TestSeedConfigCreatesThenIsIdempotent(t *testing.T) {
	home := t.TempDir()

	created, err := SeedConfig(home)
	if err != nil {
		t.Fatalf("SeedConfig: %v", err)
	}
	if !created {
		t.Error("first SeedConfig should report created=true")
	}

	// The seeded file must parse back into 3 identities.
	cfg, err := identity.LoadConfig(identity.IdentitiesPath(home))
	if err != nil {
		t.Fatalf("LoadConfig of seeded file: %v", err)
	}
	if len(cfg.Identities) != 3 {
		t.Fatalf("seeded config has %d identities, want 3", len(cfg.Identities))
	}

	// A second call must not overwrite an existing file.
	path := identity.IdentitiesPath(home)
	if err := os.WriteFile(path, []byte("# user edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, err = SeedConfig(home)
	if err != nil {
		t.Fatalf("second SeedConfig: %v", err)
	}
	if created {
		t.Error("second SeedConfig should report created=false")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "# user edited\n" {
		t.Errorf("SeedConfig overwrote an existing file: %q", string(b))
	}
}

func TestRunKeyRoundTrip(t *testing.T) {
	const name = "identity-tray-test-DELETE-ME"
	value := `"C:/tmp/tray.exe"`
	t.Cleanup(func() { _ = DeleteRunKey(name) })

	if err := SetRunKey(name, value); err != nil {
		t.Fatalf("SetRunKey: %v", err)
	}
	got, ok, err := GetRunKey(name)
	if err != nil {
		t.Fatalf("GetRunKey: %v", err)
	}
	if !ok || got != value {
		t.Fatalf("GetRunKey = (%q, %v), want (%q, true)", got, ok, value)
	}

	if err := DeleteRunKey(name); err != nil {
		t.Fatalf("DeleteRunKey: %v", err)
	}
	if _, ok, _ := GetRunKey(name); ok {
		t.Error("run key still present after delete")
	}
}

func TestSeedConfigWritesUnderConfigDir(t *testing.T) {
	home := t.TempDir()
	if _, err := SeedConfig(home); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".identity-tray", "identities.toml")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected seeded file at %s: %v", want, err)
	}
}
