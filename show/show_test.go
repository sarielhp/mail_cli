package show

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

const plainMessage = "From: Alice Smith <alice@example.com>\r\n" +
	"To: bob@example.com\r\n" +
	"Cc: carol@example.com\r\n" +
	"Subject: quarterly report\r\n" +
	"Date: Tue, 02 Sep 2025 10:04:05 +0000\r\n" +
	"Message-ID: <report@example.com>\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
	"the body of the message\r\n"

// Render is the single renderer both the label path and the by-ID path go
// through. It replaced two near-identical functions that had drifted apart --
// only one of them printed the To header.
func TestRenderPrintsHeadersAndBody(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(Options{}, &buf, &cfg_g.Config{}, "msg123", "Receipts", []byte(plainMessage)); err != nil {
		t.Errorf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"msg123",
		"Receipts",
		"alice@example.com",
		"bob@example.com",
		"quarterly report",
		"Tue, 02 Sep 2025",
		"the body of the message",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output is missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// Showing a message found by ID alone has no folder to report, and the line
// should be left out rather than printed empty.
func TestRenderOmitsTheFolderLineWhenUnknown(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(Options{}, &buf, &cfg_g.Config{}, "msg123", "", []byte(plainMessage)); err != nil {
		t.Errorf("Render: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "Folder:") {
		t.Errorf("output prints an empty folder line:\n%s", out)
	}
	if !strings.Contains(out, "quarterly report") {
		t.Error("output lost the subject")
	}
}

func TestRenderRejectsUnparsableMessage(t *testing.T) {
	err := Render(Options{}, io.Discard, &cfg_g.Config{}, "broken", "", []byte("this is not a message"))
	if err == nil {
		t.Error("Render accepted a message that is not RFC 5322")
	}
}

// Headers are MIME word-encoded in the wild; the reader should see them decoded.
func TestRenderDecodesEncodedHeaders(t *testing.T) {
	raw := "From: =?utf-8?B?SsO2cmcgTcO8bGxlcg==?= <jorg@example.com>\r\n" +
		"To: bob@example.com\r\n" +
		"Subject: =?utf-8?B?cXVhcnRlcmx5IHJlcG9ydA==?=\r\n" +
		"Date: Tue, 02 Sep 2025 10:04:05 +0000\r\n\r\n" +
		"body\r\n"

	var buf bytes.Buffer
	if err := Render(Options{}, &buf, &cfg_g.Config{}, "m1", "", []byte(raw)); err != nil {
		t.Errorf("Render: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "Jörg Müller") {
		t.Errorf("encoded From header was not decoded:\n%s", out)
	}
	if !strings.Contains(out, "quarterly report") {
		t.Errorf("encoded Subject header was not decoded:\n%s", out)
	}
	if strings.Contains(out, "=?utf-8?B?") {
		t.Errorf("raw MIME encoding leaked into the output:\n%s", out)
	}
}

func TestFindTargetEmailID(t *testing.T) {
	ids := []email.ID{"19f3a8216d52dfc6", "1a0ad9973a7a20aa"}

	tests := []struct {
		name   string
		target string
		want   email.ID
	}{
		{name: "exact", target: "19f3a8216d52dfc6", want: "19f3a8216d52dfc6"},
		{name: "case insensitive", target: "19F3A8216D52DFC6", want: "19f3a8216d52dfc6"},
		{name: "surrounding space", target: "  19f3a8216d52dfc6 ", want: "19f3a8216d52dfc6"},
		{name: "no match", target: "ffffffffffffffff", want: ""},
		{name: "empty", target: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := findTargetEmailID(ids, tc.target); got != tc.want {
				t.Errorf("findTargetEmailID(%q) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

// The short ID is what listings display, so it has to be a way in.
func TestFindTargetEmailIDMatchesTheShortForm(t *testing.T) {
	ids := []email.ID{"19f3a8216d52dfc6"}
	short := shortIDOf(t, string(ids[0]))

	if got := findTargetEmailID(ids, short); got != ids[0] {
		t.Errorf("findTargetEmailID(%q) = %q, want the full ID", short, got)
	}
}

func shortIDOf(t *testing.T, id string) string {
	t.Helper()
	// Mirrors what the listings print beside each message.
	return emailComputeShortID(id)
}

func TestExtractHTMLBodyText(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantHTML string
		wantAny  bool
	}{
		{
			name: "html body",
			raw: "From: a@b.c\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
				"<html><body><p>hello</p></body></html>\r\n",
			wantHTML: "<p>hello</p>",
			wantAny:  true,
		},
		{
			name:     "plain body is wrapped so a browser can show it",
			raw:      "From: a@b.c\r\nContent-Type: text/plain\r\n\r\nplain words\r\n",
			wantHTML: "plain words",
			wantAny:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := mail.ReadMessage(strings.NewReader(tc.raw))
			if err != nil {
				t.Fatalf("parsing fixture: %v", err)
			}
			got, err := extractHTMLBodyText(parsed)
			if err != nil {
				t.Fatalf("extractHTMLBodyText: %v", err)
			}
			if tc.wantAny && !strings.Contains(got, tc.wantHTML) {
				t.Errorf("got %q, want it to contain %q", got, tc.wantHTML)
			}
		})
	}
}

// --web falls back to the plain body when a message has no HTML part, rather
// than opening an empty browser window.
func TestRenderFallsBackWhenWebRequestedButNoHTML(t *testing.T) {
	raw := "From: a@b.c\r\nSubject: s\r\n" +
		"Content-Type: multipart/mixed; boundary=bnd\r\n\r\n" +
		"--bnd\r\nContent-Type: text/plain\r\n\r\nonly plain text\r\n" +
		"--bnd--\r\n"

	var buf bytes.Buffer
	if err := Render(Options{Web: true}, &buf, &cfg_g.Config{}, "m1", "", []byte(raw)); err != nil {
		t.Errorf("Render: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "No HTML body found") {
		t.Errorf("no fallback notice for a message with no HTML part:\n%s", out)
	}
	if !strings.Contains(out, "only plain text") {
		t.Errorf("the plain body was not shown as the fallback:\n%s", out)
	}
}

func TestByIDContext_Cancellation(t *testing.T) {
	mock := &mailclient.MockMailClient{
		Cfg: &cfg_g.Config{},
		RawMessages: map[email.ID][]byte{
			"m1": []byte(plainMessage),
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	err := ByIDContext(ctx, Options{}, io.Discard, mock, mock.Cfg, "m1", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ByIDContext error = %v, want context.Canceled", err)
	}
}

type blockingAccessorClient struct {
	mailclient.MockMailClient
}

func (b *blockingAccessorClient) GetRawMessageContext(ctx context.Context, _ email.ID) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestByID_EnforcesTimeout(t *testing.T) {
	client := &blockingAccessorClient{
		MockMailClient: mailclient.MockMailClient{
			Cfg: &cfg_g.Config{},
		},
	}
	// Verify ByID bounds execution and returns context error on cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	err := ByIDContext(ctx, Options{}, io.Discard, client, client.Cfg, "m1", "")
	if err == nil || (!errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled)) {
		t.Fatalf("ByIDContext with deadline error = %v, want deadline/canceled error", err)
	}
}

func TestPrintSecurityLineReportsTheVerdict(t *testing.T) {
	var buf bytes.Buffer
	printSecurityLine(&buf, &email.CryptoStatus{
		Signed:   true,
		ValidSig: true,
		Verdict:  email.VerdictValid,
		Signer:   "Alice <alice@example.com>",
		KeyID:    "4A1B2C3D",
	})
	out := buf.String()
	for _, want := range []string{"Security:", "VERIFIED", "Alice <alice@example.com>", "0x4A1B2C3D"} {
		if !strings.Contains(out, want) {
			t.Errorf("security line %q is missing %q", out, want)
		}
	}
}

func TestPrintSecurityLineSaysNothingAboutOrdinaryMail(t *testing.T) {
	var buf bytes.Buffer
	printSecurityLine(&buf, nil)
	if buf.Len() != 0 {
		t.Fatalf("a message with no PGP produced a security line: %q", buf.String())
	}
}

func TestSecurityLineDistinguishesAForgeryFromAnUnknownKey(t *testing.T) {
	var buf bytes.Buffer
	printSecurityLine(&buf, &email.CryptoStatus{Signed: true, Verdict: email.VerdictInvalid})
	if out := buf.String(); !strings.Contains(out, "INVALID") {
		t.Errorf("invalid signature rendered as %q", out)
	}

	buf.Reset()
	printSecurityLine(&buf, &email.CryptoStatus{Signed: true, Verdict: email.VerdictUnknownKey})
	out := buf.String()
	if !strings.Contains(out, "UNKNOWN KEY") {
		t.Errorf("unknown key rendered as %q", out)
	}
	if strings.Contains(out, "INVALID") {
		t.Error("a signature whose key is missing was reported as invalid")
	}
}

// A prefix matching several folders, one of them empty, printed one object per
// folder: concatenated, that is not JSON. The listings now come back for the
// caller to write as one array.
func TestPerformJSONReturnsListingsAndPrintsNothing(t *testing.T) {
	client := &mailclient.MockMailClient{
		Cfg:           &cfg_g.Config{SelectedAccount: "work"},
		Labels:        []string{"receipts", "receipts/2025"},
		DownloadedIDs: map[string][]email.ID{"receipts": {"a", "b"}},
	}
	var out bytes.Buffer
	listings, err := Perform(Options{JSON: true}, &out, client, client.Cfg, "receipts", "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("listings must not be printed piecemeal, got %q", out.String())
	}
	if err := MarshalListings(&out, listings); err != nil {
		t.Fatal(err)
	}
	var got []FolderListing
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not one JSON document: %v\n%s", err, out.String())
	}
	if len(got) != 2 || got[0].Total != 2 || got[1].Total != 0 || got[1].MessageIDs == nil {
		t.Errorf("unexpected listings %+v", got)
	}
	if got[0].Account != "work" {
		t.Errorf("account = %q, want work", got[0].Account)
	}
}

// With a target, the message is the only document: no empty-folder object
// precedes it.
func TestPerformJSONTargetPrintsOnlyTheMessage(t *testing.T) {
	client := &mailclient.MockMailClient{
		Cfg:           &cfg_g.Config{},
		Labels:        []string{"a-empty", "b-full"},
		DownloadedIDs: map[string][]email.ID{"b-full": {"msg1"}},
		RawMessages:   map[email.ID][]byte{"msg1": []byte(plainMessage)},
	}
	var out bytes.Buffer
	if _, err := Perform(Options{JSON: true}, &out, client, client.Cfg, "", "msg1"); err != nil {
		t.Fatal(err)
	}
	var detail map[string]any
	if err := json.Unmarshal(out.Bytes(), &detail); err != nil {
		t.Fatalf("output is not one JSON object: %v\n%s", err, out.String())
	}
}

func TestMarshalListingsEmptyIsAnArray(t *testing.T) {
	var out bytes.Buffer
	if err := MarshalListings(&out, nil); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("got %q, want []", out.String())
	}
}

// A good signature by a key nobody vouched for must not read as VERIFIED, and
// it must not wear the green that VERIFIED has.
func TestSecurityLineSeparatesAnUnverifiedKeyFromAVerifiedOne(t *testing.T) {
	var buf bytes.Buffer
	printSecurityLine(&buf, &email.CryptoStatus{Signed: true, Verdict: email.VerdictUnverifiedKey, Signer: "Alice <alice@example.com>"})
	out := buf.String()
	if !strings.Contains(out, "UNVERIFIED KEY") {
		t.Fatalf("an unverified key rendered as %q", out)
	}
	if !strings.Contains(out, "Good signature, unverified key") || !strings.Contains(out, "alice@example.com") {
		t.Errorf("security line %q does not explain the verdict", out)
	}
	for _, tag := range []string{"UNVERIFIED KEY", "SENDER MISMATCH"} {
		if securityTagColor(tag) == app.ColorBoldGreen.Sprint(tag) && app.ColorBoldGreen.Sprint(tag) != app.ColorBoldYellow.Sprint(tag) {
			t.Errorf("%s is coloured as verified", tag)
		}
	}
}
