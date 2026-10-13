package email

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// A signed part with two headers whose order and spacing a rebuilt part would
// not reproduce: the point of slicing the raw bytes is that both survive.
var signedPart = []byte("Content-Type: text/plain; charset=utf-8\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"hello =E2=80=94 world\r\n")

func TestSplitPGPSignedPreservesTheSignedBytes(t *testing.T) {
	signature := []byte{0x01, 0x02, 0x03, 0x04, 0xff}
	raw := frameSigned(signedPart, signature)

	envelope := SplitPGP(raw)
	if envelope.Kind != PGPSignedMIME {
		t.Fatalf("kind = %v, want PGPSignedMIME", envelope.Kind)
	}
	if !bytes.Equal(envelope.Data, signedPart) {
		t.Fatalf("signed data = %q, want the part verbatim %q", envelope.Data, signedPart)
	}
	if !bytes.Equal(envelope.Signature, signature) {
		t.Fatalf("signature = %x, want %x", envelope.Signature, signature)
	}
}

func TestSplitPGPEncryptedExtractsCiphertext(t *testing.T) {
	ciphertext := []byte("-----BEGIN PGP MESSAGE-----\r\nabc\r\n-----END PGP MESSAGE-----\r\n")
	raw := frameEncrypted(ciphertext)

	envelope := SplitPGP(raw)
	if envelope.Kind != PGPEncryptedMIME {
		t.Fatalf("kind = %v, want PGPEncryptedMIME", envelope.Kind)
	}
	if !bytes.Equal(envelope.Data, ciphertext) {
		t.Fatalf("ciphertext = %q, want %q", envelope.Data, ciphertext)
	}
}

func TestSplitPGPInline(t *testing.T) {
	signed := "Content-Type: text/plain\r\n\r\n" +
		"before\r\n-----BEGIN PGP SIGNED MESSAGE-----\r\nHash: SHA256\r\n\r\nhi\r\n" +
		"-----BEGIN PGP SIGNATURE-----\r\nabc\r\n-----END PGP SIGNATURE-----\r\nafter\r\n"
	envelope := SplitPGP([]byte(signed))
	if envelope.Kind != PGPSignedInline {
		t.Fatalf("kind = %v, want PGPSignedInline", envelope.Kind)
	}
	if !bytes.Contains(envelope.Data, []byte("BEGIN PGP SIGNED MESSAGE")) {
		t.Fatalf("inline data = %q", envelope.Data)
	}
	if bytes.Contains(envelope.Data, []byte("after")) {
		t.Fatal("the block ran past its end marker")
	}

	encrypted := "Content-Type: text/plain\r\n\r\n" +
		"-----BEGIN PGP MESSAGE-----\r\nxyz\r\n-----END PGP MESSAGE-----\r\n"
	if got := SplitPGP([]byte(encrypted)); got.Kind != PGPEncryptedInline {
		t.Fatalf("kind = %v, want PGPEncryptedInline", got.Kind)
	}
}

func TestSplitPGPIgnoresOtherProtocols(t *testing.T) {
	raw := []byte("Content-Type: multipart/signed; protocol=\"application/x-nonsense\"; boundary=\"B\"\r\n\r\n" +
		"--B\r\nContent-Type: text/plain\r\n\r\nhi\r\n--B--\r\n")
	if got := SplitPGP(raw); got.HasPGP() {
		t.Fatalf("kind = %v, want none for a non-PGP protocol", got.Kind)
	}
}

func TestSplitPGPOrdinaryMessage(t *testing.T) {
	raw := []byte("Content-Type: text/plain\r\n\r\njust a note\r\n")
	if got := SplitPGP(raw); got.HasPGP() {
		t.Fatalf("kind = %v, want none", got.Kind)
	}
}

func TestSplitPGPSignedRejectsExtraParts(t *testing.T) {
	signature := []byte{0x01, 0x02, 0x03, 0x04, 0xff}
	raw := frameSigned(signedPart, signature)
	if got := SplitPGP(raw); !got.HasPGP() {
		t.Fatalf("expected 2-part message to be recognized as PGP")
	}

	threeParts := bytes.Replace(raw, []byte("--SIGBOUND--"), []byte("--SIGBOUND\r\nContent-Type: text/plain\r\n\r\ninjected text\r\n--SIGBOUND--"), 1)
	if got := SplitPGP(threeParts); got.HasPGP() {
		t.Fatalf("kind = %v, want PGPNone for multipart/signed with >2 parts", got.Kind)
	}

	onePart := []byte("Content-Type: multipart/signed; protocol=\"application/pgp-signature\"; boundary=\"SIGBOUND\"\r\n\r\n" +
		"--SIGBOUND\r\nContent-Type: text/plain\r\n\r\nonly one part\r\n--SIGBOUND--\r\n")
	if got := SplitPGP(onePart); got.HasPGP() {
		t.Fatalf("kind = %v, want PGPNone for multipart/signed with 1 part", got.Kind)
	}
}

// The tag and the sentence are what the CLI header line and the TUI pill are
// both built from, so their contract is pinned here rather than in either
// presentation.
func TestCryptoStatusTagAndDescription(t *testing.T) {
	tests := []struct {
		name        string
		status      CryptoStatus
		wantTag     string
		wantInDescr string
	}{
		{
			"verified",
			CryptoStatus{Signed: true, ValidSig: true, Verdict: VerdictValid, Signer: "Alice <a@b>", KeyID: "4a1b2c3d"},
			"VERIFIED",
			"Signed by Alice <a@b> (Key 0x4A1B2C3D)",
		},
		{
			"forged",
			CryptoStatus{Signed: true, Verdict: VerdictInvalid},
			"INVALID",
			"Signature verification failed!",
		},
		{
			"unknown key is not an accusation",
			CryptoStatus{Signed: true, Verdict: VerdictUnknownKey, KeyID: "4a1b2c3d"},
			"UNKNOWN KEY",
			"not in your keyring",
		},
		{
			"decrypted",
			CryptoStatus{Encrypted: true, Decrypted: true},
			"DECRYPTED",
			"decrypted",
		},
		{
			"encrypted and unreadable",
			CryptoStatus{Encrypted: true},
			"ENCRYPTED",
			"Encrypted",
		},
		{
			"signed but unchecked",
			CryptoStatus{Signed: true},
			"SIGNED",
			"not checked",
		},
		{
			"nothing",
			CryptoStatus{},
			"",
			"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.status.Tag(); got != tc.wantTag {
				t.Errorf("Tag() = %q, want %q", got, tc.wantTag)
			}
			description := tc.status.Description()
			if tc.wantInDescr == "" {
				if description != "" {
					t.Errorf("Description() = %q, want empty", description)
				}
				return
			}
			if !strings.Contains(description, tc.wantInDescr) {
				t.Errorf("Description() = %q, want it to contain %q", description, tc.wantInDescr)
			}
		})
	}
}

