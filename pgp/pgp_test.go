package pgp

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/mail"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/email"
)

const testUser = "Test User <test@example.com>"

// keyring creates a throwaway GNUPGHOME with one passphrase-less key and points
// the test process at it, so nothing here can read or write the developer's own
// keyring. Tests that need gpg skip when it is not installed.
func keyring(t *testing.T) {
	t.Helper()
	if !Available() {
		t.Skip("gpg is not installed")
	}
	home := t.TempDir()
	t.Setenv("GNUPGHOME", home)
	cmd := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", testUser, "default", "default", "never")
	cmd.Env = append(os.Environ(), "GNUPGHOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate test key: %v\n%s", err, out)
	}
}

func clearsign(t *testing.T, text string) []byte {
	t.Helper()
	cmd := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--clearsign", "--output", "-")
	cmd.Stdin = strings.NewReader(text)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("clearsign: %v", err)
	}
	return out
}

func TestVerifyDetachedRoundTrip(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	signed := []byte("Content-Type: text/plain\r\n\r\nhello\r\n")

	signature, err := SignDetached(ctx, signed, "")
	if err != nil {
		t.Fatalf("SignDetached: %v", err)
	}
	status, err := VerifyDetached(ctx, signed, signature)
	if err != nil {
		t.Fatalf("VerifyDetached: %v", err)
	}
	if len(status.Signatures) != 1 || status.Signatures[0].Outcome != OutcomeGood {
		t.Fatalf("status = %+v, want one good signature", status)
	}
	sig := status.Signatures[0]
	if sig.Fingerprint == "" || !sig.validated() {
		t.Error("VALIDSIG fingerprint was not captured")
	}
	if sig.Trust != TrustUltimate {
		t.Errorf("Trust = %v, want ultimate for the keyring's own key", sig.Trust)
	}
	if len(sig.UIDs) != 1 || sig.UIDs[0] != testUser {
		t.Errorf("UIDs = %q, want the key's user id", sig.UIDs)
	}
}

// RFC 3156 signs the part with CRLF line endings. A message stored with bare
// LF -- an mbox -- has to verify, not read as a forgery.
func TestVerifyDetachedAcceptsBareLFStorage(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	signed := []byte("Content-Type: text/plain\r\n\r\nline one\r\nline two\r\n")
	signature, err := SignDetached(ctx, signed, "")
	if err != nil {
		t.Fatal(err)
	}
	stored := bytes.ReplaceAll(multipartSigned(signed, signature), []byte("\r\n"), []byte("\n"))

	result := Process(ctx, stored, Options{Verify: true})
	if result.Status == nil || result.Status.Verdict != email.VerdictValid {
		t.Fatalf("status = %+v, want an LF-stored message to verify", result.Status)
	}
}

