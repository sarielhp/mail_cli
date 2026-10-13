// Package bogofiltertest installs a stand-in bogofilter executable for tests.
//
// It exists so a test can exercise code that shells out to bogofilter without
// touching the user's real training database: the installed script replaces the
// real binary on PATH and answers exactly what the test tells it to.
package bogofiltertest

import (
	"os"
	"path/filepath"
	"testing"
)

// Install writes script to a bogofilter executable in a fresh temporary
// directory and makes that directory the test's PATH for its duration. The
// script is run by the shell and receives the message on stdin, so a test
// controls both the verdict printed and the exit status the caller sees.
func Install(t *testing.T, script string) {
	t.Helper()
	bin := t.TempDir()
	path := filepath.Join(bin, "bogofilter")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}
