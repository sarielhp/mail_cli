package cli

import (
	"fmt"
	"os"
	"testing"
)

// The drafts commands fall back to $HOME/.cache/mail_cli/drafts when a
// session has no download directory. No test may reach the developer's real
// cache that way, so the whole package runs with HOME in a scratch directory.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mail_cli_cli_home_")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot create a scratch HOME:", err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
