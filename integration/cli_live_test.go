package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// cliBinaryPath resolves an executable mail_cli binary.
// It checks ~/bin/mail_cli first, then builds a temporary test binary if needed.
func cliBinaryPath(t *testing.T) string {
	t.Helper()

	home, err := os.UserHomeDir()
	if err == nil {
		userBin := filepath.Join(home, "bin", "mail_cli")
		if _, err := os.Stat(userBin); err == nil {
			return userBin
		}
	}

	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "mail_cli")
	cmd := exec.Command("go", "build", "-o", binPath, "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building mail_cli test binary: %v\n%s", err, string(out))
	}
	return binPath
}

func cliTestAccount(t *testing.T) string {
	t.Helper()

	if os.Getenv(mutateEnvVar) == "" {
		t.Skipf("CLI live tests are opt-in; set %s=1 to run them", mutateEnvVar)
	}
	name := os.Getenv(mutateAcctVar)
	if name == "" {
		t.Skipf("%s must name the account to mutate; there is deliberately no default", mutateAcctVar)
	}

	config, err := cfg_g.LoadConfig()
	if err != nil || config == nil {
		t.Skipf("no usable configuration: %v", err)
	}

	acc, found := findAccount(config, name)
	if !found {
		t.Fatalf("account %q is not configured", name)
	}
	if !strings.EqualFold(acc.AccountType, testAccountType) {
		t.Fatalf("refusing to run CLI live tests on %q: account_type is %q, not %q", name, acc.AccountType, testAccountType)
	}
	return acc.Name
}

func runCLICmd(t *testing.T, bin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestCLIScanInbox(t *testing.T) {
	acct := cliTestAccount(t)
	bin := cliBinaryPath(t)

	stdout, stderr, err := runCLICmd(t, bin, "-A", acct, "%Inbox")
	if err != nil {
		t.Fatalf("mail_cli -A %s %%Inbox failed: %v\nStderr: %s\nStdout: %s", acct, err, stderr, stdout)
	}

	if !strings.Contains(stdout, "Inbox") && !strings.Contains(stderr, "Inbox") {
		t.Errorf("expected output to mention Inbox, got:\nStdout: %s\nStderr: %s", stdout, stderr)
	}
}

func TestCLISearch(t *testing.T) {
	acct := cliTestAccount(t)
	bin := cliBinaryPath(t)

	stdout, stderr, err := runCLICmd(t, bin, "-A", acct, "search", "Fixture", "-n", "3")
	if err != nil {
		t.Fatalf("mail_cli search failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	if !strings.Contains(stdout, "SEARCH:") && !strings.Contains(stdout, "match") {
		t.Errorf("expected search output header, got:\n%s", stdout)
	}
}

func TestCLILabelLifecycle(t *testing.T) {
	acct := cliTestAccount(t)
	bin := cliBinaryPath(t)

	lbl := fmt.Sprintf("mail_cli_cli_test_%d", time.Now().UnixNano()%10000000)

	// Create label
	stdout, stderr, err := runCLICmd(t, bin, "-A", acct, "label", "create", lbl)
	if err != nil {
		t.Fatalf("label create failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	t.Cleanup(func() {
		_, _, _ = runCLICmd(t, bin, "-A", acct, "label", "delete", "%"+lbl, "--yes")
	})

	// Verify label list contains new label
	stdout, stderr, err = runCLICmd(t, bin, "-A", acct, "label", "list")
	if err != nil {
		t.Fatalf("label list failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}
	if !strings.Contains(stdout, lbl) {
		t.Errorf("expected label %q in list, got:\n%s", lbl, stdout)
	}

	// Delete label
	stdout, stderr, err = runCLICmd(t, bin, "-A", acct, "label", "delete", "%"+lbl, "--yes")
	if err != nil {
		t.Fatalf("label delete failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	// Verify label list no longer contains deleted label
	stdout, stderr, err = runCLICmd(t, bin, "-A", acct, "label", "list")
	if err != nil {
		t.Fatalf("label list post-delete failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}
	if strings.Contains(stdout, lbl) {
		t.Errorf("expected label %q to be gone after delete, got:\n%s", lbl, stdout)
	}
}
