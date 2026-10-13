package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/tui/render"
)

func TestSecurityPill(t *testing.T) {
	tests := []struct {
		name      string
		status    *email.CryptoStatus
		want      string
		wantColor render.ColorID
	}{
		{"none", nil, "", render.ColorDefault},
		{
			"valid signature",
			&email.CryptoStatus{Signed: true, ValidSig: true, Verdict: email.VerdictValid, Signer: "Alice <alice@example.com>"},
			"[✓ PGP Signed: Alice <alice@example.com>]", render.ColorBorder,
		},
		{
			"a good signature by an unverified key is not the verified pill",
			&email.CryptoStatus{Signed: true, Verdict: email.VerdictUnverifiedKey, Signer: "Alice <alice@example.com>"},
			"[~ PGP Good sig, unverified key: Alice <alice@example.com>]", render.ColorHighlightFg,
		},
		{
			"a signer who is not the sender",
			&email.CryptoStatus{Signed: true, Verdict: email.VerdictSenderMismatch, SenderMismatch: true, Signer: "Alice <mallory@example.com>"},
			"[✗ PGP Signer is not From: Alice <mallory@example.com>]", render.ColorSpam,
		},
		{
			"invalid signature",
			&email.CryptoStatus{Signed: true, Verdict: email.VerdictInvalid},
			"[✗ PGP Invalid Signature]", render.ColorSpam,
		},
		{
			"unknown key",
			&email.CryptoStatus{Signed: true, Verdict: email.VerdictUnknownKey},
			"[? PGP Unknown Key]", render.ColorHighlightFg,
		},
		{
			"decrypted",
			&email.CryptoStatus{Encrypted: true, Decrypted: true},
			"[🔒 PGP Decrypted]", render.ColorSender,
		},
		{
			"still encrypted",
			&email.CryptoStatus{Encrypted: true},
			"[🔒 PGP Encrypted]", render.ColorDim,
		},
		{
			"a signature that was never checked is not a verdict",
			&email.CryptoStatus{Signed: true},
			"[· PGP Signed, unchecked]", render.ColorDim,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, color := securityPill(tc.status)
			if got != tc.want {
				t.Errorf("pill = %q, want %q", got, tc.want)
			}
			if color != tc.wantColor {
				t.Errorf("color = %v, want %v", color, tc.wantColor)
			}
		})
	}
}

func TestSignerShortFallsBackToSomethingUsable(t *testing.T) {
	if got := signerShort(""); got != "unknown" {
		t.Errorf("signerShort(\"\") = %q", got)
	}
	if got := signerShort("Alice Example <alice@example.com>"); got != "Alice Example <alice@example.com>" {
		t.Errorf("signerShort = %q, want the display name and the address", got)
	}
	longName := strings.Repeat("b", 40) + " <bob@example.com>"
	if got := signerShort(longName); !strings.HasSuffix(got, "… <bob@example.com>") {
		t.Errorf("signerShort(long name) = %q, want the name cut and the address kept", got)
	}
	long := strings.Repeat("a", 40)
	if got := signerShort(long); len([]rune(got)) > 25 || !strings.HasSuffix(got, "…") {
		t.Errorf("signerShort(long) = %q, want it truncated with an ellipsis", got)
	}
}
