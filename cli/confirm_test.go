package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/app"
)

func confirmSession(stdin string) (*Session, *bytes.Buffer) {
	var status bytes.Buffer
	return &Session{In: strings.NewReader(stdin), Err: &status}, &status
}

func TestConfirmWithYesDoesNotAsk(t *testing.T) {
	session, status := confirmSession("")

	if err := session.Confirm("empty the Spam folder", true); err != nil {
		t.Fatalf("Confirm with --yes: %v", err)
	}
	if status.Len() != 0 {
		t.Errorf("expected no prompt with --yes, got %q", status.String())
	}
}

// A cron line has nobody to ask. Assuming yes there is how an irreversible
// command runs unattended; assuming no makes it silently do nothing. It
// refuses, and says which flag to add.
func TestConfirmRefusesWithoutATerminal(t *testing.T) {
	session, _ := confirmSession("y\n")

	err := session.Confirm("empty the Spam folder", false)
	if err == nil {
		t.Fatal("expected a refusal when stdin is not a terminal")
	}
	if !errors.Is(err, app.ErrUsage) {
		t.Errorf("a missing --yes is a usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("the refusal should name the flag, got %q", err)
	}
	if !strings.Contains(err.Error(), "empty the Spam folder") {
		t.Errorf("the refusal should name the action, got %q", err)
	}
}

// Declining is not an error the caller reports as a failure.
func TestErrDeclinedIsDistinct(t *testing.T) {
	if errors.Is(ErrDeclined, app.ErrUsage) {
		t.Error("declining is not a usage error")
	}
}