// frameSigned builds a multipart/signed message around a signed part and a
// detached signature, in the framing RFC 3156 specifies.
func frameSigned(signed, signature []byte) []byte {
	var b bytes.Buffer
	b.WriteString("From: alice@example.com\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/signed; micalg=pgp-sha256; " +
		"protocol=\"application/pgp-signature\"; boundary=\"SIGBOUND\"\r\n\r\n")
	b.WriteString("--SIGBOUND\r\n")
	b.Write(signed)
	b.WriteString("\r\n--SIGBOUND\r\n")
	b.WriteString("Content-Type: application/pgp-signature\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(base64.StdEncoding.EncodeToString(signature))
	b.WriteString("\r\n--SIGBOUND--\r\n")
	return b.Bytes()
}

func frameEncrypted(ciphertext []byte) []byte {
	var b bytes.Buffer
	b.WriteString("From: alice@example.com\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/encrypted; " +
		"protocol=\"application/pgp-encrypted\"; boundary=\"ENCBOUND\"\r\n\r\n")
	b.WriteString("--ENCBOUND\r\n")
	b.WriteString("Content-Type: application/pgp-encrypted\r\n\r\n")
	b.WriteString("Version: 1\r\n")
	b.WriteString("--ENCBOUND\r\n")
	b.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	b.Write(ciphertext)
	b.WriteString("\r\n--ENCBOUND--\r\n")
	return b.Bytes()
}
