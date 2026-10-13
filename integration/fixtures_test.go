package integration

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Fixtures are synthetic messages for tests that need real mail on a real
// server. Every one is obviously artificial -- example.com senders, subjects
// prefixed with a run marker -- so nothing here can be mistaken for a person's
// mail, and so a test can assert on exact subjects and clean up precisely what
// it created.
//
// Several exist because of specific defects: a body line beginning "From "
// catches mbox escaping, a Hebrew subject catches the bidi divergence between
// the listing paths and the TUI, encoded headers catch MIME decoding, and the
// threaded pair catches the References header that the listing path used to
// drop.

type fixture struct {
	slug    string
	subject string
	body    string
	headers map[string]string

	// display is what the subject looks like once a reader has decoded it,
	// where that differs from the wire form. A MIME word-encoded subject goes
	// out as =?utf-8?B?...?= and comes back as text, so a test asserting on
	// what the server returned has to expect the decoded form.
	display string
}

func b64(s string) string {
	return "=?utf-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
}

func fixtureSet() []fixture {
	csv := base64.StdEncoding.EncodeToString([]byte("col1,col2\r\n1,2\r\n3,4\r\n"))

	return []fixture{
		{slug: "plain-01", subject: "Fixture 01 - plain message",
			body: "A plain message with nothing special about it.\r\n"},
		{slug: "plain-02", subject: "Fixture 02 - second plain",
			body: "Another ordinary message, for move and copy assertions.\r\n"},
		{slug: "plain-03", subject: "Fixture 03 - third plain",
			body: "A third, so a batch operation has something to batch.\r\n"},
		{slug: "mbox-trap", subject: "Fixture 04 - body line starting with From",
			body: "This body deliberately contains an mbox trap:\r\n" +
				"From here on the line must be escaped in an archive.\r\n" +
				"If it is not, the archive gains a phantom message.\r\n"},
		{slug: "hebrew", subject: "Fixture 05 - שלום עולם mixed direction",
			body: "A subject mixing Hebrew and Latin, for the bidi rendering paths.\r\n"},
		{slug: "encoded", subject: b64("Fixture 06 - Jörg Müller encoded"),
			display: "Fixture 06 - Jörg Müller encoded",
			body:    "Headers here are MIME word-encoded and must be decoded on display.\r\n",
			headers: map[string]string{"From": b64("Jörg Müller") + " <jorg@example.com>"}},
		{slug: "thread-parent", subject: "Fixture 07 - thread parent",
			body: "The first message in a thread.\r\n"},
		{slug: "thread-reply", subject: "Re: Fixture 07 - thread parent",
			body: "A reply, which should thread under fixture 07.\r\n",
			headers: map[string]string{
				"In-Reply-To": "<fixture-thread-parent@example.com>",
				"References":  "<fixture-thread-parent@example.com>",
			}},
		{slug: "attachment", subject: "Fixture 09 - with attachment",
			headers: map[string]string{"Content-Type": `multipart/mixed; boundary="bnd42"`},
			body: "--bnd42\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" +
				"This message carries a small CSV attachment.\r\n" +
				"--bnd42\r\nContent-Type: text/csv; name=\"data.csv\"\r\n" +
				"Content-Disposition: attachment; filename=\"data.csv\"\r\n" +
				"Content-Transfer-Encoding: base64\r\n\r\n" + csv + "\r\n--bnd42--\r\n"},
		{slug: "spammy", subject: "Fixture 10 - WINNER!! claim your PRIZE now!!!",
			body: "CONGRATULATIONS you have WON. Click here to CLAIM your FREE PRIZE NOW!!!\r\n" +
				"This is synthetic test content, not real spam.\r\n"},
	}
}

// markedSubject tags a fixture so a test can find exactly its own messages and
// a human can see at a glance where they came from.
func (f fixture) markedSubject(runID string) string {
	return fmt.Sprintf("[mail_cli-test %s] %s", runID, f.subject)
}

// expectedSubject is what the subject should read as once the message has been
// fetched back and parsed.
func (f fixture) expectedSubject(runID string) string {
	subject := f.subject
	if f.display != "" {
		subject = f.display
	}
	return fmt.Sprintf("[mail_cli-test %s] %s", runID, subject)
}

// raw renders the fixture as an RFC 5322 message.
func (f fixture) raw(runID string, index int) []byte {
	when := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC).AddDate(0, 0, index)

	headers := map[string]string{
		"From":         fmt.Sprintf("Fixture %s <%s@example.com>", f.slug, f.slug),
		"To":           "mail-cli-test@example.com",
		"Subject":      f.markedSubject(runID),
		"Date":         when.Format(time.RFC1123Z),
		"Message-ID":   fmt.Sprintf("<fixture-%s-%s@example.com>", f.slug, runID),
		"MIME-Version": "1.0",
		"Content-Type": `text/plain; charset="utf-8"`,
	}
	for k, v := range f.headers {
		headers[k] = v
	}

	// A marked subject must survive encoding: an encoded fixture's subject is
	// already a MIME word, so the marker goes in front of it as plain text.
	var b strings.Builder
	for _, k := range []string{"From", "To", "Subject", "Date", "Message-ID", "MIME-Version", "Content-Type", "In-Reply-To", "References"} {
		if v, ok := headers[k]; ok {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	b.WriteString("\r\n")
	b.WriteString(f.body)
	return []byte(b.String())
}

// writeFixtureMbox renders the whole set as an mbox, for seeding a mailbox by
// hand with `mail_cli upload <folder> <file>`.
func writeFixtureMbox(path, runID string) error {
	var b strings.Builder
	for i, f := range fixtureSet() {
		when := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		fmt.Fprintf(&b, "From fixture@example.com %s\n", when.Format("Mon Jan 02 15:04:05 2006"))
		for _, line := range strings.Split(string(f.raw(runID, i)), "\r\n") {
			if strings.HasPrefix(line, "From ") {
				b.WriteString(">")
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0600)
}

// TestWriteFixtureMbox regenerates testdata/fixtures.mbox. It is opt-in because
// it writes into the repository:
//
//	MAIL_CLI_WRITE_FIXTURES=1 go test ./integration/ -run TestWriteFixtureMbox
func TestWriteFixtureMbox(t *testing.T) {
	if os.Getenv("MAIL_CLI_WRITE_FIXTURES") == "" {
		t.Skip("set MAIL_CLI_WRITE_FIXTURES=1 to regenerate testdata/fixtures.mbox")
	}
	const path = "../testdata/fixtures.mbox"
	if err := writeFixtureMbox(path, "manual"); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	t.Logf("wrote %d fixtures to %s", len(fixtureSet()), path)
}

// The fixtures have to parse, or a live test fails for a reason that has
// nothing to do with the server.
func TestFixturesAreWellFormed(t *testing.T) {
	seen := make(map[string]bool)
	for i, f := range fixtureSet() {
		if f.slug == "" || seen[f.slug] {
			t.Errorf("fixture %d has a missing or duplicate slug %q", i, f.slug)
		}
		seen[f.slug] = true

		raw := f.raw("run1", i)
		if !strings.Contains(string(raw), "[mail_cli-test run1]") {
			t.Errorf("fixture %q is not marked with the run ID", f.slug)
		}
		if !strings.Contains(string(raw), "\r\n\r\n") {
			t.Errorf("fixture %q has no header/body separator", f.slug)
		}
	}
	if len(seen) < 5 {
		t.Errorf("only %d fixtures; the set should cover the known edge cases", len(seen))
	}
}