func TestVerifyDetachedDetectsTampering(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	signature, err := SignDetached(ctx, []byte("hello\r\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	status, err := VerifyDetached(ctx, []byte("hello there\r\n"), signature)
	if err != nil {
		t.Fatalf("VerifyDetached: %v", err)
	}
	if len(status.Signatures) != 1 || status.Signatures[0].Outcome != OutcomeBad {
		t.Fatalf("status = %+v, want BADSIG", status)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	plaintext := []byte("the secret plans\r\n")

	ciphertext, err := Encrypt(ctx, plaintext, []string{testUser})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, status, err := Decrypt(ctx, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !status.Decrypted {
		t.Fatalf("status = %+v, want DECRYPTION_OKAY", status)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("plaintext = %q, want %q", got, plaintext)
	}
}

func TestDecryptFailsOnTamperedCiphertext(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	plaintext := []byte("secret information\r\n")

	ciphertext, err := Encrypt(ctx, plaintext, []string{testUser})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	tampered := bytes.Replace(ciphertext, []byte("w"), []byte("z"), 1)
	if bytes.Equal(tampered, ciphertext) {
		tampered = append([]byte("junk"), ciphertext...)
	}

	_, status, err := Decrypt(ctx, tampered)
	if err == nil {
		t.Fatal("Decrypt succeeded on tampered ciphertext, want error")
	}
	if status.Decrypted {
		t.Errorf("status.Decrypted = true on tampered ciphertext")
	}
}

func TestProcessMIMEEncryptedTamperedReportsError(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	plaintext := "Content-Type: text/plain\r\n\r\nthe secret\r\n"
	ciphertext, err := Encrypt(ctx, []byte(plaintext), []string{testUser})
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(ciphertext, []byte("A"), []byte("B"), 2)
	raw := multipartEncrypted(tampered)

	result := Process(ctx, raw, Options{Decrypt: true})
	if result.Status == nil {
		t.Fatal("expected non-nil Status")
	}
	if result.Status.Decrypted {
		t.Errorf("Status.Decrypted = true on tampered ciphertext")
	}
	if result.Status.Verdict != email.VerdictError {
		t.Errorf("Verdict = %q, want %q", result.Status.Verdict, email.VerdictError)
	}
}

// A bare literal packet is not encryption: gpg "decrypts" it with exit 0 and
// no DECRYPTION_OKAY. It must be an error, not plaintext shown as encrypted mail.
func TestDecryptRefusesALiteralPacketWithoutDecryptionOkay(t *testing.T) {
	keyring(t)
	cmd := exec.Command("gpg", "--batch", "--armor", "--store", "--output", "-")
	cmd.Stdin = strings.NewReader("Content-Type: text/plain\r\n\r\nnot encrypted at all\r\n")
	literal, err := cmd.Output()
	if err != nil {
		t.Fatalf("gpg --store: %v", err)
	}

	if _, _, err := Decrypt(context.Background(), literal); err == nil {
		t.Fatal("Decrypt accepted a literal packet that was never encrypted")
	}
	result := Process(context.Background(), multipartEncrypted(literal), Options{Decrypt: true})
	if result.Status == nil || result.Status.Decrypted || result.Status.Verdict != email.VerdictError {
		t.Fatalf("status = %+v, want an error and nothing decrypted", result.Status)
	}
	if bytes.Contains(result.Raw, []byte("not encrypted at all")) && !bytes.Contains(result.Raw, []byte("BEGIN PGP")) {
		t.Fatal("the literal packet's contents were rendered as the message")
	}
}

func TestProcessMIMESigned(t *testing.T) {
	keyring(t)
	signed := []byte("Content-Type: text/plain\r\n\r\nsigned body\r\n")
	signature, err := SignDetached(context.Background(), signed, "")
	if err != nil {
		t.Fatal(err)
	}
	raw := multipartSigned(signed, signature)

	result := Process(context.Background(), raw, Options{Verify: true})
	if result.Status == nil || !result.Status.Signed {
		t.Fatalf("status = %+v, want a signed verdict", result.Status)
	}
	if !result.Status.ValidSig || result.Status.Verdict != email.VerdictValid {
		t.Fatalf("status = %+v, want a valid signature", result.Status)
	}
	if result.Status.Signer == "" {
		t.Error("signer was not captured")
	}
}

func TestProcessMIMESignedRejectsExtraInjectedParts(t *testing.T) {
	keyring(t)
	signed := []byte("Content-Type: text/plain\r\n\r\nsigned body\r\n")
	signature, err := SignDetached(context.Background(), signed, "")
	if err != nil {
		t.Fatal(err)
	}
	raw := multipartSigned(signed, signature)
	tampered := bytes.Replace(raw, []byte("--SIGBOUND--"), []byte("--SIGBOUND\r\nContent-Type: text/plain\r\n\r\nattacker extra content\r\n--SIGBOUND--"), 1)

	result := Process(context.Background(), tampered, Options{Verify: true})
	if result.Status != nil && result.Status.Signed {
		t.Fatalf("expected message with injected parts not to be treated as signed PGP, got: %+v", result.Status)
	}
}

func TestProcessMIMESignedWithoutVerification(t *testing.T) {
	keyring(t)
	signed := []byte("Content-Type: text/plain\r\n\r\nsigned body\r\n")
	signature, _ := SignDetached(context.Background(), signed, "")

	result := Process(context.Background(), multipartSigned(signed, signature), Options{})
	if result.Status == nil || !result.Status.Signed {
		t.Fatalf("status = %+v, want the message recognised as signed", result.Status)
	}
	if result.Status.Verdict != "" {
		t.Fatalf("verdict = %q, want none: nothing was checked", result.Status.Verdict)
	}
}

func TestProcessMIMEEncrypted(t *testing.T) {
	keyring(t)
	plaintext := "Content-Type: text/plain\r\n\r\nthe secret\r\n"
	ciphertext, err := Encrypt(context.Background(), []byte(plaintext), []string{testUser})
	if err != nil {
		t.Fatal(err)
	}
	raw := multipartEncrypted(ciphertext)

	result := Process(context.Background(), raw, Options{Decrypt: true})
	if result.Status == nil || !result.Status.Decrypted {
		t.Fatalf("status = %+v, want decrypted", result.Status)
	}
	if !bytes.Contains(result.Raw, []byte("the secret")) {
		t.Fatalf("decrypted raw = %q, want the plaintext message", result.Raw)
	}
}

func TestProcessInlineSigned(t *testing.T) {
	keyring(t)
	block := clearsign(t, "hello world\r\n")
	raw := append([]byte("From: test@example.com\r\nContent-Type: text/plain\r\n\r\n"), block...)

	result := Process(context.Background(), raw, Options{Verify: true})
	if result.Status == nil || !result.Status.ValidSig {
		t.Fatalf("status = %+v, want a valid inline signature", result.Status)
	}
	if !strings.Contains(result.Body, "hello world") {
		t.Fatalf("body = %q, want the cleartext without the armour", result.Body)
	}
	if strings.Contains(result.Body, "BEGIN PGP") {
		t.Fatalf("body still carries the armour: %q", result.Body)
	}
}

func TestProcessInlineEncrypted(t *testing.T) {
	keyring(t)
	ciphertext, err := Encrypt(context.Background(), []byte("bare inline secret\n"), []string{testUser})
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte("Content-Type: text/plain\r\n\r\n"), ciphertext...)

	result := Process(context.Background(), raw, Options{Decrypt: true})
	if result.Status == nil || !result.Status.Decrypted {
		t.Fatalf("status = %+v, want decrypted", result.Status)
	}
	if !strings.Contains(result.Body, "bare inline secret") {
		t.Fatalf("body = %q, want the decrypted text", result.Body)
	}
}

func TestProcessLeavesOrdinaryMailAlone(t *testing.T) {
	raw := []byte("Content-Type: text/plain\r\n\r\nnothing special\r\n")
	result := Process(context.Background(), raw, Options{Verify: true, Decrypt: true})
	if result.Status != nil {
		t.Fatalf("status = %+v, want nil for a message with no PGP", result.Status)
	}
	if !bytes.Equal(result.Raw, raw) {
		t.Fatal("an ordinary message was rewritten")
	}
}

func TestClearsignedTextUndoesDashEscaping(t *testing.T) {
	block := "-----BEGIN PGP SIGNED MESSAGE-----\r\nHash: SHA256\r\n\r\n" +
		"- - - a dashed line\r\nplain\r\n-----BEGIN PGP SIGNATURE-----\r\nwhatever\r\n"
	if got := clearsignedText(block); got != "- - a dashed line\nplain" {
		t.Fatalf("clearsignedText = %q", got)
	}
}

func TestProtectSignsAMessageThatVerifies(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	msg := email.BuiltMessage{
		Headers: []byte("From: test@example.com\r\nSubject: hi\r\nMIME-Version: 1.0\r\n"),
		Entity:  []byte("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\nhello\r\n"),
	}

	raw, err := Protect(ctx, msg, OutboundOptions{Sign: true})
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	// The message this program produces is one this program can verify. The
	// loop is the test: a signature that only gpg agrees with, or that only
	// our own verifier agrees with, is not a signature.
	result := Process(ctx, raw, Options{Verify: true})
	if result.Status == nil || !result.Status.ValidSig {
		t.Fatalf("status = %+v, want the message we just signed to verify", result.Status)
	}
	if !bytes.Contains(raw, []byte("Subject: hi")) {
		t.Error("the routing headers were lost")
	}
	if !bytes.Contains(raw, []byte("hello")) {
		t.Error("the body was lost")
	}
}

func TestProtectEncryptsAMessageThatDecrypts(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	entity := []byte("Content-Type: text/plain; charset=utf-8\r\n\r\nsecret\r\n")
	msg := email.BuiltMessage{Headers: []byte("From: test@example.com\r\nSubject: hi\r\n"), Entity: entity}

	raw, err := Protect(ctx, msg, OutboundOptions{Encrypt: true, Recipients: []string{testUser}})
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	if bytes.Contains(raw, []byte("secret")) {
		t.Fatal("the plaintext is still in the message")
	}
	result := Process(ctx, raw, Options{Decrypt: true})
	if result.Status == nil || !result.Status.Decrypted {
		t.Fatalf("status = %+v, want decrypted", result.Status)
	}
	if !bytes.Contains(result.Raw, []byte("secret")) {
		t.Fatalf("decrypted = %q, want the entity back", result.Raw)
	}
	if !bytes.Contains(result.Raw, []byte("Content-Type: text/plain")) {
		t.Error("the entity headers were left outside the encryption")
	}
	if !bytes.Contains(raw, []byte("Subject: hi")) {
		t.Error("the routing headers were lost")
	}
}

func TestProtectSignsAndEncryptsInOnePayload(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	entity := []byte("Content-Type: text/plain; charset=utf-8\r\n\r\nboth\r\n")
	msg := email.BuiltMessage{Headers: []byte("From: test@example.com\r\n"), Entity: entity}

	raw, err := Protect(ctx, msg, OutboundOptions{Sign: true, Encrypt: true, Recipients: []string{testUser}})
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	result := Process(ctx, raw, Options{Verify: true, Decrypt: true})
	if result.Status == nil || !result.Status.Decrypted || !result.Status.ValidSig {
		t.Fatalf("status = %+v, want decrypted and verified in one step", result.Status)
	}
	if !bytes.Contains(result.Raw, []byte("both")) {
		t.Fatalf("decrypted = %q", result.Raw)
	}
}

func TestProtectWithoutOptionsIsAPassThrough(t *testing.T) {
	msg := email.BuiltMessage{Headers: []byte("From: a@b\r\n"), Entity: []byte("body\r\n")}
	raw, err := Protect(context.Background(), msg, OutboundOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, msg.Bytes()) {
		t.Fatalf("raw = %q, want the message untouched", raw)
	}
}

func TestEncryptableRecipientsNamesTheOneWithoutAKey(t *testing.T) {
	keyring(t)
	_, err := EncryptableRecipients(context.Background(), []string{testUser, "nobody@example.com"})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "nobody@example.com") {
		t.Fatalf("err = %v, want it to name the recipient with no key", err)
	}
}

// A Bcc recipient's key ID must not be in the ciphertext every recipient
// receives, yet the Bcc recipient must still be able to read it.
func TestProtectHidesTheKeysOfBccRecipients(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	gen := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", "Hidden <hidden@example.com>", "default", "default", "never")
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate hidden key: %v\n%s", err, out)
	}
	list := exec.Command("gpg", "--batch", "--with-colons", "--list-keys", "<hidden@example.com>")
	out, err := list.Output()
	if err != nil {
		t.Fatal(err)
	}
	var hiddenSubkey string
	for _, line := range strings.Split(string(out), "\n") {
		if fields := strings.Split(line, ":"); fields[0] == "sub" && len(fields) > 4 {
			hiddenSubkey = fields[4]
		}
	}

	msg := email.BuiltMessage{Headers: []byte("From: test@example.com\r\n"), Entity: []byte("Content-Type: text/plain\r\n\r\nsecret\r\n")}
	raw, err := Protect(ctx, msg, OutboundOptions{Encrypt: true,
		Recipients: []string{"test@example.com"}, HiddenRecipients: []string{"hidden@example.com"}})
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	packets := exec.Command("gpg", "--batch", "--list-packets", "--list-only")
	packets.Stdin = bytes.NewReader(email.SplitPGP(raw).Data)
	dump, _ := packets.CombinedOutput()
	if hiddenSubkey == "" || strings.Contains(strings.ToUpper(string(dump)), strings.ToUpper(hiddenSubkey)) {
		t.Fatalf("the Bcc recipient's key %q is named in the ciphertext:\n%s", hiddenSubkey, dump)
	}
	if !strings.Contains(string(dump), "0000000000000000") {
		t.Fatalf("no anonymous recipient in the ciphertext:\n%s", dump)
	}
}

// gpg matches a bare address as a substring; the exact form must be used, or
// "est@example.com" finds the key of test@example.com.
func TestRecipientsMatchExactly(t *testing.T) {
	keyring(t)
	if HasPublicKey(context.Background(), "est@example.com") {
		t.Fatal("a substring of an address matched a key")
	}
	if !HasPublicKey(context.Background(), "Test <test@example.com>") {
		t.Fatal("the exact address did not match its key")
	}
	if got := recipientSpec("0xDEADBEEF"); got != "0xDEADBEEF" {
		t.Errorf("recipientSpec(key id) = %q, want it untouched", got)
	}
}

func TestEncryptableRecipientsKeepsTheOnesWithKeys(t *testing.T) {
	keyring(t)
	got, err := EncryptableRecipients(context.Background(), []string{testUser, testUser})
	if err != nil {
		t.Fatalf("EncryptableRecipients: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v, want the duplicate collapsed", got)
	}
}
func TestParseStatus(t *testing.T) {
	text := strings.Join([]string{
		"[GNUPG:] NEWSIG",
		"[GNUPG:] GOODSIG 0123456789ABCDEF Alice Example <alice@example.com>",
		"[GNUPG:] VALIDSIG ABCDEF0123456789ABCDEF0123456789ABCDEF01 2026-10-12 1700000000 0 4 0 1 8 00 ABCDEF0123456789ABCDEF0123456789ABCDEF01",
		"gpg: some prose that is not a status line",
	}, "\n")
	status := parseStatus(text)
	if len(status.Signatures) != 1 || status.Signatures[0].Outcome != OutcomeGood {
		t.Fatalf("status = %+v, want a good signature", status)
	}
	sig := status.Signatures[0]
	if sig.KeyID != "0123456789ABCDEF" {
		t.Errorf("KeyID = %q", sig.KeyID)
	}
	if sig.UserID != "Alice Example <alice@example.com>" {
		t.Errorf("UserID = %q", sig.UserID)
	}
	if sig.Fingerprint != "ABCDEF0123456789ABCDEF0123456789ABCDEF01" {
		t.Errorf("Fingerprint = %q", sig.Fingerprint)
	}
	if sig.validated() {
		t.Error("a VALIDSIG for another key validated the GOODSIG")
	}
}

func TestParseStatusUnknownKeyIsNotAnInvalidSignature(t *testing.T) {
	status := parseStatus("[GNUPG:] ERRSIG 0123456789ABCDEF 1 8 00 1700000000 9 -\n[GNUPG:] NODATA 1\n")
	if len(status.Signatures) != 1 || status.Signatures[0].Outcome != OutcomeUnknownKey {
		t.Fatalf("status = %+v, want one ERRSIG and neither good nor bad", status)
	}
	if !status.NoData {
		t.Fatalf("status = %+v, want NODATA recorded", status)
	}
}

// multipartSigned frames a signed part and its detached signature the way
// RFC 3156 does: the signed bytes verbatim, then the base64 signature.
func multipartSigned(signed, signature []byte) []byte {
	var b bytes.Buffer
	b.WriteString("From: test@example.com\r\n")
	b.WriteString("Subject: signed\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/signed; micalg=pgp-sha256; " +
		"protocol=\"application/pgp-signature\"; boundary=\"SIGBOUND\"\r\n\r\n")
	b.WriteString("--SIGBOUND\r\n")
	b.Write(signed)
	b.WriteString("\r\n--SIGBOUND\r\n")
	b.WriteString("Content-Type: application/pgp-signature; name=\"signature.asc\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(base64.StdEncoding.EncodeToString(signature))
	b.WriteString("\r\n--SIGBOUND--\r\n")
	return b.Bytes()
}

// multipartEncrypted frames a control part and the ciphertext, as RFC 3156
// does.
func multipartEncrypted(ciphertext []byte) []byte {
	var b bytes.Buffer
	b.WriteString("From: test@example.com\r\n")
	b.WriteString("Subject: encrypted\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/encrypted; " +
		"protocol=\"application/pgp-encrypted\"; boundary=\"ENCBOUND\"\r\n\r\n")
	b.WriteString("--ENCBOUND\r\n")
	b.WriteString("Content-Type: application/pgp-encrypted\r\n\r\n")
	b.WriteString("Version: 1\r\n")
	b.WriteString("--ENCBOUND\r\n")
	b.WriteString("Content-Type: application/octet-stream; name=\"msg.asc\"\r\n\r\n")
	b.Write(ciphertext)
	b.WriteString("\r\n--ENCBOUND--\r\n")
	return b.Bytes()
}

// The routing headers sit outside the encryption, so a decrypted message has to
// come back whole: without From, To and Subject the result is an entity no
// caller can parse as mail.
func TestProcessKeepsTheRoutingHeadersOfAnEncryptedMessage(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	msg := email.BuiltMessage{
		Headers: []byte("From: Alice <alice@example.com>\r\nTo: bob@example.com\r\nSubject: hi\r\n"),
		Entity:  []byte("Content-Type: text/plain; charset=utf-8\r\n\r\nsecret\r\n"),
	}
	raw, err := Protect(ctx, msg, OutboundOptions{Encrypt: true, Recipients: []string{testUser}})
	if err != nil {
		t.Fatal(err)
	}

	result := Process(ctx, raw, Options{Decrypt: true})
	parsed, err := mail.ReadMessage(bytes.NewReader(result.Raw))
	if err != nil {
		t.Fatalf("the decrypted message does not parse: %v", err)
	}
	if got := parsed.Header.Get("Subject"); got != "hi" {
		t.Errorf("Subject = %q, want it carried through the decryption", got)
	}
	if got := parsed.Header.Get("From"); got != "Alice <alice@example.com>" {
		t.Errorf("From = %q", got)
	}
	// The entity's own Content-Type is the message's: an extra blank line
	// between the routing headers and the entity used to push it into the body.
	if got := parsed.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want the decrypted entity's", got)
	}
	body, err := io.ReadAll(parsed.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "secret\r\n" {
		t.Fatalf("body = %q, want only the plaintext", body)
	}
}

// An inline signature covers only its own block. A clearsigned text/plain part
// replayed inside a multipart/alternative must not lend its verdict to the HTML
// sibling or to an attachment, so what is returned holds the cleartext alone.
func TestProcessInlineSignedReturnsOnlyTheSignedText(t *testing.T) {
	keyring(t)
	block := clearsign(t, "the signed words\r\n")
	raw := []byte("From: test@example.com\r\nSubject: hi\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"MIX\"\r\n\r\n" +
		"--MIX\r\nContent-Type: multipart/alternative; boundary=\"ALT\"\r\n\r\n" +
		"--ALT\r\nContent-Type: text/plain\r\n\r\n" + string(block) + "\r\n" +
		"--ALT\r\nContent-Type: text/html\r\n\r\n<p>wire the money</p>\r\n--ALT--\r\n" +
		"--MIX\r\nContent-Type: application/octet-stream; name=\"payload.exe\"\r\n" +
		"Content-Disposition: attachment; filename=\"payload.exe\"\r\n\r\nMZ\r\n--MIX--\r\n")

	result := Process(context.Background(), raw, Options{Verify: true})
	if result.Status == nil || result.Status.Verdict != email.VerdictValid {
		t.Fatalf("status = %+v, want the inline signature verified", result.Status)
	}
	if bytes.Contains(result.Raw, []byte("wire the money")) || bytes.Contains(result.Raw, []byte("payload.exe")) {
		t.Fatalf("raw = %q, want nothing the signature does not cover", result.Raw)
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(result.Raw))
	if err != nil {
		t.Fatalf("the result does not parse: %v", err)
	}
	if got := parsed.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("Content-Type = %q, want a single text/plain entity", got)
	}
	if got := parsed.Header.Get("Subject"); got != "hi" {
		t.Errorf("Subject = %q, want the routing headers kept", got)
	}
	body, _ := io.ReadAll(parsed.Body)
	if !strings.Contains(string(body), "the signed words") || strings.Contains(string(body), "BEGIN PGP") {
		t.Errorf("body = %q, want the cleartext without its armour", body)
	}
	if atts, _ := email.ExtractAttachments(result.Raw); len(atts) != 0 {
		t.Errorf("attachments = %d, want none beside a verified inline signature", len(atts))
	}
}

// A draft that is encrypted to its own sender is the only way an encrypted
// compose session can keep a draft without writing plaintext to disk, so
// resuming one has to give the compose fields back.
func TestAnEncryptedDraftResumesWithItsComposeFields(t *testing.T) {
	keyring(t)
	ctx := context.Background()
	comp := &email.ComposeMessage{
		From:    "Alice <alice@example.com>",
		To:      "bob@example.com",
		Subject: "the draft",
		Body:    "the draft text\n",
	}
	built, err := comp.Build()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Protect(ctx, built, OutboundOptions{Encrypt: true, Recipients: []string{testUser}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("the draft text")) {
		t.Fatal("the draft was stored in plaintext")
	}

	resumed, err := email.ParseRFC822ToCompose(Process(ctx, raw, Options{Decrypt: true}).Raw)
	if err != nil {
		t.Fatalf("ParseRFC822ToCompose: %v", err)
	}
	if resumed.Subject != "the draft" {
		t.Errorf("Subject = %q", resumed.Subject)
	}
	if !strings.Contains(resumed.Body, "the draft text") {
		t.Errorf("Body = %q", resumed.Body)
	}
	if resumed.To != "bob@example.com" {
		t.Errorf("To = %q", resumed.To)
	}
}

func TestLimitedWriter_CapsOutput(t *testing.T) {
	var buf bytes.Buffer
	w := &limitedWriter{buf: &buf, max: 10}

	n, err := w.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("first write: n=%d, err=%v", n, err)
	}

	// Past the cap the writer keeps accepting and discarding: an error here
	// would stop the copy draining gpg's pipe and leave gpg blocked on it.
	n, err = w.Write([]byte("world! overflow"))
	if err != nil || n != len("world! overflow") {
		t.Fatalf("write past the cap: n=%d, err=%v; want it drained", n, err)
	}
	if !w.exceeded {
		t.Error("the overflow was not recorded")
	}
	if _, err = w.Write([]byte("more")); err != nil {
		t.Fatalf("a later write failed: %v", err)
	}
	if buf.Len() != 10 {
		t.Errorf("buf.Len() = %d, want 10", buf.Len())
	}
	if buf.String() != "helloworld" {
		t.Errorf("buf = %q, want %q", buf.String(), "helloworld")
	}
}
