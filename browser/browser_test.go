package browser

import (
	"strings"
	"testing"
)

func TestWindowsCmd(t *testing.T) {
	const testURL = "https://example.com/path?a=b"

	cmd := windowsCmd(testURL)
	if cmd.Args[0] != "rundll32" {
		t.Errorf("expected rundll32, got %q", cmd.Args[0])
	}
	if got := strings.Join(cmd.Args, " "); !strings.Contains(got, testURL) {
		t.Errorf("URL missing from command %q", got)
	}
}

func TestOpenWithRejectsEmptyCommand(t *testing.T) {
	if err := OpenWith("", "https://example.com"); err == nil {
		t.Error("expected an error when no browser command is given")
	}
}
