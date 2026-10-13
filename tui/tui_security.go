package tui

import (
	"strings"

	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/tui/render"
)

// securityPill is the one-line PGP verdict shown above a message's headers.
//
// The shape is a bracket, a glyph and a word, so the verdict survives a
// terminal that renders no colour: the glyph and the word are the message, and
// the colour is the emphasis.
func securityPill(status *email.CryptoStatus) (string, render.ColorID) {
	if status == nil {
		return "", render.ColorDefault
	}
	switch {
	case status.Signed && status.Verdict == email.VerdictValid:
		return "[✓ PGP Signed: " + signerShort(status.Signer) + "]", render.ColorBorder
	case status.Signed && status.Verdict == email.VerdictUnverifiedKey:
		return "[~ PGP Good sig, unverified key: " + signerShort(status.Signer) + "]", render.ColorHighlightFg
	case status.Signed && status.Verdict == email.VerdictSenderMismatch:
		return "[✗ PGP Signer is not From: " + signerShort(status.Signer) + "]", render.ColorSpam
	case status.Signed && status.Verdict == email.VerdictInvalid:
		return "[✗ PGP Invalid Signature]", render.ColorSpam
	case status.Signed && status.Verdict == email.VerdictUnknownKey:
		return "[? PGP Unknown Key]", render.ColorHighlightFg
	case status.Signed && status.Verdict == email.VerdictExpired:
		return "[! PGP Expired Key]", render.ColorHighlightFg
	case status.Signed && status.Verdict == email.VerdictRevoked:
		return "[✗ PGP Revoked Key]", render.ColorSpam
	case status.Signed:
		return "[· PGP Signed, unchecked]", render.ColorDim
	case status.Decrypted:
		return "[🔒 PGP Decrypted]", render.ColorSender
	case status.Encrypted:
		return "[🔒 PGP Encrypted]", render.ColorDim
	}
	return "", render.ColorDefault
}

// signerShort is the signer's user id with the display name trimmed to fit a
// pill. The address is kept whole: a display name is whatever the key's
// creator typed, and the address is what tells "Alice" from someone calling
// themselves Alice.
func signerShort(signer string) string {
	signer = strings.TrimSpace(signer)
	if signer == "" {
		return "unknown"
	}
	name, addr := signer, ""
	if idx := strings.IndexByte(signer, '<'); idx >= 0 {
		name, addr = strings.TrimSpace(signer[:idx]), strings.TrimSpace(signer[idx:])
	}
	if runes := []rune(name); len(runes) > 24 {
		name = strings.TrimRight(string(runes[:24]), " ") + "…"
	}
	return strings.TrimSpace(name + " " + addr)
}
