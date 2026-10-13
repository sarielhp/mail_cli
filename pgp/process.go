package pgp

import (
	"bytes"
	"context"
	"strings"

	"github.com/sarielhp/mail_cli/email"
)

// Options is what the caller asked the PGP layer to do. Both default to off so
// a caller has to decide that running gpg on a stranger's message is what it
// wants.
type Options struct {
	Verify  bool
	Decrypt bool
}

// Result is a message after the PGP layer has had it.
//
// Raw is the message to render, and it holds only what the verdict covers:
// the signed or decrypted entity under the message's routing headers. For
// inline PGP that is a single text/plain entity holding the cleartext, never
// the original message: an inline block can sit in one part of a
// multipart/alternative whose HTML sibling, and any attachment, the signature
// says nothing about -- rendered beside a VERIFIED badge, they would borrow
// it. The original bytes are returned only when nothing was verified or
// decrypted. Body is the same inline text, for callers that want it directly.
type Result struct {
	Raw    []byte
	Body   string
	Status *email.CryptoStatus
}

// Process inspects a raw message, verifies or decrypts what it carries, and
// returns what should actually be shown.
//
// A message with no PGP content is returned untouched and with a nil Status, so
// callers do not have to branch before they can render ordinary mail.
func Process(ctx context.Context, raw []byte, opts Options) Result {
	envelope := email.SplitPGP(raw)
	if !envelope.HasPGP() {
		return Result{Raw: raw}
	}

	status := initialStatus(envelope.Kind)
	if !Available() {
		status.Verdict = email.VerdictError
		status.Error = ErrUnavailable.Error()
		return Result{Raw: raw, Status: status}
	}

	p := &processing{ctx: ctx, raw: raw, envelope: envelope, opts: opts, status: status}
	switch envelope.Kind {
	case email.PGPSignedMIME:
		return p.signedMIME()
	case email.PGPSignedInline:
		return p.signedInline()
	default:
		return p.encrypted()
	}
}

// processing is one message on its way through Process.
type processing struct {
	ctx      context.Context
	raw      []byte
	envelope email.PGPEnvelope
	opts     Options
	status   *email.CryptoStatus
}

// initialStatus records what the message is before anything has been checked.
func initialStatus(kind email.PGPKind) *email.CryptoStatus {
	if kind == email.PGPSignedMIME || kind == email.PGPSignedInline {
		return &email.CryptoStatus{Signed: true}
	}
	return &email.CryptoStatus{Encrypted: true}
}

func (p *processing) signedMIME() Result {
	if p.opts.Verify {
		result, err := VerifyDetached(p.ctx, p.envelope.Data, p.envelope.Signature)
		p.applySignature(result, err)
	}
	return Result{Raw: reassemble(routingHeaders(p.raw), p.envelope.Data), Status: p.status}
}

func (p *processing) signedInline() Result {
	if p.opts.Verify {
		result, err := VerifyInline(p.ctx, p.envelope.Data)
		p.applySignature(result, err)
	}
	// The armour is not part of what the sender wrote; showing it would bury
	// the message under a block of dashes.
	text := clearsignedText(string(p.envelope.Data))
	return Result{Raw: plainTextMessage(p.raw, text), Body: text, Status: p.status}
}

func (p *processing) encrypted() Result {
	raw, status := p.raw, p.status
	if !p.opts.Decrypt {
		return Result{Raw: raw, Status: status}
	}
	plaintext, result, err := Decrypt(p.ctx, p.envelope.Data)
	if err != nil {
		status.Verdict = email.VerdictError
		status.Error = err.Error()
		return Result{Raw: raw, Status: status}
	}
	status.Decrypted = result.Decrypted
	p.applySignature(result, nil)
	if looksLikeMessage(plaintext) {
		// The routing headers stay outside the encryption, and they are what
		// carries From, To and Subject; the plaintext carries only the entity.
		// Rejoining them makes the result a whole message again, so a caller
		// can parse it exactly as it would parse any other.
		return Result{Raw: reassemble(routingHeaders(raw), plaintext), Status: status}
	}
	text := string(plaintext)
	return Result{Raw: plainTextMessage(raw, text), Body: text, Status: status}
}

