package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestExamplesFlagRendersTheView(t *testing.T) {
	var out bytes.Buffer
	app := InitCLI(&Session{Config: &cfg_g.Config{}})
	app.Stdout = &out

	if err := app.Execute([]string{"-E"}); err != nil {
		t.Fatalf("expected -E to succeed, got: %v", err)
	}
	if !strings.Contains(out.String(), "Examples:") {
		t.Errorf("expected the examples view, got %q", out.String())
	}
}

// `-E spam` narrows to one command; the word must survive the flag filtering.
func TestExamplesFlagNarrowsToACommand(t *testing.T) {
	var out bytes.Buffer
	app := InitCLI(&Session{Config: &cfg_g.Config{}})
	app.Stdout = &out

	if err := app.Execute([]string{"-E", "spam"}); err != nil {
		t.Fatalf("expected -E spam to succeed, got: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "spam") {
		t.Errorf("expected spam's examples, got %q", rendered)
	}
	if strings.Contains(rendered, "completion install") {
		t.Errorf("narrowing to spam should not show unrelated commands:\n%s", rendered)
	}
}
