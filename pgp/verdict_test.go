package pgp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/email"
)

const (
	goodLine  = "[GNUPG:] GOODSIG 0123456789ABCDEF Alice <alice@example.com>"
	validLine = "[GNUPG:] VALIDSIG AAAA0123456789ABCDEF0123456789ABCDEF 2026-10-12 1 0 4 0 1 8 00 AAAA0123456789ABCDEF0123456789ABCDEF"
)

// judge runs a status transcript through the same path Process uses.
func judge(t *testing.T, from string, lines ...string) *email.CryptoStatus {
	t.Helper()
	raw := []byte("From: " + from + "\r\nContent-Type: text/plain\r\n\r\nbody\r\n")
	p := &processing{raw: raw, status: &email.CryptoStatus{Signed: true}}
	p.applySignature(parseStatus(strings.Join(lines, "\n")), nil)
	return p.status
}

func TestVerdictFromStatusTranscript(t *testing.T) {
	tests := []struct {
		name        string
		from        string
		lines       []string
		wantVerdict string
		wantValid   bool
	}{
		{"fully trusted and From matches", "alice@example.com",
			[]string{"[GNUPG:] NEWSIG", goodLine, validLine, "[GNUPG:] TRUST_FULLY 0 pgp"},
			email.VerdictValid, true},
		{"ultimate trust", "Alice <ALICE@example.com>",
			[]string{"[GNUPG:] NEWSIG", goodLine, validLine, "[GNUPG:] TRUST_ULTIMATE 0 pgp"},
			email.VerdictValid, true},
		{"undefined trust is a good signature, unverified key", "alice@example.com",
			[]string{"[GNUPG:] NEWSIG", goodLine, validLine, "[GNUPG:] TRUST_UNDEFINED 0 pgp"},
			email.VerdictUnverifiedKey, false},
		{"marginal trust is not verified", "alice@example.com",
			[]string{"[GNUPG:] NEWSIG", goodLine, validLine, "[GNUPG:] TRUST_MARGINAL 0 pgp"},
			email.VerdictUnverifiedKey, false},
		{"no trust token at all is not verified", "alice@example.com",
			[]string{"[GNUPG:] NEWSIG", goodLine, validLine},
			email.VerdictUnverifiedKey, false},
		{"GOODSIG without VALIDSIG is not believed", "alice@example.com",
			[]string{"[GNUPG:] NEWSIG", goodLine, "[GNUPG:] TRUST_FULLY 0 pgp"},
			email.VerdictError, false},
		{"good beside bad is invalid", "alice@example.com",
			[]string{"[GNUPG:] NEWSIG", goodLine, validLine, "[GNUPG:] TRUST_FULLY 0 pgp",
				"[GNUPG:] NEWSIG", "[GNUPG:] BADSIG 1111222233334444 Mallory <m@example.com>"},
			email.VerdictInvalid, false},
		{"good beside a missing key is not verified", "alice@example.com",
			[]string{"[GNUPG:] NEWSIG", goodLine, validLine, "[GNUPG:] TRUST_FULLY 0 pgp",
				"[GNUPG:] NEWSIG", "[GNUPG:] ERRSIG 1111222233334444 1 8 00 1 9 -"},
			email.VerdictUnknownKey, false},
		{"a signer that is not the sender", "bob@example.com",
			[]string{"[GNUPG:] NEWSIG", goodLine, validLine, "[GNUPG:] TRUST_FULLY 0 pgp"},
			email.VerdictSenderMismatch, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := judge(t, tc.from, tc.lines...)
			if status.Verdict != tc.wantVerdict || status.ValidSig != tc.wantValid {
				t.Fatalf("verdict = %q valid = %v, want %q valid = %v (%+v)",
					status.Verdict, status.ValidSig, tc.wantVerdict, tc.wantValid, status)
			}
			if tc.wantVerdict == email.VerdictSenderMismatch && !status.SenderMismatch {
				t.Error("SenderMismatch was not set")
			}
		})
	}
}

// The signature that decided the verdict is the one named, not whichever
// signature gpg happened to report last.
func TestVerdictNamesTheDecidingSignature(t *testing.T) {
	status := judge(t, "alice@example.com",
		"[GNUPG:] NEWSIG", "[GNUPG:] BADSIG 1111222233334444 Mallory <m@example.com>",
		"[GNUPG:] NEWSIG", goodLine, validLine, "[GNUPG:] TRUST_FULLY 0 pgp")
	if status.KeyID != "1111222233334444" || !strings.Contains(status.Signer, "Mallory") {
		t.Fatalf("status = %+v, want the bad signature named", status)
	}
}

// fakeGPG puts a gpg on PATH that runs script, so a test can control exactly
// which descriptor says what.
func fakeGPG(t *testing.T, script string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "gpg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// A status line on stderr is prose, whatever it looks like: gpg repeats text
// the message controls there, which is how a crafted message forged GOODSIG
// when the status channel shared that stream (CVE-2018-12020).
func TestStatusIsReadOnlyFromItsOwnDescriptor(t *testing.T) {
	fakeGPG(t, `echo '`+goodLine+`' >&2
echo '`+validLine+`' >&2
echo '[GNUPG:] TRUST_ULTIMATE 0 pgp' >&2
echo '[GNUPG:] NEWSIG' >&3
echo '[GNUPG:] BADSIG 0123456789ABCDEF Alice <alice@example.com>' >&3
exit 1`)
	status, err := VerifyInline(context.Background(), []byte("block"))
	if err != nil {
		t.Fatalf("VerifyInline: %v", err)
	}
	if len(status.Signatures) != 1 || status.Signatures[0].Outcome != OutcomeBad {
		t.Fatalf("status = %+v, want only the BADSIG from descriptor 3", status)
	}
}

// A good signature from a key in the keyring that nobody certified is a good
// signature by an unverified key -- not VERIFIED.
func TestProcessReportsAnUncertifiedKeyAsUnverified(t *testing.T) {
	keyring(t)
	otherHome := t.TempDir()
	gpgIn := func(home string, stdin []byte, args ...string) []byte {
		t.Helper()
		cmd := exec.Command("gpg", append([]string{"--batch", "--pinentry-mode", "loopback", "--passphrase", ""}, args...)...)
		cmd.Env = append(os.Environ(), "GNUPGHOME="+home)
		cmd.Stdin = strings.NewReader(string(stdin))
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("gpg %v: %v", args, err)
		}
		return out
	}
	gpgIn(otherHome, nil, "--quick-generate-key", "Stranger <stranger@example.com>", "default", "default", "never")
	pub := gpgIn(otherHome, nil, "--armor", "--export", "stranger@example.com")
	gpgIn(os.Getenv("GNUPGHOME"), pub, "--import")
	block := gpgIn(otherHome, []byte("hello\r\n"), "--clearsign", "--output", "-")

	raw := append([]byte("From: stranger@example.com\r\nContent-Type: text/plain\r\n\r\n"), block...)
	result := Process(context.Background(), raw, Options{Verify: true})
	if result.Status == nil || result.Status.Verdict != email.VerdictUnverifiedKey || result.Status.ValidSig {
		t.Fatalf("status = %+v, want a good signature by an unverified key", result.Status)
	}
	if result.Status.Tag() == "VERIFIED" {
		t.Fatal("an uncertified key was shown as VERIFIED")
	}
}
