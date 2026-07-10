// Command sshwrap is the SSH wrapper git invokes via core.sshCommand. It reads
// the active identity, injects that identity's key for github.com, warns (never
// blocks) on repo-owner mismatch, and execs the real ssh binary — transparently
// proxying stdio and the exit code. Its overriding invariant is to never break
// git: on any error it falls back to a plain passthrough.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dougc95/identity-crisis/internal/identity"
	"github.com/dougc95/identity-crisis/internal/sshwrap"
)

// defaultSSH is the real OpenSSH client shipped with Windows.
const defaultSSH = `C:\Windows\System32\OpenSSH\ssh.exe`

// env holds the process dependencies so run() is testable.
type env struct {
	realSSH string
	home    string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func main() {
	home, _ := os.UserHomeDir()
	realSSH := os.Getenv("IDENTITY_TRAY_SSH")
	if realSSH == "" {
		realSSH = defaultSSH
	}
	os.Exit(run(os.Args[1:], env{
		realSSH: realSSH,
		home:    home,
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}))
}

// run applies identity decisions to args and execs the real ssh, returning its
// exit code.
func run(args []string, e env) int {
	final, mismatch, keyMissing := decide(args, e.home)

	if keyMissing != "" {
		fmt.Fprintf(e.stderr, "identity-tray: active key not found (%s); using ssh defaults\n", keyMissing)
	}
	if mismatch != "" {
		active, _ := identity.ReadActive(identity.ActivePath(e.home))
		fmt.Fprintf(e.stderr, "identity-tray: WARNING active identity %q does not own %q — pushing/pulling with the wrong account\n", active, mismatch)
		appendEvent(e.home, active, mismatch)
	}

	cmd := exec.Command(e.realSSH, final...)
	cmd.Stdin = e.stdin
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(e.stderr, "identity-tray: failed to run ssh (%s): %v\n", e.realSSH, err)
		return 255
	}
	return 0
}

// decide inspects args and the active identity to produce the final ssh args.
// It returns the (possibly key-injected) args, the mismatched owner (empty if
// none), and the path of a missing key (empty unless the active key is absent).
// Any error resolving state yields a plain passthrough (fail-safe).
func decide(args []string, home string) (final []string, mismatchOwner, keyMissing string) {
	host, idx, ok := sshwrap.FindHost(args)
	if !ok || !sshwrap.IsManaged(sshwrap.Hostname(host)) {
		return args, "", ""
	}

	label, err := identity.ReadActive(identity.ActivePath(home))
	if err != nil {
		return args, "", ""
	}
	cfg, err := identity.LoadConfig(identity.IdentitiesPath(home))
	if err != nil {
		return args, "", ""
	}
	id, ok := cfg.Find(label)
	if !ok {
		return args, "", ""
	}

	// Mismatch check against the repo owner in the remote command.
	if idx+1 < len(args) {
		command := strings.Join(args[idx+1:], " ")
		if owner, ok := sshwrap.ParseOwner(command); ok && !sshwrap.OwnerAllowed(owner, id.Owners) {
			mismatchOwner = owner
		}
	}

	keyPath := identity.ExpandHome(id.Key, home)
	if _, err := os.Stat(keyPath); err != nil {
		return args, mismatchOwner, keyPath
	}
	return sshwrap.InjectKey(args, keyPath), mismatchOwner, ""
}

// appendEvent records a mismatch as a JSON line the tray can tail for toasts.
// Best-effort: failures are ignored so a logging problem never breaks git.
func appendEvent(home, active, owner string) {
	f, err := os.OpenFile(identity.EventsPath(home), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, `{"time":%q,"type":"mismatch","active":%q,"owner":%q}`+"\n", ts, active, owner)
}
