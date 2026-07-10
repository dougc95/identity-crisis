package sshwrap

import "testing"

func TestParseGreeting(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantOK  bool
	}{
		{
			name:   "standard github greeting",
			in:     "Hi personal-account! You've successfully authenticated, but GitHub does not provide shell access.",
			want:   "personal-account",
			wantOK: true,
		},
		{
			name:   "greeting on stderr with surrounding noise",
			in:     "Warning: Permanently added...\nHi work-account! You've successfully authenticated, but GitHub does not provide shell access.\n",
			want:   "work-account",
			wantOK: true,
		},
		{
			name:   "permission denied",
			in:     "git@github.com: Permission denied (publickey).",
			want:   "",
			wantOK: false,
		},
		{
			name:   "empty",
			in:     "",
			want:   "",
			wantOK: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseGreeting(c.in)
			if ok != c.wantOK || got != c.want {
				t.Errorf("ParseGreeting() = (%q, %v), want (%q, %v)", got, ok, c.want, c.wantOK)
			}
		})
	}
}
