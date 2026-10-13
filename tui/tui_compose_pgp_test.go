package tui

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/pgp"
)

// tuiGpgKeyring points the test process at a throwaway keyring holding one
// passphrase-less key for the compose tests' sender, and skips when gpg is not
// installed.
func tuiGpgKeyring(t *testing.T) {
	t.Helper()
	if !pgp.Available() {
		t.Skip("gpg is not installed")
	}
	home := t.TempDir()
	t.Setenv("GNUPGHOME", home)
	cmd := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", "user@personal.com", "default", "default", "never")
	cmd.Env = append(os.Environ(), "GNUPGHOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate test key: %v\n%s", err, out)
	}
}

func TestComposeSessionSignsAndEncryptsTheSentMessage(t *testing.T) {
	tuiGpgKeyring(t)
	m, _, _ := setupTestComposeModel(t, false)
	m.compose = &composeState{
		From:    "user@personal.com",
		To:      "user@personal.com",
		Subject: "s",
		Body:    "the body",
		Sign:    true,
		Encrypt: true,
	}

	raw, err := m.buildComposeRFC822()
	if err != nil {
		t.Fatalf("buildComposeRFC822: %v", err)
	}
	if bytes.Contains(raw, []byte("the body")) {
		t.Fatal("the body went out in the clear")
	}

	result := pgp.Process(context.Background(), raw, pgp.Options{Verify: true, Decrypt: true})
	if result.Status == nil || !result.Status.Decrypted || !result.Status.ValidSig {
		t.Fatalf("status = %+v, want a message that is both signed and encrypted", result.Status)
	}
	if !bytes.Contains(result.Raw, []byte("the body")) {
		t.Fatalf("decrypted = %q", result.Raw)
	}
}

// Bcc addresses are encrypted to as hidden recipients; To and Cc are not.
func TestComposeRecipientsKeepBccApart(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.compose = &composeState{To: "A <a@example.com>", Cc: "c@example.com", Bcc: "Secret <s@example.com>"}
	got := m.composeRecipients()
	if strings.Join(got.Visible, ",") != "a@example.com,c@example.com" {
		t.Errorf("Visible = %q", got.Visible)
	}
	if strings.Join(got.Hidden, ",") != "s@example.com" {
		t.Errorf("Hidden = %q, want the Bcc address alone", got.Hidden)
	}
}

func TestComposeSessionLeavesOrdinaryMailAlone(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.compose = &composeState{From: "user@personal.com", To: "x@example.com", Subject: "s", Body: "plain"}

	raw, err := m.buildComposeRFC822()
	if err != nil {
		t.Fatalf("buildComposeRFC822: %v", err)
	}
	if !bytes.Contains(raw, []byte("Content-Type: text/plain")) {
		t.Fatalf("an unprotected message was rewritten: %q", raw)
	}
	if headerOf(raw, "Subject") != "s" {
		t.Error("the routing headers were lost")
	}
}

// A compose session that is encrypted cannot keep its draft in the clear. The
// draft is encrypted to the sender alone, so it stays resumable here and its
// body is readable by no one else.
func TestAnEncryptedSessionNeverWritesAPlaintextDraft(t *testing.T) {
	tuiGpgKeyring(t)
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		From:    "user@personal.com",
		To:      "someone@elsewhere.com",
		Subject: "s",
		Body:    "the draft body",
		Encrypt: true,
	}

	path, err := m.saveLocalDraft()
	if err != nil {
		t.Fatalf("saveLocalDraft: %v", err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, []byte("the draft body")) {
		t.Fatal("the draft was written to disk in plaintext")
	}

	processed := pgp.Process(context.Background(), onDisk, pgp.Options{Decrypt: true})
	if processed.Status == nil || !processed.Status.Decrypted {
		t.Fatalf("status = %+v, want the draft to be readable again", processed.Status)
	}
	if !bytes.Contains(processed.Raw, []byte("the draft body")) {
		t.Fatalf("resumed = %q", processed.Raw)
	}
	if !bytes.Contains(processed.Raw, []byte("someone@elsewhere.com")) {
		t.Error("the recipient was lost with the encryption")
	}
}