// reassemble puts a message's routing headers in front of an entity, which
// begins with its own headers. One CRLF ends the last routing header; a second
// would end the header block there and turn the entity's Content-Type into
// body text.
func reassemble(headers, entity []byte) []byte {
	out := make([]byte, 0, len(headers)+2+len(entity))
	out = append(out, headers...)
	out = append(out, '\r', '\n')
	return append(out, entity...)
}

// plainTextMessage is the message's routing headers over one text/plain
// entity holding text: the shape an inline signature or inline encryption
// leaves once the armour is gone.
func plainTextMessage(raw []byte, text string) []byte {
	entity := "Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	return reassemble(routingHeaders(raw), []byte(entity))
}

// routingHeaders returns a message's headers without any Content-* header,
// along with the folded continuation lines that belonged to them. The entity
// put after them carries its own; an outer Content-Transfer-Encoding left
// behind would be read in place of the entity's.
func routingHeaders(raw []byte) []byte {
	header, _ := email.SplitHeaderBody(raw)
	lines := strings.Split(strings.ReplaceAll(string(header), "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	dropping := false
	for _, line := range lines {
		if dropping && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			continue
		}
		dropping = strings.HasPrefix(strings.ToLower(line), "content-")
		if !dropping {
			kept = append(kept, line)
		}
	}
	return []byte(strings.Join(kept, "\r\n"))
}

// applySignature folds a status channel into the presentation model. It is used
// for a detached signature and for whatever signature gpg found inside an
// encrypted payload.
func (p *processing) applySignature(result Status, err error) {
	status := p.status
	switch {
	case result.Signed():
	case result.NoData:
		status.Signed, status.Verdict = true, email.VerdictError
		status.Error = "gpg found no signature to check"
		return
	case err != nil:
		status.Verdict, status.Error = email.VerdictError, err.Error()
		return
	default:
		return
	}

	judged := judgeSignatures(result.Signatures)
	status.Signed, status.Verdict, status.Error = true, judged.verdict, judged.message
	status.KeyID, status.Signer = judged.sig.KeyID, judged.sig.UserID
	status.Fingerprint = judged.sig.PrimaryFingerprint
	if status.Fingerprint == "" {
		status.Fingerprint = judged.sig.Fingerprint
	}
	if judged.sig.Outcome == OutcomeGood {
		status.Trust = judged.sig.Trust.String()
	}
	p.checkSender(result.Signatures)
	status.ValidSig = status.Verdict == email.VerdictValid
}

// checkSender demotes a good signature whose key does not carry the From
// address. The signature proves who signed, and a reader looks at From.
func (p *processing) checkSender(sigs []Signature) {
	status := p.status
	if status.Verdict != email.VerdictValid && status.Verdict != email.VerdictUnverifiedKey {
		return
	}
	from := senderAddress(p.raw)
	if signedBy(sigs, from) {
		return
	}
	status.SenderMismatch, status.Verdict = true, email.VerdictSenderMismatch
	if from == "" {
		status.Error = "the message has no From address to match the signer against"
		return
	}
	status.Error = "no address on the signing key is the From address " + from
}

// clearsignedText returns the message inside a cleartext signature: after the
// armour headers and their blank line, before the signature, with RFC 4880
// dash-escaping undone.
func clearsignedText(block string) string {
	lines := strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n")
	start := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "-----BEGIN PGP SIGNED MESSAGE-----") {
			start = i + 1
			break
		}
	}
	body := lines[start:]
	// Skip the armour headers ("Hash: SHA256") and the blank line after them.
	for len(body) > 0 {
		line := body[0]
		body = body[1:]
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	var out []string
	for _, line := range body {
		if strings.HasPrefix(line, "-----BEGIN PGP SIGNATURE-----") {
			break
		}
		out = append(out, strings.TrimPrefix(line, "- "))
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// looksLikeMessage reports whether decrypted bytes are a MIME message rather
// than a bare body. RFC 3156 encrypts a whole entity, but mail in the wild
// sometimes encrypts only the text.
func looksLikeMessage(data []byte) bool {
	for _, separator := range [][]byte{[]byte("\r\n\r\n"), []byte("\n\n")} {
		idx := bytes.Index(data, separator)
		if idx <= 0 {
			continue
		}
		header := data[:idx]
		return bytes.Contains(header, []byte(":")) && !bytes.Contains(header, []byte("-----BEGIN"))
	}
	return false
}
