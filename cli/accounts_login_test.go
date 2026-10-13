package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"

	"github.com/fatih/color"
)

// flakyClient fails Validate a set number of times, then succeeds.
type flakyClient struct {
	mailclient.MailClient
	failures int
	calls    int
}

func (f *flakyClient) Validate() error {
	f.calls++
	if f.calls <= f.failures {
		return errors.New("context deadline exceeded")
	}
	return nil
}

func (f *flakyClient) Config() *cfg_g.Config { return &cfg_g.Config{} }

// noRetryDelay removes the wait before the retry, which is there for a real
// server and only slows a test down.
func noRetryDelay(t *testing.T) {
	t.Helper()
	orig := loginRetryDelay
	loginRetryDelay = 0
	t.Cleanup(func() { loginRetryDelay = orig })
}

func quiet(t *testing.T, fn func()) {
	t.Helper()
	origOut, origColor := os.Stdout, color.Output
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	os.Stdout, color.Output = devNull, devNull
	defer func() {
		os.Stdout, color.Output = origOut, origColor
		_ = devNull.Close()
	}()
	fn()
}

// A server can be slow to honour a token it has only just issued. Graph
// answered the first request after a device-code login with a timeout, which
// was reported as a failed login even though the credential was on disk.
func TestValidateAfterLoginRetriesOnce(t *testing.T) {
	noRetryDelay(t)
	client := &flakyClient{failures: 1}

	var err error
	quiet(t, func() { err = validateAfterLogin(io.Discard, client) })

	if err != nil {
		t.Errorf("validateAfterLogin = %v, want nil on a transient failure", err)
	}
	if client.calls != 2 {
		t.Errorf("Validate called %d times, want 2 (the first attempt and one retry)", client.calls)
	}
}

func TestValidateAfterLoginSucceedsWithoutRetry(t *testing.T) {
	client := &flakyClient{}

	var err error
	quiet(t, func() { err = validateAfterLogin(io.Discard, client) })

	if err != nil {
		t.Fatalf("validateAfterLogin = %v, want nil", err)
	}
	if client.calls != 1 {
		t.Errorf("Validate called %d times, want 1 when the first attempt works", client.calls)
	}
}

// A persistent failure is still an error, but the message has to say the
// credential was saved -- otherwise the user repeats the whole browser flow.
func TestValidateAfterLoginReportsCredentialSaved(t *testing.T) {
	noRetryDelay(t)
	client := &flakyClient{failures: 99}

	var err error
	quiet(t, func() { err = validateAfterLogin(io.Discard, client) })

	if err == nil {
		t.Fatal("validateAfterLogin succeeded despite Validate always failing")
	}
	if client.calls != 2 {
		t.Errorf("Validate called %d times, want 2", client.calls)
	}
	for _, want := range []string{"saved", "account test"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// An account with no username failed its post-login check before contacting
// the server -- after the old token was already deleted -- and then reported
// "signed in ... the credential has been saved". The check now runs first and
// says how to set the username.
func TestAccountSettingsErrorSaysHowToSetTheUsername(t *testing.T) {
	err := accountSettingsError("sariel_test", cfg_g.ValidateAccountParams(cfg_acc.AccountConfig{Name: "sariel_test", Type: "outlook"}))
	msg := err.Error()
	for _, want := range []string{"cannot sign in", "username", "-A sariel_test config set username", "account login sariel_test"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
	if strings.Contains(msg, "signed in,") {
		t.Errorf("error claims a sign-in that never happened: %q", msg)
	}
}
