package pgp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sarielhp/mail_cli/email"
)

// Recipients is who a message is addressed to, split by whether that is
// visible to the others: To and Cc are, Bcc is not.
type Recipients struct {
	Visible []string
	Hidden  []string
}

// OutboundOptions is how an outgoing message should be protected.
type OutboundOptions struct {
	// Sign asks for a signature over the message entity.
	Sign bool
	// Encrypt asks for the entity to be encrypted to Recipients.
	Encrypt bool
	// KeyID names the signing key. Empty means gpg's default key.
	KeyID string
	// Recipients are the addresses to encrypt to. Their key IDs are written
	// into the ciphertext.
	Recipients []string
	// HiddenRecipients are encrypted to as well, without their key IDs in
	// the ciphertext: they are the Bcc addresses, and a key ID every
	// recipient can read would tell them who else got the message.
	HiddenRecipients []string
}

// Protect applies signing and encryption to a composed message and returns the
// bytes to transmit.
//
// Signing wraps the entity in multipart/signed and encrypting wraps it in
// multipart/encrypted, both as RFC 3156 defines them. Asking for both produces
// one signed-and-encrypted payload rather than a signature wrapped in a second
// layer: a single PGP message that is both is what a reader's client verifies
// and decrypts in one step, and it is what "signed and encrypted" means.
func Protect(ctx context.Context, msg email.BuiltMessage, opts OutboundOptions) ([]byte, error) {
	if !opts.Sign && !opts.Encrypt {
		return msg.Bytes(), nil
	}
	if opts.Encrypt {
		return protectEncrypted(ctx, msg, opts)
	}
	return protectSigned(ctx, msg, opts)
}

// protectSigned wraps the entity in multipart/signed.
func protectSigned(ctx context.Context, msg email.BuiltMessage, opts OutboundOptions) ([]byte, error) {
	signature, err := SignDetached(ctx, msg.Entity, opts.KeyID)
	if err != nil {
		return nil, fmt.Errorf("signing message: %w", err)
	}
	boundary := newBoundary()
	var buf bytes.Buffer
	buf.Write(msg.Headers)
	fmt.Fprintf(&buf, "Content-Type: multipart/signed; micalg=pgp-sha256; "+
		"protocol=\"application/pgp-signature\"; boundary=\"%s\"\r\n\r\n", boundary)
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.Write(msg.Entity)
	fmt.Fprintf(&buf, "\r\n--%s\r\n", boundary)
	buf.WriteString("Content-Type: application/pgp-signature; name=\"signature.asc\"\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	buf.WriteString(wrapBase64(signature, 76))
	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)
	return buf.Bytes(), nil
}

// protectEncrypted wraps the entity in multipart/encrypted.
func protectEncrypted(ctx context.Context, msg email.BuiltMessage, opts OutboundOptions) ([]byte, error) {
	to, err := encryptionTargets(ctx, opts)
	if err != nil {
		return nil, err
	}
	ciphertext, err := encrypt(ctx, msg.Entity, encryptRequest{keyID: opts.KeyID, sign: opts.Sign, to: to})
	if err != nil {
		return nil, fmt.Errorf("encrypting message: %w", err)
	}

	boundary := newBoundary()
	var buf bytes.Buffer
	buf.Write(msg.Headers)
	fmt.Fprintf(&buf, "Content-Type: multipart/encrypted; "+
		"protocol=\"application/pgp-encrypted\"; boundary=\"%s\"\r\n\r\n", boundary)
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: application/pgp-encrypted\r\n\r\nVersion: 1\r\n")
	fmt.Fprintf(&buf, "\r\n--%s\r\n", boundary)
	buf.WriteString("Content-Type: application/octet-stream; name=\"encrypted.asc\"\r\n\r\n")
	buf.Write(ciphertext)
	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)
	return buf.Bytes(), nil
}

// encryptionTargets checks every recipient, visible and hidden, against the
// keyring and splits them again. An address that is both visible and hidden
// is visible: its key ID is in the ciphertext either way.
func encryptionTargets(ctx context.Context, opts OutboundOptions) (encryptTo, error) {
	all := append(append([]string{}, opts.Recipients...), opts.HiddenRecipients...)
	if _, err := EncryptableRecipients(ctx, all); err != nil {
		return encryptTo{}, err
	}
	visible := dedupe(opts.Recipients)
	seen := make(map[string]bool, len(visible))
	for _, recipient := range visible {
		seen[strings.ToLower(recipient)] = true
	}
	to := encryptTo{visible: visible}
	for _, recipient := range dedupe(opts.HiddenRecipients) {
		if !seen[strings.ToLower(recipient)] {
			to.hidden = append(to.hidden, recipient)
		}
	}
	return to, nil
}

// EncryptableRecipients checks every recipient against the keyring and returns
// the ones that can be encrypted to.
//
// It fails rather than silently dropping the ones it cannot: a message sent
// encrypted-but-actually-in-the-clear to one of its recipients is worse than a
// message that was not sent, and the error names who is missing a key.
func EncryptableRecipients(ctx context.Context, recipients []string) ([]string, error) {
	var usable, missing []string
	for _, recipient := range dedupe(recipients) {
		if HasPublicKey(ctx, recipient) {
			usable = append(usable, recipient)
			continue
		}
		missing = append(missing, recipient)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("cannot encrypt: no public key for %s", strings.Join(missing, ", "))
	}
	if len(usable) == 0 {
		return nil, errors.New("cannot encrypt: no recipients")
	}
	return usable, nil
}

func dedupe(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

// newBoundary makes a MIME boundary that cannot appear in the content it
// delimits.
func newBoundary() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "mailcli-" + hex.EncodeToString(b)
}

// wrapBase64 encodes data in 76-character lines terminated with CRLF, which is
// what a MIME body requires.
func wrapBase64(data []byte, lineLen int) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	var sb strings.Builder
	for len(encoded) > lineLen {
		sb.WriteString(encoded[:lineLen])
		sb.WriteString("\r\n")
		encoded = encoded[lineLen:]
	}
	if encoded != "" {
		sb.WriteString(encoded)
		sb.WriteString("\r\n")
	}
	return sb.String()
}
