package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sarielhp/mail_cli/app"

	"github.com/sarielhp/clihelp"
)

func TestBootstrapUsesParsedWriteMode(t *testing.T) {
	for _, tt := range []struct {
		name      string
		args      []string
		unchanged bool
		usage     bool
	}{
		{"read only", []string{"--read-only"}, true, false},
		{"dry run", []string{"--dry-run=1"}, true, false},
		{"read only true", []string{"--read-only=true"}, true, false},
		{"last true", []string{"--read-only=false", "--read-only"}, true, false},
		{"last false", []string{"--read-only", "--read-only=false"}, false, false},
		{"dry run false", []string{"--dry-run=false"}, false, false},
		{"flag value", []string{"--pattern", "--read-only"}, false, false},
		{"terminator", []string{"--", "--read-only"}, true, true},
		{"invalid bool", []string{"--read-only=banana"}, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CLIHELP_NO_AUTO_COMPLETION", "1")
			path := filepath.Join(home, ".config", app.AppName, "config.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			original := []byte(`{"accounts":[]}`)
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			cliApp := buildCLI()
			cliApp.Run = func(*clihelp.Context) error { return nil }
			err := cliApp.Execute(tt.args)
			app.CloseLogger()
			if tt.usage != clihelp.IsUsageError(err) || (!tt.usage && err != nil) {
				t.Fatalf("Execute: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(got, original) != tt.unchanged {
				t.Fatalf("config unchanged=%v want=%v", bytes.Equal(got, original), tt.unchanged)
			}
		})
	}
}

func TestMetadataRequestsDoNotBootstrap(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"version"}, {"__complete", ""}, {"docs", "--help"}} {
		t.Run(args[0], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CLIHELP_NO_AUTO_COMPLETION", "1")
			cliApp := buildCLI()
			var out, report bytes.Buffer
			cliApp.Stdout, cliApp.Stderr = &out, &report
			if err := cliApp.Execute(args); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(home, ".config", app.AppName)); !os.IsNotExist(err) {
				t.Fatalf("metadata request initialized configuration or logs: %v", err)
			}
		})
	}
}

// The existing version flag exits at the CLI boundary, so exercise it in a
// child process and inspect the sandbox for unintended bootstrap writes.
func TestVersionFlagDoesNotBootstrap(t *testing.T) {
	if os.Getenv("MAIL_CLI_TEST_VERSION") == "1" {
		os.Args = []string{"mail_cli", "--version"}
		main()
		return
	}
	home := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestVersionFlagDoesNotBootstrap$")
	cmd.Env = append(os.Environ(), "HOME="+home, "MAIL_CLI_TEST_VERSION=1", "CLIHELP_NO_AUTO_COMPLETION=1")
	if out, err := cmd.CombinedOutput(); err != nil || !bytes.Contains(out, []byte("mail_cli version")) {
		t.Fatalf("version request: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", app.AppName)); !os.IsNotExist(err) {
		t.Fatalf("version request initialized configuration or logs: %v", err)
	}
}

func TestLoggerUsesParsedAppendMode(t *testing.T) {
	for _, tt := range []struct {
		name      string
		args      []string
		preserved bool
	}{
		{"append", []string{"--logappend"}, true},
		{"append true", []string{"--logappend=true"}, true},
		{"last false", []string{"--logappend", "--logappend=false"}, false},
		{"flag value", []string{"--pattern", "--logappend"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CLIHELP_NO_AUTO_COMPLETION", "1")
			path := filepath.Join(home, ".config", app.AppName, "logs", "previous.log")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			cliApp := buildCLI()
			cliApp.Run = func(*clihelp.Context) error { return nil }
			err := cliApp.Execute(tt.args)
			app.CloseLogger()
			if err != nil {
				t.Fatal(err)
			}
			_, err = os.Stat(path)
			if (err == nil) != tt.preserved {
				t.Fatalf("previous log preserved=%v want=%v", err == nil, tt.preserved)
			}
		})
	}
}
