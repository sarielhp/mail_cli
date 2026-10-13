package outlook

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/report"
)

// The device-code flow prints a URL and a code, then polls for up to fifteen
// minutes waiting for a person to open a browser. Reached from a cron scan or a
// test there is no such person, and the process simply looks busy until the
// code expires -- which is how a live read-only test run stalled for ten
// minutes on an account whose token had lapsed.
//
// With nobody to ask it must fail and say what to run, the same rule every
// other prompt in this program follows.
func TestDeviceCodeSignInRefusesWithoutATerminal(t *testing.T) {
	restore := interactiveAuthPossible
	interactiveAuthPossible = func() bool { return false }
	defer func() { interactiveAuthPossible = restore }()

	tok, err := getNewTokenViaDeviceCode(context.Background(), report.Discard())

	if err == nil {
		t.Fatalf("got token %v; an unattended process must not start a browser sign-in", tok)
	}
	if !errors.Is(err, ErrInteractiveAuthRequired) {
		t.Errorf("got %v, want ErrInteractiveAuthRequired so callers can recognise it", err)
	}
	if !strings.Contains(err.Error(), "account login") {
		t.Errorf("the error should name the command that fixes it, got: %v", err)
	}
}

// The refusal happens before any network call, so a lapsed token fails
// immediately rather than after a request to the device-code endpoint.
func TestDeviceCodeRefusalCostsNoNetworkCall(t *testing.T) {
	restore := interactiveAuthPossible
	interactiveAuthPossible = func() bool { return false }
	defer func() { interactiveAuthPossible = restore }()

	// A cancelled context would surface as a transport error if the function
	// reached the network; the refusal must win.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := getNewTokenViaDeviceCode(ctx, report.Discard())
	if !errors.Is(err, ErrInteractiveAuthRequired) {
		t.Errorf("got %v, want the refusal to precede any request", err)
	}
}

// When a terminal is present (e.g. running inside TUI), but the reporter cannot
// prompt the user directly (such as TUI ProgramReporter or Discard), the flow
// must refuse immediately without polling.
func TestDeviceCodeSignInRefusesWhenReporterCannotPrompt(t *testing.T) {
	restore := interactiveAuthPossible
	interactiveAuthPossible = func() bool { return true }
	defer func() { interactiveAuthPossible = restore }()

	tok, err := getNewTokenViaDeviceCode(context.Background(), report.Discard())
	if err == nil {
		t.Fatalf("got token %v; non-prompting reporter must not start device-code sign-in", tok)
	}
	if !errors.Is(err, ErrInteractiveAuthRequired) {
		t.Errorf("got %v, want ErrInteractiveAuthRequired", err)
	}
}
