package interactive

import (
	"path/filepath"
	"strings"
	"testing"
)

// The targets of the three links in testdata/interactive/links.eml. Each is
// longer than the reader's 60-column link, so a click that opens the full URL
// proves the target survived shortening; the second is wrapped by urldefense
// in the message and must open unwrapped.
var (
	magicLink = "https://login.example.com/magic-link#" + strings.Repeat("0a3ce7a9", 8) + ":c2lnbi1pbi10ZXN0QGV4YW1wbGUuY29t"
	trackLink = "https://click.example.net/ls/click?upn=" + strings.Repeat("u001.rFcAmKXLOm9u6wLRWHIUY-2B", 8)
	iconLink  = "https://social.example.org/follow?campaign=" + strings.Repeat("signin-footer-", 6) + "icon"
)

// TestEmlLinksOpenOnClick opens a .eml from the command line, reads the HTML
// message, and clicks each entry of its References list. w3m once dropped
// every href, and then reported them as file:///<cwd>/https://...; the
// button's link, a tracking link and an image-only link must each reach the
// browser complete.
func TestEmlLinksOpenOnClick(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "testdata", "interactive", "links.eml"))
	if err != nil {
		t.Fatal(err)
	}
	s := start(t, "tui", fixture)
	s.WaitFor("Your sign-in link is here")
	s.Key("enter")
	screen := s.WaitFor("References:")

	for _, want := range []string{"[1]Sign in", "[2]Example Support", "[3]"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if !strings.Contains(screen, "...") {
		t.Errorf("long links were not shortened, so the terminal got no hyperlinks:\n%s", screen)
	}

	for i, want := range []string{magicLink, trackLink, iconLink} {
		marker := "[" + string(rune('1'+i)) + "] https://"
		col, row := s.Find(marker)
		s.Click(col+len(marker)+2, row)
		if got := s.WaitOpened(i + 1)[i]; got != want {
			t.Errorf("click on %s opened\n  %s\nwant\n  %s", marker, got, want)
		}
	}
}
