// Package sshwrap contains the pure logic for the SSH wrapper that git invokes
// via core.sshCommand. It classifies the target host, parses the repo owner out
// of the git remote command, and builds the argument list for the real ssh
// binary. All functions here are side-effect free and unit-tested; the process
// wiring (reading state, exec) lives in cmd/sshwrap.
package sshwrap

import "strings"

// managedHost is the single host whose key selection the tray controls.
// Everything else (host aliases, other hosts) passes through untouched.
const managedHost = "github.com"

// flagsWithArg lists the single-character ssh options that consume a following
// argument when given without an attached value (e.g. "-o value", "-i key").
const flagsWithArg = "BbcDEeFIiJLlmOopQRSWw"

// IsManaged reports whether the tray controls key selection for host.
// Comparison is case-insensitive. host should already have any "user@" prefix
// stripped (see Hostname).
func IsManaged(host string) bool {
	return strings.EqualFold(host, managedHost)
}

// Hostname strips an optional "user@" prefix, returning just the hostname.
func Hostname(host string) string {
	if i := strings.LastIndex(host, "@"); i >= 0 {
		return host[i+1:]
	}
	return host
}

// FindHost locates the destination host in an ssh argument list, skipping
// leading options and their values. It returns the host token (which may still
// carry a "user@" prefix), its index, and whether a host was found.
func FindHost(args []string) (host string, index int, ok bool) {
	i := 0
	for i < len(args) {
		a := args[i]
		if len(a) > 1 && a[0] == '-' {
			opt := a[1]
			if strings.IndexByte(flagsWithArg, opt) >= 0 && len(a) == 2 {
				// Option takes its value from the next token.
				i += 2
				continue
			}
			// Valueless flag, combined flags, or attached value.
			i++
			continue
		}
		return a, i, true
	}
	return "", 0, false
}

// ParseOwner extracts the repository owner from a git remote command such as
// "git-upload-pack 'owner/repo.git'". It returns the owner and whether parsing
// succeeded. Only the recognized git pack/archive commands are accepted.
func ParseOwner(command string) (owner string, ok bool) {
	command = strings.TrimSpace(command)
	var rest string
	switch {
	case strings.HasPrefix(command, "git-upload-pack "):
		rest = command[len("git-upload-pack "):]
	case strings.HasPrefix(command, "git-receive-pack "):
		rest = command[len("git-receive-pack "):]
	case strings.HasPrefix(command, "git-upload-archive "):
		rest = command[len("git-upload-archive "):]
	default:
		return "", false
	}
	rest = strings.TrimSpace(rest)
	rest = strings.Trim(rest, `'"`)
	rest = strings.TrimPrefix(rest, "/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) < 2 || parts[0] == "" {
		return "", false
	}
	return parts[0], true
}

// InjectKey returns a new argument list that pins ssh to the given key,
// prepending "-i <keyPath> -o IdentitiesOnly=yes" ahead of the original args.
// The input slice is not modified.
func InjectKey(orig []string, keyPath string) []string {
	out := make([]string, 0, len(orig)+4)
	out = append(out, "-i", keyPath, "-o", "IdentitiesOnly=yes")
	out = append(out, orig...)
	return out
}

// OwnerAllowed reports whether owner is one this identity is expected to use.
// An empty owners list disables the check (always allowed), so mismatch warnings
// are opt-in per identity. Comparison is case-insensitive.
func OwnerAllowed(owner string, owners []string) bool {
	if len(owners) == 0 {
		return true
	}
	for _, o := range owners {
		if strings.EqualFold(o, owner) {
			return true
		}
	}
	return false
}
