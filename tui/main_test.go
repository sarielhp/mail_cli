package tui

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

// The tests drive real key handlers, and some of those hand files to the
// desktop. No test may start a program on the developer's machine, so the
// whole package runs with the opener replaced by a recorder.
var (
	openedFilesMu sync.Mutex
	openedFiles   []string
)

func recordOpenedFile(path string) {
	openedFilesMu.Lock()
	defer openedFilesMu.Unlock()
	openedFiles = append(openedFiles, path)
}

// HOME is redirected as well: anything that still derives a path from it,
// such as the drafts fallback or the signature file, lands in a scratch
// directory rather than the developer's.
func TestMain(m *testing.M) {
	openSystemFile = recordOpenedFile
	home, err := os.MkdirTemp("", "mail_cli_tui_home_")
	if err != nil {
		// Running on without the redirect would reach the real HOME.
		fmt.Fprintln(os.Stderr, "cannot create a scratch HOME:", err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	if home != "" {
		os.RemoveAll(home)
	}
	os.Exit(code)
}
