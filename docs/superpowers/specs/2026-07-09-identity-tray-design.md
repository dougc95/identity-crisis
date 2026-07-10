# identity-tray — Design Spec

**Date:** 2026-07-09
**Status:** Approved (brainstorming), pending implementation plan
**Location:** `C:\Users\you\Code\identity-tray` (standalone; unrelated to the Hellium/Dual-Website app)

## Problem

The user has multiple GitHub accounts, each tied to a different SSH key:

| SSH key | GitHub account |
|---|---|
| `~/.ssh/id_work` | `work-account` |
| `~/.ssh/id_personal` | `personal-account` |
| `~/.ssh/id_side` | `side-project` |

SSH silently uses whichever key `~/.ssh/config` dictates, with no picker. This causes the wrong account to be used for git operations (wrong-key auth failures, and — worse — commits authored/signed under the wrong identity). Host aliases solve per-repo pinning but require remembering to use them.

## Goal

A Windows system-tray app that lets the user set an **active identity** with one click. The active identity determines the SSH key, commit author, and commit signing key used by subsequent git operations — so "the next operation the agent runs" picks up the selected key automatically.

## Decisions (from brainstorming)

- **Interaction model:** active-identity toggle. Pick one; it persists until changed. No per-operation prompts.
- **Identity scope:** switching an identity changes **all three** of — SSH auth key, commit author (`user.name`/`user.email`), and SSH commit signing key (`user.signingkey` + `gpg.format=ssh`).
- **v1 features:** identity indicator, verify-connection action, mismatch warning, run-on-login.
- **Mismatch behavior:** **warn only, never block** git.
- **Tech stack:** Go — two single static `.exe` binaries, no runtime, fast wrapper cold start.
- **Emails:** the same email (`you@example.com`) for all three identities. Each identity's `name` defaults to its GitHub login (distinguishable, editable).
- **Icons:** auto-generated colored initial icons (no hand-supplied `.ico` files).
- **Host aliases** (`github-work`, etc.): pass straight through untouched — explicit per-repo pins always win over the tray.

## Architecture

The design leans on one chokepoint: **git invokes SSH for every network operation**, passing the host and the remote command (which contains the repo owner/name). By setting git's `core.sshCommand` to a wrapper we control, that wrapper is the single point where both key-injection and mismatch-detection happen.

```
User clicks identity in tray
        │
        ▼
  tray.exe ──writes──▶ ~/.identity-tray/active        (label of active identity)
        │
        └──runs──▶ git config --global user.name/email/signingkey, gpg.format, commit.gpgsign

git push / pull / clone
        │
        ▼
git calls core.sshCommand ─▶ sshwrap.exe github.com "git-upload-pack 'owner/repo.git'"
        │
        ├─ classify host (github.com | alias | other)
        ├─ read active identity + key
        ├─ parse owner from args; if owner ∉ active identity's owners → warn (stderr + events.log)
        └─ exec real ssh.exe with -i <key> -o IdentitiesOnly=yes + original args (proxy stdio + exit code)
                                                              │
                                             tray tails events.log ─▶ Windows toast on mismatch
```

### Components

| Component | Path | Responsibility |
|---|---|---|
| Tray UI | `cmd/tray` | Systray menu, apply identity, indicator, verify, toasts, run-on-login |
| SSH wrapper | `cmd/sshwrap` | Chokepoint: classify host, inject key, detect mismatch, exec real ssh |
| Identity core | `internal/identity` | Load config, read/write active state, produce git-config actions |
| Wrapper core | `internal/sshwrap` | Pure functions: host classification, owner parsing, arg construction |
| Setup | `internal/setup` | Configure `core.sshCommand`, seed config, register run-on-login |

## Config & state

Directory: `~/.identity-tray/`

- `identities.toml` — hand-editable (Settings → opens this file). Seeded at install with the three known accounts.
- `active` — one line: the label of the active identity. Written atomically (temp file + rename) so a concurrent git op never reads a torn file.
- `events.log` — append-only JSON lines of mismatch events; tailed by the tray to raise toasts.

`identities.toml` shape:

```toml
[[identity]]
label       = "work-account"
key         = "~/.ssh/id_work"
name        = "work-account"          # -> git user.name (editable)
email       = "you@example.com"          # shared across all identities
signing_key = "~/.ssh/id_work.pub"  # -> git user.signingkey
owners      = ["work-account"]         # repo owners this identity is expected for

[[identity]]
label       = "personal-account"
key         = "~/.ssh/id_personal"
name        = "personal-account"
email       = "you@example.com"
signing_key = "~/.ssh/id_personal.pub"
owners      = ["personal-account"]

[[identity]]
label       = "side-project"
key         = "~/.ssh/id_side"
name        = "side-project"
email       = "you@example.com"
signing_key = "~/.ssh/id_side.pub"
owners      = ["side-project"]
```

If an identity's `owners` list is empty, mismatch detection is disabled for it (opt-in, no false alarms).

## Tray app (`tray.exe`)

