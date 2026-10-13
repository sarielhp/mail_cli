package pgp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
)

// VerifyDetached checks a detached signature over data, the shape RFC 3156
// multipart/signed uses.
//
// Both are written to files rather than piped: gpg --verify takes the signed
// data as a file argument, and a temp file preserves the exact bytes the
// signature covers.
//
// The data is put in canonical form first. RFC 3156 signs the part with CRLF
// line endings, and a message stored with bare LF -- an mbox, a maildir
// written on Unix -- otherwise fails as a forgery.
func VerifyDetached(ctx context.Context, data, signature []byte) (Status, error) {
	data = canonicalLineEndings(data)
	dir, err := os.MkdirTemp("", "mail_cli-pgp-")
	if err != nil {
		return Status{}, fmt.Errorf("prepare signature check: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	dataPath := filepath.Join(dir, "signed-data")
	sigPath := filepath.Join(dir, "signature.asc")
	if writeErr := os.WriteFile(dataPath, data, 0600); writeErr != nil {
		return Status{}, fmt.Errorf("prepare signature check: %w", writeErr)
	}
	if writeErr := os.WriteFile(sigPath, signature, 0600); writeErr != nil {
		return Status{}, fmt.Errorf("prepare signature check: %w", writeErr)
	}

	res, err := execGPG(ctx, nil, "--verify", sigPath, dataPath)
	if err != nil {
		return Status{}, err
	}
	status, err := answer(res, "verify")
	return withSignerUIDs(ctx, status), err
}

// canonicalLineEndings turns every bare LF into CRLF, leaving CRLF alone.
func canonicalLineEndings(data []byte) []byte {
	if !bytes.Contains(data, []byte("\n")) {
		return data
	}
	lf := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
}

// VerifyInline checks a cleartext-signed block, which carries its own
// signature. gpg reads it from stdin.
func VerifyInline(ctx context.Context, block []byte) (Status, error) {
	res, err := execGPG(ctx, block, "--verify")
	if err != nil {
		return Status{}, err
	}
	status, err := answer(res, "verify")
	return withSignerUIDs(ctx, status), err
}

// Decrypt decrypts ciphertext -- armoured or binary -- and returns the
// plaintext. The status channel still reports any signature the message
// carried inside the encrypted payload.
func Decrypt(ctx context.Context, ciphertext []byte) ([]byte, Status, error) {
	res, err := execGPG(ctx, ciphertext, "--decrypt")
	if err != nil {
		return nil, Status{}, err
	}
	if res.LimitErr != nil {
		return nil, res.Status, errors.New("decrypt: " + res.LimitErr.Error())
	}
	// Success is DECRYPTION_OKAY and nothing else. A bare literal packet
	// "decrypts" with exit 0 and no such token, because nothing was
	// encrypted; treating the exit code as the answer rendered it as
	// encrypted mail. A non-zero exit after DECRYPTION_OKAY is a signature
	// problem inside the payload, and the status reports that separately.
	if !res.Status.Decrypted || res.Status.DecryptFailed {
		if res.ExitErr == nil && res.Diag == "" {
			return nil, res.Status, errors.New("decrypt: gpg did not report a successful decryption")
		}
		return nil, res.Status, errors.New("decrypt: " + reason(res))
	}
	return res.Stdout, withSignerUIDs(ctx, res.Status), nil
}

// SignDetached produces a binary detached signature over data.
func SignDetached(ctx context.Context, data []byte, keyID string) ([]byte, error) {
	args := []string{"--detach-sign", "--output", "-"}
	if keyID != "" {
		args = append(args, "--local-user", keyID)
	}
	res, err := execGPG(ctx, data, args...)
	if err != nil {
		return nil, err
	}
	if err := res.failure("sign"); err != nil {
		return nil, err
	}
	return res.Stdout, nil
}

// Encrypt encrypts data for every recipient, armoured so it fits in a text
// MIME part.
func Encrypt(ctx context.Context, data []byte, recipients []string) ([]byte, error) {
	return encrypt(ctx, data, encryptRequest{to: encryptTo{visible: recipients}})
}

// SignAndEncrypt signs and encrypts data in one message, which is what "signed
// and encrypted" means for PGP/MIME: a single payload rather than a signature
// wrapped in encryption.
func SignAndEncrypt(ctx context.Context, data []byte, keyID string, recipients []string) ([]byte, error) {
	return encrypt(ctx, data, encryptRequest{keyID: keyID, sign: true, to: encryptTo{visible: recipients}})
}

// encryptTo is who a message is encrypted for. A hidden recipient's key ID is
// left out of the ciphertext (--hidden-recipient), which every recipient
// receives: a Bcc recipient named there would be disclosed to all of them.
type encryptTo struct {
	visible []string
	hidden  []string
}

type encryptRequest struct {
	keyID string
	sign  bool
	to    encryptTo
}

func encrypt(ctx context.Context, data []byte, req encryptRequest) ([]byte, error) {
	if len(req.to.visible)+len(req.to.hidden) == 0 {
		return nil, errors.New("encrypt: no recipients")
	}
	args := []string{"--armor", "--output", "-", "--encrypt", "--yes"}
	if req.sign {
		// --sign must be passed even with no key named: gpg then signs with
		// its default key, whereas omitting it produces an unsigned message
		// that looks signed to the caller who asked for a signature.
		args = append(args, "--sign")
		if req.keyID != "" {
			args = append(args, "--local-user", req.keyID)
		}
	}
	for _, recipient := range req.to.visible {
		args = append(args, "--recipient", recipientSpec(recipient))
	}
	for _, recipient := range req.to.hidden {
		args = append(args, "--hidden-recipient", recipientSpec(recipient))
	}
	res, err := execGPG(ctx, data, args...)
	if err != nil {
		return nil, err
	}
	if err := res.failure("encrypt"); err != nil {
		return nil, err
	}
	return res.Stdout, nil
}

// HasPublicKey reports whether the keyring holds a key for recipient, so an
// encrypt can fail with "no key for bob@example.com" before it fails with gpg's
// less specific message.
func HasPublicKey(ctx context.Context, recipient string) bool {
	if recipient == "" {
		return false
	}
	res, err := execGPG(ctx, nil, "--list-keys", "--", recipientSpec(recipient))
	return err == nil && res.ExitErr == nil
}

// recipientSpec turns an address into gpg's exact-match form, "<addr>". A
// bare address is a substring search to gpg, so "bob@example.com" also
// selects "notbob@example.com" -- encrypting to whoever owns the longer one.
// Anything that is not an address, such as a key ID or fingerprint, is passed
// as written.
func recipientSpec(recipient string) string {
	recipient = strings.TrimSpace(recipient)
	if addr, err := mail.ParseAddress(recipient); err == nil {
		return "<" + addr.Address + ">"
	}
	return recipient
}

// answer reads a verification or decryption result, treating a non-zero exit as
// the answer when the status channel already gave one.
func answer(res gpgResult, op string) (Status, error) {
	if res.LimitErr != nil {
		return Status{}, errors.New(op + ": " + res.LimitErr.Error())
	}
	if res.ExitErr != nil && !res.Status.decisive() {
		return res.Status, errors.New(op + ": " + reason(res))
	}
	return res.Status, nil
}

func reason(res gpgResult) string {
	if res.Diag != "" {
		return res.Diag
	}
	if res.ExitErr != nil {
		return res.ExitErr.Error()
	}
	return "no diagnostic from gpg"
}
