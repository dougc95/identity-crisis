package sshwrap

import (
	"regexp"
	"strings"
)

// greetingRe matches GitHub's SSH auth greeting: "Hi <user>! You've ...".
var greetingRe = regexp.MustCompile(`Hi ([^!]+)!`)

// ParseGreeting extracts the authenticated GitHub account from ssh -T output.
func ParseGreeting(output string) (account string, ok bool) {
	m := greetingRe.FindStringSubmatch(output)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}
