package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

func TestCLI_Autocompletion_RootCommands(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf

	// Complete with empty string -> should list root commands
	err := cliApp.Execute([]string{"__complete", ""})
	if err != nil {
		t.Fatalf("__complete error: %v", err)
	}

	output := buf.String()
	for _, expected := range []string{"scan", "spam", "tui", "account", "utils"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected completion output to contain %q, got:\n%s", expected, output)
		}
	}

	// Complete with prefix "tu" -> should return "tui"
	buf.Reset()
	err = cliApp.Execute([]string{"__complete", "tu"})
	if err != nil {
		t.Fatalf("__complete error: %v", err)
	}
	if !strings.Contains(buf.String(), "tui") {
		t.Errorf("expected 'tui' in completion for 'tu', got:\n%s", buf.String())
	}
}

func TestCLI_Autocompletion_AccountsFlag(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	configDir := filepath.Join(tempDir, ".config", app.AppName)
	_ = os.MkdirAll(configDir, 0700)
	configPath := filepath.Join(configDir, "config.json")

	cfg := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{Name: "personal-gmail", Type: "gmail"},
			{Name: "work-fastmail", Type: "jmap"},
		},
	}
	_ = cfg_g.SaveConfigFile(configPath, cfg)

	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf

	// Complete with "-A", "" -> should suggest account names
	err := cliApp.Execute([]string{"__complete", "-A", ""})
	if err != nil {
		t.Fatalf("__complete with -A error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "personal-gmail") || !strings.Contains(output, "work-fastmail") {
		t.Errorf("expected accounts to be completed for -A, got:\n%s", output)
	}

	// Complete with "--account=per"
	buf.Reset()
	err = cliApp.Execute([]string{"__complete", "--account=per"})
	if err != nil {
		t.Fatalf("__complete with --account=per error: %v", err)
	}
	if !strings.Contains(buf.String(), "personal-gmail") {
		t.Errorf("expected personal-gmail in completion for --account=per, got:\n%s", buf.String())
	}
}

func TestCLI_CompletionCommand_Generators(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	for _, shell := range []string{"bash", "zsh", "fish"} {
		var buf bytes.Buffer
		cliApp.Stdout = &buf

		err := cliApp.Execute([]string{"completion", shell})
		if err != nil {
			t.Errorf("completion %s failed: %v", shell, err)
		}
		output := buf.String()
		if len(output) < 50 || !strings.Contains(output, "mail_cli") {
			t.Errorf("expected valid %s completion script for mail_cli, got:\n%s", shell, output)
		}
	}
}

func TestCLI_CompletionCommand_Install(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempDir, ".local", "share"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, ".config"))
	t.Setenv("SHELL", "/bin/bash")

	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf

	err := cliApp.Execute([]string{"completion", "install"})
	if err != nil {
		t.Fatalf("completion install error: %v", err)
	}

	expectedPath, err := clihelp.IntegrationPath(cliApp, "bash")
	if err != nil {
		t.Fatalf("IntegrationPath error: %v", err)
	}
	if _, statErr := os.Stat(expectedPath); os.IsNotExist(statErr) {
		t.Errorf("expected completion file installed at %s", expectedPath)
	}

	bashrcPath := filepath.Join(tempDir, ".bashrc")
	bashrcContent, err := os.ReadFile(bashrcPath)
	if err != nil {
		t.Errorf("expected .bashrc to exist: %v", err)
	} else if !strings.Contains(string(bashrcContent), "mail_cli") {
		t.Errorf("expected .bashrc to contain mail_cli integration: %s", string(bashrcContent))
	}

	buf.Reset()
	err = cliApp.Execute([]string{"completion", "uninstall"})
	if err != nil {
		t.Fatalf("completion uninstall error: %v", err)
	}
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Errorf("expected completion file to be removed at %s", expectedPath)
	}
}

func TestCLI_Autocompletion_FolderNames(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tempDir, ".cache"))

	cacheDir := filepath.Join(tempDir, ".cache", app.AppName)
	_ = os.MkdirAll(cacheDir, 0700)
	labelsPath := filepath.Join(cacheDir, "labels_cache.json")
	_ = os.WriteFile(labelsPath, []byte(`[{"name":"Projects"},{"name":"Receipts"}]`), 0600)

	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf

	// Complete folder with -f
	err := cliApp.Execute([]string{"__complete", "label", "sort", "%inbox", "-f", ""})
	if err != nil {
		t.Fatalf("__complete splice -f error: %v", err)
	}
	output := buf.String()
	for _, expected := range []string{"INBOX", "Archive", "Projects", "Receipts"} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected folder completion output to contain %q, got:\n%s", expected, output)
		}
	}

	// Complete folder with --folder=Pro
	buf.Reset()
	err = cliApp.Execute([]string{"__complete", "label", "sort", "%inbox", "--folder=Pro"})
	if err != nil {
		t.Fatalf("__complete splice --folder=Pro error: %v", err)
	}
	if !strings.Contains(buf.String(), "Projects") {
		t.Errorf("expected Projects in completion for --folder=Pro, got:\n%s", buf.String())
	}
}

func TestCLI_Autocompletion_EnumFlag(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf

	err := cliApp.Execute([]string{"__complete", "account", "new", "--type", ""})
	if err != nil {
		t.Fatalf("__complete account new --type error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "regular") || !strings.Contains(output, "test") {
		t.Errorf("expected enum completion to contain 'regular' and 'test', got:\n%s", output)
	}
}

func TestCLI_CompletionCommand_Keys(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf

	err := cliApp.Execute([]string{"completion", "keys", "bash"})
	if err != nil {
		t.Fatalf("completion keys bash error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "mail_cli") || !strings.Contains(output, "__explain") {
		t.Errorf("expected Alt-H key binding snippet, got:\n%s", output)
	}
}

func TestCLI_ManPageCommand(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	cliApp := InitCLI(session)

	var buf bytes.Buffer
	cliApp.Stdout = &buf

	err := cliApp.Execute([]string{"manpage"})
	if err != nil {
		t.Fatalf("manpage error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, ".TH MAIL_CLI") && !strings.Contains(output, ".TH \"MAIL_CLI\"") {
		t.Errorf("expected roff manpage header in output, got:\n%s", output)
	}
}
