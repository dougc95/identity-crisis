# identity-crisis

[![CI](https://github.com/dougc95/identity-crisis/actions/workflows/ci.yml/badge.svg)](https://github.com/dougc95/identity-crisis/actions/workflows/ci.yml)

*For when git can't decide who you are.* A Windows system-tray app (binary +
config namespace: `identity-tray`) that switches the **active git/ssh identity**
with one click. Picking an identity sets, globally:

- the **SSH key** used to authenticate git pushes/pulls/clones,
- the **commit author** (`user.name` / `user.email`), and
- the **SSH commit signing key** (`user.signingkey` + `gpg.format=ssh`).

Built for juggling multiple GitHub accounts (e.g. `work-account`,
`personal-account`, `side-project`) that each use a different SSH key — no more silent
"wrong account" pushes, and no host-alias gymnastics for the common case.

## How it works

git invokes SSH for every network operation and passes it the repo path. By
pointing git's `core.sshCommand` at a small wrapper (`sshwrap.exe`), that wrapper
becomes the single chokepoint where:

1. the **active key is injected** (`-i <key> -o IdentitiesOnly=yes`) for
   `github.com`, and
2. the **repo owner is checked** against the active identity — if they don't
   match, it warns (to stderr *and* a tray toast) but **never blocks** git.

```
tray.exe  ── writes ─▶  ~/.identity-tray/active   (+ git config user.*)
git push  ──▶  core.sshCommand = sshwrap.exe  ──▶  real ssh.exe -i <active key>
                              │
                              └─ owner mismatch? → warn (stderr + events.log → toast)
```

Host **aliases** (`github-work`, etc.) pass through untouched, so any repo you
pinned to a specific account keeps that pin regardless of the tray selection.

## Build

Requires Go 1.24+.

```sh
go build -o bin/sshwrap.exe ./cmd/sshwrap
go build -o bin/tray.exe -ldflags="-H windowsgui" ./cmd/tray
```

(`-H windowsgui` keeps the tray from opening a console window.)

## Install / uninstall

```sh
bin\tray.exe --install     # seeds config, sets core.sshCommand, adds run-on-login
bin\tray.exe --uninstall   # reverts core.sshCommand to its prior value, removes run key
```

Install is user-scoped — **no administrator rights needed**. It captures your
previous `core.sshCommand` so uninstall restores it exactly.

## Usage

Run `tray.exe`. The tray menu shows:

- **Radio list of identities** — click one to make it active. The icon changes
  color/initial to match, and your git author + signing config update instantly.
- **Verify connection** — runs `ssh -T git@github.com` with the active key and
  toasts which account GitHub sees you as.
- **Open config** — opens `identities.toml` in your default editor.
- **Run at login** — toggles the HKCU run-on-login entry.
- **Quit**.

## Configuration

`~/.identity-tray/identities.toml` (seeded on first run, hand-editable):

```toml
[[identity]]
label       = "work-account"   # menu label + active-state token
key         = "~/.ssh/id_work"
name        = "work-account"    # git user.name
email       = "you@example.com"    # git user.email
signing_key = "~/.ssh/id_work.pub"
owners      = ["work-account"]  # owners this identity is used for; empty = no mismatch warnings
```

Other files under `~/.identity-tray/`:

- `active` — the label of the current identity (written atomically).
- `events.log` — append-only JSON lines of mismatch events (tailed by the tray).
- `previous_sshcommand` — your prior `core.sshCommand`, saved at install for revert.

## Scope / limitations

- Windows only; manages **`github.com`** (other hosts and aliases pass through).
- Tools that call `ssh` directly (not via git) bypass the wrapper by design.
- The wrapper's overriding invariant is to **never break git**: on any error
  (missing config/key, parse failure) it falls back to a plain ssh passthrough.

## Tests

```sh
go test ./...
```

The wrapper's pure logic (host classification, owner parsing, key injection),
the identity model (config load, atomic state, git-config actions, apply), the
icon/setup helpers, and an end-to-end `cmd/sshwrap` run against a stub ssh
(asserting arg injection + exit-code propagation) are all covered.

CI runs `go build`, `go vet`, and `go test` on `windows-latest` for every push
and pull request.

## License

[MIT](LICENSE).
