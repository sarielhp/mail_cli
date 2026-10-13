package main

import (
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cli"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"strings"
	"testing"
)

func TestValidateAccountParams(t *testing.T) {
	tests := []struct {
		name    string
		acc     AccountConfig
		wantErr bool
		errSub  string
	}{
		{"valid gmail", AccountConfig{
			Name: "test", Type: "gmail", Username: "a@b.com",
			Password: "pass", IMAPHost: "imap.gmail.com:993",
			SpamFolder: "[Gmail]/Spam", ReceivedFolder: "received",
		}, false, ""},
		{"valid jmap", AccountConfig{
			Name: "test", Type: "jmap", Username: "a@b.com",
			Password: "pass", SessionURL: "https://api.example.com/jmap",
			SpamFolder: "Spam", ReceivedFolder: "Archive",
		}, false, ""},
		{"valid outlook", AccountConfig{
			Name: "test", Type: "outlook", Username: "a@b.com",
			SpamFolder: "Junk Email", ReceivedFolder: "Archive",
		}, false, ""},
		{"received folder cannot be inbox", AccountConfig{
			Name: "test", Type: "outlook", Username: "a@b.com",
			SpamFolder: "Junk Email", ReceivedFolder: "Inbox",
		}, true, "received_folder cannot be inbox"},
		{"missing name", AccountConfig{Type: "gmail"}, true, "account name is missing"},
		{"missing type", AccountConfig{Name: "test"}, true, "account type is missing"},
		{"missing username", AccountConfig{Name: "test", Type: "gmail"}, true, "account username is missing"},
		{"missing password", AccountConfig{Name: "test", Type: "gmail", Username: "a@b.com"}, true, "account password is missing"},
		{"missing spam folder", AccountConfig{Name: "test", Type: "gmail", Username: "a@b.com", Password: "p", IMAPHost: "imap.gmail.com:993"}, true, "spam_folder is missing"},
		{"missing received folder", AccountConfig{Name: "test", Type: "gmail", Username: "a@b.com", Password: "p", IMAPHost: "imap.gmail.com:993", SpamFolder: "Spam"}, true, "received_folder is missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.name == "received folder cannot be inbox" {
				mc := &mailclient.MockMailClient{
					Cfg: &cfg_g.Config{Policy: cfg_g.Policy{ReceivedFolder: tt.acc.ReceivedFolder}},
				}
				client := mailclient.NewCheckingMailClient(mc, "", nil)
				err = client.Validate()
			} else {
				err = cfg_g.ValidateAccountParams(tt.acc)
			}
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errSub)
				} else if !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errSub)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestLookupFromFileConfig resolves an account the way the file-config callers
// do; it once exercised FindAccountLocally, Lookup's deprecated nil-returning
// wrapper, which nothing else used.
func TestLookupFromFileConfig(t *testing.T) {
	fc := &FileConfig{
		Accounts: &[]AccountConfig{
			{Name: "gmail1", Type: "gmail"},
			{Name: "jmap1", Type: "jmap"},
			{Name: "gmail2", Type: "gmail"},
		},
	}

	tests := []struct {
		name     string
		selected string
		wantName string
		wantErr  bool
	}{
		{"first when empty", "", "gmail1", false},
		{"exact match case-insensitive", "GMAIL1", "gmail1", false},
		{"exact match", "jmap1", "jmap1", false},
		{"no match is an error", "nonexistent", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cfg_g.Lookup(*fc.Accounts, tt.selected)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Lookup(%q) = %q, want an error", tt.selected, got.Name)
				}
				return
			}
			if err != nil {
				t.Errorf("Lookup(%q): %v", tt.selected, err)
				return
			}
			if got.Name != tt.wantName {
				t.Errorf("Lookup(%q) = %q, want %q", tt.selected, got.Name, tt.wantName)
			}
		})
	}
}

func TestSanitizeLabelForCache(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple label", "inbox", "inbox"},
		{"with spaces", "inbox spam", "inbox~20~spam"},
		{"with special chars", "Gmail/Spam", "gmail~2f~spam"},
		{"mixed case", "Important/Work", "important~2f~work"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg_g.SanitizeLabelForCache(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeLabelForCache(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSanitizeLabelForCache_Injective guards against the collision the
// old flatten-to-'_' scheme allowed: distinct account/folder names must
// never sanitize to the same cache path (H4).
func TestSanitizeLabelForCache_Injective(t *testing.T) {
	pairs := [][2]string{
		{"corp.com", "corp_com"},
		{"עבודה", "חופשה"},
		{"a~b", "a_b"},
		{"user+work@outlook.com", "user_work@outlook.com"},
	}
	for _, p := range pairs {
		a, b := cfg_g.SanitizeLabelForCache(p[0]), cfg_g.SanitizeLabelForCache(p[1])
		if a == b {
			t.Errorf("sanitizeLabelForCache(%q) == sanitizeLabelForCache(%q) == %q; want distinct outputs", p[0], p[1], a)
		}
	}
}

func TestComputeShortID(t *testing.T) {
	id1 := "19001b97b0a701fa"
	id2 := "19001b97b0a701fb"

	short1 := email.ComputeShortID(email.ID(id1))
	short2 := email.ComputeShortID(email.ID(id2))

	if len(short1) != 8 {
		t.Errorf("computeShortID(%q) length = %d, want 8", id1, len(short1))
	}

	if short1 == short2 {
		t.Errorf("computeShortID yielded same hash for different inputs: %q", short1)
	}

	for _, r := range short1 {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')) {
			t.Errorf("computeShortID(%q) contained non-uppercase hex char: %c", id1, r)
		}
	}
}

func TestPreprocessArgs(t *testing.T) {
	config := &Config{
		Accounts: []AccountConfig{
			{Name: "account1"},
		},
	}

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no alias",
			input:    []string{"mail_cli", "scan", "inbox"},
			expected: []string{"mail_cli", "scan", "inbox"},
		},
		// ss and sb are real hidden commands now, so the pre-pass must leave
		// them alone rather than rewriting them into a command path.
		{
			name:     "ss passes through to the command",
			input:    []string{"mail_cli", "ss"},
			expected: []string{"mail_cli", "ss"},
		},
		{
			name:     "sb passes through to the command",
			input:    []string{"mail_cli", "sb"},
			expected: []string{"mail_cli", "sb"},
		},
		{
			name:     "__clihelp keeps the wrapped command line",
			input:    []string{"mail_cli", "__clihelp", "wrapper", "mt", "-2", "tui"},
			expected: []string{"mail_cli", "__clihelp", "wrapper", "mt", "-2", "tui"},
		},
		{
			name:     "ss keeps its flags",
			input:    []string{"mail_cli", "ss", "-v"},
			expected: []string{"mail_cli", "ss", "-v"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cliApp := cli.InitCLI(&cli.Session{Config: config})
			got, err := normalizeAccountArgs(cliApp, tt.input[1:])
			if err != nil {
				t.Fatal(err)
			}
			got = append([]string{tt.input[0]}, got...)
			if len(got) != len(tt.expected) {
				t.Fatalf("preprocessArgs(%v) returned slice of length %d, want %d", tt.input, len(got), len(tt.expected))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("preprocessArgs(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}
