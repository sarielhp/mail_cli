package cli

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestPromptSecretWithInput_NonTerminal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "secret entered", input: "s3cr3t-t0k3n\n", want: "s3cr3t-t0k3n"},
		{name: "empty input uses default", input: "\n", want: "default-pass"},
		{name: "whitespace only uses default", input: "   \t  \n", want: "default-pass"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tc.input))
			got, err := promptSecretWithInput(r, -1, false, "Password", "default-pass")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Pressing Enter chooses the default. Closed stdin is not pressing Enter.
//
// Returning the default for both meant `account new jmap work < /dev/null`
// stored the literal placeholder "your-jmap-api-token" as the account password
// and a Fastmail session URL the user never saw -- written to the config as
// though they had typed them.
func TestPromptSecretWithInput_ClosedStdinIsNotAnAnswer(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("")) // EOF immediately

	got, err := promptSecretWithInput(r, -1, false, "Password (JMAP API Token)", "your-jmap-api-token")
	if err == nil {
		t.Fatalf("closed stdin returned %q with no error; the default must not be assumed", got)
	}
	if got != "" {
		t.Errorf("got %q alongside the error, want the empty string", got)
	}
	if !errors.Is(err, ErrUsage()) {
		t.Errorf("want a usage error so main exits 2, got %v", err)
	}
	if !strings.Contains(err.Error(), "Password (JMAP API Token)") {
		t.Errorf("the error should name the prompt that went unanswered, got %v", err)
	}
}

func TestAccountNew_NilAccountsInConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"username": "test@example.com"}`), 0600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	go func() {
		defer w.Close()
		_, _ = w.WriteString("testuser@gmail.com\n\n\n")
	}()

	cfg := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tempDir}}
	session := &Session{Config: cfg}
	if execErr := InitCLI(session).Execute([]string{"account", "new", "gmail", "personal"}); execErr != nil {
		t.Fatalf("account new execution failed: %v", execErr)
	}

	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile failed: %v", err)
	}
	if fc.Accounts == nil || len(*fc.Accounts) != 1 {
		t.Fatalf("expected 1 account saved, got %v", fc.Accounts)
	}
	if (*fc.Accounts)[0].Name != "personal" {
		t.Errorf("expected account name personal, got %s", (*fc.Accounts)[0].Name)
	}
}

// withPipedStdin runs fn with os.Stdin replaced by a pipe carrying input, and
// returns the fn error. It exists so the interactive-path tests below read as
// the answers they feed rather than as pipe plumbing.
func withPipedStdin(t *testing.T, input string, fn func() error) error {
	t.Helper()

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	go func() {
		defer w.Close()
		_, _ = w.WriteString(input)
	}()

	return fn()
}

// --interactive runs the guided flow, not the terse one.
//
// The two paths ask for the same fields, so a test that only checked the saved
// account would pass for either. These lock the differences that justify the
// flag: the provider folder defaults and the connection check come from the
// guided helpers, and --type still decides the account mode.
func TestAccountAddInteractiveUsesGuidedFlow(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		input    string
		wantMode string
		wantURL  string
	}{
		{
			name:     "jmap regular",
			args:     []string{"account", "new", "jmap", "guided", "--interactive"},
			input:    "me@fastmail.com\ns3cr3t\n\nn\n",
			wantMode: "regular",
			wantURL:  "https://api.fastmail.com/jmap/session",
		},
		{
			name:     "jmap test account",
			args:     []string{"account", "new", "jmap", "guided", "--interactive", "--type", "test"},
			input:    "me@fastmail.com\ns3cr3t\n\nn\n",
			wantMode: "test",
			wantURL:  "https://api.fastmail.com/jmap/session",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.json")
			if err := os.WriteFile(configPath, []byte(`{"username": "test@example.com"}`), 0600); err != nil {
				t.Fatalf("write config failed: %v", err)
			}

			session := &Session{Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tempDir}}}
			// The trailing "n" answers "Verify connection to server now?" so
			// the test never reaches the network.
			if err := withPipedStdin(t, tc.input, func() error {
				return InitCLI(session).Execute(tc.args)
			}); err != nil {
				t.Fatalf("account new --interactive: %v", err)
			}

			fc, err := cfg_g.LoadConfigFile(configPath)
			if err != nil {
				t.Fatalf("LoadConfigFile: %v", err)
			}
			if fc.Accounts == nil || len(*fc.Accounts) != 1 {
				t.Fatalf("expected one account, got %v", fc.Accounts)
			}

			acc := (*fc.Accounts)[0]
			for _, field := range []struct{ name, got, want string }{
				{"Name", acc.Name, "guided"},
				{"Type", acc.Type, "jmap"},
				{"Username", acc.Username, "me@fastmail.com"},
				{"Password", acc.Password, "s3cr3t"},
				{"SessionURL", acc.SessionURL, tc.wantURL},
				{"AccountType", acc.AccountType, tc.wantMode},
				// Set by the guided helper; the terse path leaves it empty.
				{"SpamFolder", acc.SpamFolder, "Spam"},
			} {
				if field.got != field.want {
					t.Errorf("%s = %q, want %q", field.name, field.got, field.want)
				}
			}
		})
	}
}

func TestAccountNewCmd_TitleAndParameters(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cmd := accountNewCmd(session)
	if cmd.Title != "account add <type> [name]" {
		t.Errorf("expected Title %q, got %q", "account add <type> [name]", cmd.Title)
	}
	if len(cmd.Parameters) < 2 || cmd.Parameters[0].Name != "<jmap|gmail|outlook>" || cmd.Parameters[1].Name != "[name]" {
		t.Errorf("unexpected parameters: %+v", cmd.Parameters)
	}
}

// Every answer must survive, not just the first.
//
// Each prompt used to build its own bufio.Reader over os.Stdin. The first one
// buffered all three lines below, answered with line one and dropped the rest,
// so the next two prompts read EOF and took their defaults -- and the account
// was saved with an IMAP host the user had typed something else for. The
// earlier test passed through this bug rather than in spite of it: it supplied
// three lines, two were discarded, and the defaults happened to be acceptable.
func TestAccountNewKeepsEveryPipedAnswer(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"username": "test@example.com"}`), 0600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	go func() {
		defer w.Close()
		_, _ = w.WriteString("someone@gmail.com\nhunter2\nimap.example.org:993\n")
	}()

	session := &Session{Config: &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tempDir}}}
	if execErr := InitCLI(session).Execute([]string{"account", "new", "gmail", "piped"}); execErr != nil {
		t.Fatalf("account new: %v", execErr)
	}

	fc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if fc.Accounts == nil || len(*fc.Accounts) != 1 {
		t.Fatalf("expected one account, got %v", fc.Accounts)
	}

	acc := (*fc.Accounts)[0]
	for _, tc := range []struct{ field, got, want string }{
		{"Username", acc.Username, "someone@gmail.com"},
		{"Password", acc.Password, "hunter2"},
		{"IMAPHost", acc.IMAPHost, "imap.example.org:993"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q (answer lost to a per-prompt reader?)", tc.field, tc.got, tc.want)
		}
	}
}
