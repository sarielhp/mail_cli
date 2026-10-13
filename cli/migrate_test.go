package cli

import (
	"strings"
	"testing"
)

func TestResolveSshTarget_ValidTargets(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		expected string
	}{
		{
			name:     "user and hostname",
			target:   "alice@server.example.com",
			expected: "alice@server.example.com",
		},
		{
			name:     "ip address destination",
			target:   "backup@192.168.1.50",
			expected: "backup@192.168.1.50",
		},
		{
			name:     "custom port-style host",
			target:   "deploy@host-01.internal",
			expected: "deploy@host-01.internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSSHTarget(tt.target)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.target, err)
			}
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestResolveSSHTarget_RejectsHyphenAndCommandInjection(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{
			name:   "leading hyphen flag option",
			target: "-oProxyCommand=touch /tmp/pwned",
		},
		{
			name:   "user with leading hyphen",
			target: "-oProxyCommand=id@attacker.com",
		},
		{
			name:   "host with leading hyphen",
			target: "alice@-oProxyCommand=calc.exe",
		},
		{
			name:   "double dash flag",
			target: "--target=evil.com",
		},
		{
			name:   "semicolon injection",
			target: "alice@server;rm -rf /",
		},
		{
			name:   "backtick command substitution",
			target: "alice@`id`",
		},
		{
			name:   "space separated injection",
			target: "alice@server extra args",
		},
		{
			name:   "pipe injection",
			target: "alice@server|cat",
		},
		{
			name:   "multiple at signs",
			target: "user@host1@host2",
		},
		{
			name:   "empty target",
			target: "   ",
		},
		{
			name:   "empty host with user",
			target: "user@",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSSHTarget(tt.target)
			if err == nil {
				t.Errorf("expected error for malicious target %q, got nil (result: %q)", tt.target, got)
			}
			if !strings.Contains(err.Error(), "migrate:") {
				t.Errorf("expected error to have migrate prefix, got: %v", err)
			}
		})
	}
}

func TestParseTarget(t *testing.T) {
	u, h := parseTarget("user@host.com")
	if u != "user" || h != "host.com" {
		t.Errorf("unexpected result: user=%q, host=%q", u, h)
	}

	u2, h2 := parseTarget("hostonly.local")
	if u2 != "" || h2 != "hostonly.local" {
		t.Errorf("unexpected result: user=%q, host=%q", u2, h2)
	}
}