- **Menu:** radio list of identities (active is checked) · `Verify connection` · `Open config` · `Run at login` (toggle) · `Quit`.
- **Apply identity** (on select): write `active` atomically, then run the equivalent of:
  ```
  git config --global user.name        <identity.name>
  git config --global user.email       <identity.email>
  git config --global user.signingkey  <identity.signing_key>
  git config --global gpg.format ssh
  git config --global commit.gpgsign true
  git config --global gpg.ssh.allowedSignersFile ~/.ssh/allowed_signers
  ```
  then refresh icon + tooltip.
- **Indicator:** auto-generated icon (per-identity color + initial); tooltip shows `label → GitHub account`.
- **Verify connection:** run `ssh -T git@github.com` with the active key (`-i <key> -o IdentitiesOnly=yes`), parse `Hi <user>!`, toast the result (success/mismatch/failure).
- **Mismatch toasts:** watch `events.log` via fsnotify; toast on each new mismatch event.

## SSH wrapper (`sshwrap.exe`)

Registered via `git config --global core.sshCommand "<path>/sshwrap.exe"`. Git appends `<host> <remote-command>`.

Logic:

1. **Classify host** (first non-flag arg):
   - Exactly `github.com` → follow active identity (inject key + mismatch check).
   - Anything else — host aliases (`github-work`, `github-side`, …) and all other hosts → **pass through untouched**. No ssh-config parsing required; the single managed host is `github.com`.
2. **Inject key** (github.com only): read `active` → resolve `key` → add `-i <key> -o IdentitiesOnly=yes`.
3. **Mismatch check** (github.com only): parse owner from the remote command (`git-upload-pack`/`git-receive-pack`, handling quoted `'owner/repo.git'` paths). If owner ∉ active identity's `owners`, write a human-readable warning to **stderr** and append a JSON event to `events.log`. Do **not** block.
4. **Exec real ssh:** invoke `C:/Windows/System32/OpenSSH/ssh.exe` with injected args + all original args; transparently proxy stdin/stdout/stderr and propagate the **exit code**.

### Fail-safe invariant

The wrapper sits in the critical path of every git command. **It must never break git.** If config/state is missing, the key file is absent, or parsing fails, it falls back to a plain passthrough to real ssh (optionally logging a warning) and exits with real ssh's status.

### Interaction with existing aliases

The previously-added `~/.ssh/config` host aliases (`github-work`, `github-side`) remain valid. Repos cloned via an alias keep their pinned key regardless of the active identity, because the wrapper passes alias hosts through untouched. Plain `git@github.com:` remotes follow the tray.

## Setup & run-on-login

A `setup` path (e.g. `tray.exe --install`) that:
- Places the binaries and sets `git config --global core.sshCommand` to `sshwrap.exe`.
- Creates `~/.identity-tray/` and seeds `identities.toml` (three known accounts) if absent.
- Registers run-on-login via `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` (**no admin required**); on launch, `tray.exe` restores the last active identity.
- An `uninstall` path reverts `core.sshCommand` (to the previous `ssh.exe`) and removes the Run key.

## Project structure

```
identity-tray/
  cmd/tray/main.go            # systray UI (thin)
  cmd/sshwrap/main.go         # chokepoint entry (thin)
  internal/identity/          # config load, active state, apply-to-git actions
  internal/sshwrap/           # host classify, owner parse, arg build (pure, tested)
  internal/setup/             # git config, registry run-key, seeding, uninstall
  internal/icon/              # auto-generate colored initial icons
  go.mod
  README.md
  docs/superpowers/specs/2026-07-09-identity-tray-design.md
```

**Dependencies:** `fyne.io/systray` (tray), `go-toast/toast` (Windows toasts), `fsnotify/fsnotify` (event log watching), `BurntSushi/toml` (config). Everything else stdlib.

## Error handling

- **Wrapper:** never breaks git — always falls back to passthrough (see invariant). Warnings go to stderr + `events.log`.
- **Tray apply:** if a `git config` write fails, toast the error and leave `active` unchanged (or roll back).
- **Atomic state writes:** `active` written via temp + rename to avoid torn reads under concurrent git ops.
- **Missing key file:** wrapper logs and passes through; tray Verify surfaces it clearly.

## Testing strategy (TDD)

- **`internal/sshwrap` (pure, primary coverage):**
  - Owner parsing across `git-upload-pack` and `git-receive-pack`, quoted/unquoted paths, nested org paths, and malformed input.
  - Host classification: `github.com` vs `github-*` alias vs arbitrary host.
  - Arg construction: correct `-i`/`IdentitiesOnly` injection; original args preserved and ordered.
- **Wrapper integration:** run `sshwrap` against a **stub "real ssh"** (path overridable via env var) to assert injected args, stdio passthrough, and exit-code propagation; assert passthrough (no `-i`) for alias/other hosts.
- **`internal/identity`:** config load (valid/missing/partial), active read/write atomicity, apply-to-git produces the expected git-config command set.
- **`internal/setup`:** seeding is idempotent; run-key add/remove; `core.sshCommand` set/revert.
- **Manual/e2e:** real `ssh -T`, a real push to a throwaway repo, mismatch toast, run-on-login across a reboot.

## Out of scope (v1 / YAGNI)

- Auto-selecting the correct key from the repo owner (contradicts the explicit toggle + warn-only choices). Possible future "auto mode."
- Non-git ssh usage (tools that call ssh directly bypass the wrapper by design).
- Non-GitHub hosts.
- Cross-platform (Windows only).