// A signed-only session's draft is not a secret: it stays in the clear so it
// can be read, diffed and resumed without a key.
func TestASignedOnlySessionKeepsItsDraftInTheClear(t *testing.T) {
	tuiGpgKeyring(t)
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{
		From: "user@personal.com", To: "someone@elsewhere.com", Subject: "s", Body: "the draft body", Sign: true,
	}

	path, err := m.saveLocalDraft()
	if err != nil {
		t.Fatalf("saveLocalDraft: %v", err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(onDisk, []byte("the draft body")) {
		t.Fatalf("a signed-only draft is unreadable: %q", onDisk)
	}
	// Nor is it signed: an autosave must not ask gpg-agent for a signature.
	if bytes.Contains(onDisk, []byte("multipart/signed")) || bytes.Contains(onDisk, []byte("PGP SIGNATURE")) {
		t.Fatalf("a signed-only session's draft was signed: %q", onDisk)
	}
}

// Turning encryption on retires the clear-text server copy. Moved to the
// trash it would keep the plaintext on the server; it is deleted outright.
func TestEncryptingASessionDeletesItsClearServerDraft(t *testing.T) {
	tuiGpgKeyring(t)
	m, mock := composeWithTrash(t)
	m.compose.Body = "written before encryption"

	drain(t, m, m.saveAndSyncDraftCmd())
	if len(mock.UploadedEmails) != 1 {
		t.Fatalf("uploads = %d, want the clear copy", len(mock.UploadedEmails))
	}
	_, cmd := m.toggleComposeEncrypt()
	drain(t, m, cmd)

	if len(mock.DeletedMessages) != 1 || string(mock.DeletedMessages[0]) != uploadID(0) {
		t.Fatalf("deleted = %v, want the clear copy %s deleted permanently", mock.DeletedMessages, uploadID(0))
	}
	for _, mv := range mock.MovedEmails {
		if mv.DestLabel == "TRASH" {
			t.Fatalf("the clear copy was moved to the trash: %+v", mv)
		}
	}

	// An encrypted copy replaced by another is trashed as usual.
	m.compose.Body = "edited after encryption"
	drain(t, m, m.saveAndSyncDraftCmd())
	if len(mock.DeletedMessages) != 1 || len(mock.MovedEmails) != 1 {
		t.Fatalf("deleted %v moved %+v; want only the encrypted copy trashed", mock.DeletedMessages, mock.MovedEmails)
	}
}

// Leaving compose uploads the draft form, the same bytes the local draft
// holds -- not the outgoing form, which is encrypted to the recipients rather
// than the sender and is signed.
func TestLeavingAnEncryptedSessionUploadsTheDraftForm(t *testing.T) {
	tuiGpgKeyring(t)
	m, mock := composeWithTrash(t)
	m.compose.Body = "the secret draft"
	m.compose.To = "someone@elsewhere.com"
	m.compose.Encrypt, m.compose.Sign = true, true

	_, cmd := m.saveDraftAndLeave(false)
	drain(t, m, cmd)
	if len(mock.UploadedEmails) != 1 {
		t.Fatalf("uploads = %d, want one", len(mock.UploadedEmails))
	}
	uploaded := mock.UploadedEmails[0].RawBytes
	if bytes.Contains(uploaded, []byte("the secret draft")) {
		t.Fatal("the draft was uploaded in plaintext")
	}
	// Decryptable with the sender's key alone, and not signed.
	processed := pgp.Process(context.Background(), uploaded, pgp.Options{Decrypt: true, Verify: true})
	if processed.Status == nil || !processed.Status.Decrypted || processed.Status.Signed {
		t.Fatalf("status = %+v, want the sender-only, unsigned draft form", processed.Status)
	}
}

func TestPgpTogglesAndBadge(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{From: "user@personal.com", To: "x@example.com", Subject: "s", Body: "b"}

	if m.composePgpBadge() != "" {
		t.Errorf("badge = %q, want nothing while PGP is off", m.composePgpBadge())
	}

	m2, _ := m.handleComposeKey(key("p"))
	m = m2.(*tuiModel)
	if !m.compose.Sign || m.composePgpBadge() != "PGP: sign" {
		t.Errorf("after p: Sign=%v badge=%q", m.compose.Sign, m.composePgpBadge())
	}
	if !strings.Contains(m.saveStatus, "signed") {
		t.Errorf("saveStatus = %q", m.saveStatus)
	}

	m2, _ = m.handleComposeKey(key("p"))
	m = m2.(*tuiModel)
	if m.compose.Sign || m.composePgpBadge() != "" {
		t.Errorf("p did not toggle back: Sign=%v badge=%q", m.compose.Sign, m.composePgpBadge())
	}
}

// Turning encryption on with no key for the sender would write a draft that
// cannot be written and send a message that cannot be sent; it is refused with
// a reason instead.
func TestEnablingEncryptionWithoutAKeyIsRefused(t *testing.T) {
	if pgp.Available() {
		t.Setenv("GNUPGHOME", t.TempDir()) // an empty keyring
	}
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{From: "user@personal.com", To: "x@example.com", Subject: "s", Body: "b"}

	m2, _ := m.handleComposeKey(key("P"))
	m = m2.(*tuiModel)
	if m.compose.Encrypt {
		t.Fatal("encryption was enabled with no key")
	}
	if !strings.Contains(m.saveStatus, "No PGP key") {
		t.Errorf("saveStatus = %q, want it to say why", m.saveStatus)
	}
}

func TestRecipientAddressesTakesTheAddressNotTheName(t *testing.T) {
	got := recipientAddresses("Alice <alice@example.com>, bob@example.com", "Carol <carol@example.com>")
	want := []string{"alice@example.com", "bob@example.com", "carol@example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
