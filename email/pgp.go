package email

import (
	"bytes"
	"encoding/base64"
	"mime"
	"net/mail"
	"strings"
)

// CryptoStatus is what a PGP operation reported about a message. It is pure
// data: nothing in this package runs gpg, so a message can be parsed, listed
// and cached on a machine with no keyring at all.
type CryptoStatus struct {
	Encrypted bool `json:"encrypted,omitempty"`
	Decrypted bool `json:"decrypted,omitempty"`
	Signed    bool `json:"signed,omitempty"`
	// ValidSig is true only for VerdictValid: every signature is good, gpg
	// validated each one, the keys are fully or ultimately trusted and one of
	// them carries the From address. A good signature from a key nobody
	// vouched for is not "valid" to a consumer reading this field.
	ValidSig    bool   `json:"valid,omitempty"`
	KeyID       string `json:"key_id,omitempty"`
	Signer      string `json:"signer,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	// Trust is gpg's validity for the signing key: never, undefined,
	// marginal, fully, ultimate, or unknown when gpg reported none.
	Trust string `json:"trust,omitempty"`
	// SenderMismatch is set when the signature is good but no address on the
	// signing key is the message's From address.
	SenderMismatch bool `json:"sender_mismatch,omitempty"`
	// Verdict is the nuance the booleans cannot carry. ValidSig answers "is
	// this verified"; Verdict distinguishes a forged signature from one whose
	// key is simply not in the keyring -- the difference between a warning
	// and an accusation -- and a good signature by an unverified key from one
	// by a key the user trusts.
	Verdict string `json:"verdict,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Signature verdicts.
const (
	// VerdictValid is a good signature by a fully trusted key whose
	// addresses include the From address.
	VerdictValid = "valid"
	// VerdictUnverifiedKey is a good signature by a key whose owner nobody
	// vouched for: the bytes are intact, the identity is unconfirmed.
	VerdictUnverifiedKey = "unverified-key"
	// VerdictSenderMismatch is a good signature by a key that does not carry
	// the From address: someone signed it, not necessarily the sender.
	VerdictSenderMismatch = "sender-mismatch"
	// VerdictInvalid is a signature that did not.
	VerdictInvalid = "invalid"
	// VerdictUnknownKey is a signature that cannot be checked because the
	// signer's public key is not in the keyring.
	VerdictUnknownKey = "unknown-key"
	// VerdictExpired is a signature by a key that has expired.
	VerdictExpired = "expired"
	// VerdictRevoked is a signature by a revoked key.
	VerdictRevoked = "revoked"
	// VerdictError is a check that could not run at all.
	VerdictError = "error"
)

// Tag is the one-word verdict shown beside a message: the thing a reader needs
// to see before deciding whether the sender is who the message claims.
//
// A signature is reported ahead of the encryption, because "the message is
// encrypted" is visible from the fact that it could not be read otherwise,
// while a forged signature is not visible at all.
func (c *CryptoStatus) Tag() string {
	switch {
	case c.Signed && c.Verdict == VerdictValid:
		return "VERIFIED"
	case c.Signed && c.Verdict == VerdictUnverifiedKey:
		return "UNVERIFIED KEY"
	case c.Signed && c.Verdict == VerdictSenderMismatch:
		return "SENDER MISMATCH"
	case c.Signed && c.Verdict == VerdictInvalid:
		return "INVALID"
	case c.Signed && c.Verdict == VerdictUnknownKey:
		return "UNKNOWN KEY"
	case c.Signed && c.Verdict == VerdictExpired:
		return "EXPIRED KEY"
	case c.Signed && c.Verdict == VerdictRevoked:
		return "REVOKED KEY"
	case c.Signed && c.Verdict == VerdictError:
		return "UNCHECKED"
	case c.Signed:
		return "SIGNED"
	case c.Decrypted:
		return "DECRYPTED"
	case c.Encrypted:
		return "ENCRYPTED"
	}
	return ""
}

// Description is the sentence after the tag.
func (c *CryptoStatus) Description() string {
	prefix := ""
	if c.Decrypted {
		prefix = "Decrypted; "
	}
	switch {
	case c.Signed && c.Verdict == VerdictValid:
		return prefix + "Signed by " + c.signerName() + c.keySuffix()
	case c.Signed && c.Verdict == VerdictUnverifiedKey:
		return prefix + "Good signature, unverified key: " + c.signerName() + c.keySuffix()
	case c.Signed && c.Verdict == VerdictSenderMismatch:
		return prefix + "Good signature by " + c.signerName() + c.keySuffix() + ", not the From address"
	case c.Signed && c.Verdict == VerdictInvalid:
		return prefix + "Signature verification failed!"
	case c.Signed && c.Verdict == VerdictUnknownKey:
		return prefix + "Signed by a key that is not in your keyring" + c.keySuffix()
	case c.Signed && c.Verdict == VerdictExpired:
		return prefix + "Signed by an expired key" + c.keySuffix()
	case c.Signed && c.Verdict == VerdictRevoked:
		return prefix + "Signed by a revoked key" + c.keySuffix()
	case c.Signed && c.Verdict == VerdictError:
		return prefix + "Signature not checked: " + c.Error
	case c.Signed:
		return prefix + "Signed, not checked"
	case c.Decrypted:
		return "Encrypted, decrypted"
	case c.Encrypted:
		return "Encrypted"
	}
	return c.Error
}

// signerName is the signer's user id, or the key id when gpg named no user.
func (c *CryptoStatus) signerName() string {
	if c.Signer != "" {
		return c.Signer
	}
	if c.KeyID != "" {
		return "key " + c.KeyID
	}
	return "an unknown signer"
}

// keySuffix names the key, when the signer's name does not already carry it.
func (c *CryptoStatus) keySuffix() string {
	if c.KeyID == "" {
		return ""
	}
	return " (Key 0x" + strings.ToUpper(c.KeyID) + ")"
}

// PGPKind classifies how a message carries PGP content.
type PGPKind int

const (
	// PGPNone is a message with no PGP content at all.
	PGPNone PGPKind = iota
	// PGPSignedMIME is RFC 3156 multipart/signed: a detached signature over
	// the first part.
	PGPSignedMIME
	// PGPEncryptedMIME is RFC 3156 multipart/encrypted.
	PGPEncryptedMIME
	// PGPSignedInline is a cleartext -----BEGIN PGP SIGNED MESSAGE----- block.
	PGPSignedInline
	// PGPEncryptedInline is a -----BEGIN PGP MESSAGE----- block in the body.
	PGPEncryptedInline
)

// PGPEnvelope is the PGP material a message carries, pulled out verbatim.
//
// Data is what gets verified or decrypted: the exact bytes of the signed part
// for PGPSignedMIME, and the ciphertext for the encrypted kinds. Signature is
// the detached signature, and is set only for PGPSignedMIME.
type PGPEnvelope struct {
	Kind      PGPKind
	Data      []byte
	Signature []byte
}

// HasPGP reports whether a message carries any PGP content.
func (e PGPEnvelope) HasPGP() bool { return e.Kind != PGPNone }

// SplitPGP inspects a raw message and returns the PGP material it carries.
//
// The signed bytes are sliced out of the raw message rather than rebuilt from
// parsed headers: a detached signature covers the part's headers, their order
// and their exact spacing, and a MIMEHeader is a map that has already lost the
// first two. Rebuilding the part would produce a byte stream that is almost
// right, which verifies as a forged signature rather than as a parse error.
func SplitPGP(raw []byte) PGPEnvelope {
	body := rawBody(raw)
	mediaType, params, err := topMediaType(raw)
	if err != nil {
		return inlineEnvelope(rawBodyText(raw))
	}

	switch mediaType {
	case "multipart/signed":
		if !strings.EqualFold(params["protocol"], "application/pgp-signature") {
			return PGPEnvelope{}
		}
		return mimeSignedEnvelope(body, params["boundary"])
	case "multipart/encrypted":
		if !strings.EqualFold(params["protocol"], "application/pgp-encrypted") {
			return PGPEnvelope{}
		}
		return mimeEncryptedEnvelope(body, params["boundary"])
	}
	return inlineEnvelope(rawBodyText(raw))
}

// mimeSignedEnvelope pulls the signed part and its detached signature out of a
// multipart/signed body. RFC 3156 Section 5 requires exactly two parts.
func mimeSignedEnvelope(body []byte, boundary string) PGPEnvelope {
	parts := splitRawParts(body, boundary)
	if len(parts) != 2 {
		return PGPEnvelope{}
	}
	signature := signatureOf(parts[1])
	if len(signature) == 0 {
		return PGPEnvelope{}
	}
	return PGPEnvelope{Kind: PGPSignedMIME, Data: parts[0], Signature: signature}
}

// mimeEncryptedEnvelope pulls the ciphertext out of a multipart/encrypted body.
// The first part is the control block ("Version: 1") and carries nothing to
// decrypt; the second is the message itself.
func mimeEncryptedEnvelope(body []byte, boundary string) PGPEnvelope {
	parts := splitRawParts(body, boundary)
	if len(parts) < 2 {
		return PGPEnvelope{}
	}
	ciphertext := partBody(parts[1])
	if len(ciphertext) == 0 {
		return PGPEnvelope{}
	}
	return PGPEnvelope{Kind: PGPEncryptedMIME, Data: ciphertext}
}

// inlineEnvelope looks for a cleartext PGP block in a message's body text.
func inlineEnvelope(text string) PGPEnvelope {
	if block, ok := blockBetween(text, "-----BEGIN PGP SIGNED MESSAGE-----", "-----END PGP SIGNATURE-----"); ok {
		return PGPEnvelope{Kind: PGPSignedInline, Data: []byte(block)}
	}
	if block, ok := blockBetween(text, "-----BEGIN PGP MESSAGE-----", "-----END PGP MESSAGE-----"); ok {
		return PGPEnvelope{Kind: PGPEncryptedInline, Data: []byte(block)}
	}
	return PGPEnvelope{}
}

// blockBetween returns the text from begin through the end marker, inclusive.
func blockBetween(text, begin, end string) (string, bool) {
	start := strings.Index(text, begin)
	if start < 0 {
		return "", false
	}
	stop := strings.Index(text[start:], end)
	if stop < 0 {
		return "", false
	}
	return text[start : start+stop+len(end)], true
}

// signatureOf decodes the body of a signature part, which is base64 in every
// message seen in practice and bare armour otherwise.
func signatureOf(part []byte) []byte {
	raw := partBody(part)
	decoded, err := base64.StdEncoding.DecodeString(stripWhitespace(string(raw)))
	if err != nil {
		return raw
	}
	return decoded
}

// partBody returns a part's payload with its transfer encoding undone.
func partBody(part []byte) []byte {
	header, body := SplitHeaderBody(part)
	encoding := headerValue(header, "Content-Transfer-Encoding")
	decoded, err := DecodePartBody(bytes.NewReader(body), encoding)
	if err != nil {
		return body
	}
	return decoded
}

// rawBody returns everything after the message's header block.
func rawBody(raw []byte) []byte {
	_, body := SplitHeaderBody(raw)
	return body
}

// SplitHeaderBody cuts a message or part at the blank line that ends its header
// block, accepting either CRLF or bare LF.
func SplitHeaderBody(raw []byte) (header, body []byte) {
	if idx := bytes.Index(raw, []byte("\r\n\r\n")); idx >= 0 {
		return raw[:idx], raw[idx+4:]
	}
	if idx := bytes.Index(raw, []byte("\n\n")); idx >= 0 {
		return raw[:idx], raw[idx+2:]
	}
	return raw, nil
}

// headerBlock is a header block terminated for mail.ReadMessage, in a new
// slice. header is usually a prefix of the raw message, and appending to it
// in place wrote over the start of the body -- harmless after CRLF CRLF, but
// after the LF LF of an mbox-stored message it overwrote the first boundary.
func headerBlock(header []byte) []byte {
	out := make([]byte, 0, len(header)+4)
	out = append(out, header...)
	return append(out, "\r\n\r\n"...)
}

// topMediaType parses the message's own Content-Type header.
func topMediaType(raw []byte) (string, map[string]string, error) {
	header, _ := SplitHeaderBody(raw)
	msg, err := mail.ReadMessage(bytes.NewReader(headerBlock(header)))
	if err != nil {
		return "", nil, err
	}
	return mime.ParseMediaType(msg.Header.Get("Content-Type"))
}

// headerValue reads one header out of a header block, case-insensitively.
func headerValue(header []byte, name string) string {
	msg, err := mail.ReadMessage(bytes.NewReader(headerBlock(header)))
	if err != nil {
		return ""
	}
	return msg.Header.Get(name)
}

// rawBodyText decodes a message's body to text without the display-time
// rewriting -- escape stripping, URL un-defanging -- that would alter a PGP
// block and break its signature.
func rawBodyText(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	text, err := extractRawPlainBodyText(msg)
	if err != nil {
		return ""
	}
	return text
}

func stripWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r != '\n' && r != '\r' && r != ' ' && r != '\t' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitRawParts cuts a multipart body into its parts, returning each part's
// bytes verbatim -- headers and body -- so a detached signature can be checked
// against exactly what the sender signed.
//
// The CRLF that precedes a boundary delimiter belongs to the delimiter
// (RFC 2046), so exactly one is removed from the end of each part.
func splitRawParts(body []byte, boundary string) [][]byte {
	if boundary == "" {
		return nil
	}
	delim := "--" + boundary
	closing := delim + "--"
	var parts [][]byte
	partStart := -1
	for pos := 0; pos < len(body); {
		line, next := nextLine(body, pos)
		trimmed := strings.TrimRight(string(line), "\r\n")
		switch trimmed {
		case closing:
			if partStart >= 0 {
				parts = append(parts, trimTrailingCRLF(body[partStart:pos]))
			}
			return parts
		case delim:
			if partStart >= 0 {
				parts = append(parts, trimTrailingCRLF(body[partStart:pos]))
			}
			partStart = next
		}
		pos = next
	}
	if partStart >= 0 {
		parts = append(parts, trimTrailingCRLF(body[partStart:]))
	}
	return parts
}

// nextLine returns the line beginning at pos, terminator included, and the
// offset of the next one. It always advances.
func nextLine(b []byte, pos int) ([]byte, int) {
	if idx := bytes.IndexByte(b[pos:], '\n'); idx >= 0 {
		end := pos + idx + 1
		return b[pos:end], end
	}
	return b[pos:], len(b)
}

func trimTrailingCRLF(b []byte) []byte {
	if len(b) >= 2 && b[len(b)-2] == '\r' && b[len(b)-1] == '\n' {
		return b[:len(b)-2]
	}
	if len(b) >= 1 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}
