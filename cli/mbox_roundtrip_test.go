package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

// mbox "From " quoting must be idempotent at any depth: the writer escalates
// every `>*From ` by one, the reader strips exactly one. A body that already
// contained `>From ` used to lose a level per round trip.
func TestMboxFromQuotingSurvivesRoundTrip(t *testing.T) {
	// LF only: the writer normalises CRLF to LF on the way into the mbox, which
	// is the format's own convention and not what this test is about.
	bodies := []string{
		"Subject: t\n\nFrom the desk of someone\n",
		"Subject: t\n\n>From the desk of someone\n",
		"Subject: t\n\n>>>From the desk of someone\n",
		"Subject: t\n\nnot a From line\n",
	}

	for _, body := range bodies {
		t.Run(strings.SplitN(body, "\n\n", 2)[1], func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeToMbox(&buf, []byte(body), "a@b.c", time.Now()); err != nil {
				t.Fatalf("writeToMbox: %v", err)
			}

			msgs, err := readFromMbox(&buf)
			if err != nil {
				t.Fatalf("readFromMbox: %v", err)
			}
			if len(msgs) != 1 {
				t.Fatalf("got %d messages, want 1", len(msgs))
			}

			got := strings.TrimRight(string(msgs[0].Body), "\n")
			want := strings.TrimRight(body, "\n")
			if got != want {
				t.Errorf("round trip changed the body:\n got %q\nwant %q", got, want)
			}
		})
	}
}

// A line longer than the old 10MB scanner cap aborted the read, and
// readFromMbox returned (nil, err) -- so one overlong line discarded every
// message parsed before it.
func TestMboxReadSurvivesAnOverlongLine(t *testing.T) {
	var mbox bytes.Buffer
	for i := range 3 {
		fmt.Fprintf(&mbox, "From a@b.c Mon Jan  2 15:04:05 2006\nSubject: m%d\n\nbody %d\n\n", i, i)
	}
	// Comfortably past the cap that used to abort the whole parse.
	fmt.Fprintf(&mbox, "From a@b.c Mon Jan  2 15:04:05 2006\nSubject: big\n\n%s\n\n",
		strings.Repeat("x", 12*1024*1024))

	msgs, err := readFromMbox(&mbox)
	if err != nil {
		t.Fatalf("readFromMbox: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want all 4 including the one with the long line", len(msgs))
	}
	if !bytes.Contains(msgs[3].Body, []byte(strings.Repeat("x", 1024))) {
		t.Error("the long line was not preserved in the last message")
	}
}
