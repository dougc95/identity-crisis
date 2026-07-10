package identity

import (
	"path/filepath"
	"testing"
)

// The committed example config must always parse and expose usable identities,
// so the documentation can't silently drift into an invalid state.
func TestExampleConfigParses(t *testing.T) {
	path := filepath.Join("..", "..", "identities.example.toml")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("identities.example.toml failed to parse: %v", err)
	}
	if len(cfg.Identities) == 0 {
		t.Fatal("identities.example.toml has no identities")
	}
	for _, id := range cfg.Identities {
		if id.Label == "" || id.Key == "" || id.SigningKey == "" {
			t.Errorf("example identity missing required field: %+v", id)
		}
	}
}
