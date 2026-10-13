package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestTerminalReporterMarkers(t *testing.T) {
	var buf bytes.Buffer
	r := NewTerminalReporter(&buf)

	r.Infof("scanning %s", "inbox")
	r.Successf("done")
	r.Printf("  continuation\n")

	out := buf.String()
	for _, want := range []string{"scanning inbox", "done", "  continuation"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got %q", want, out)
		}
	}
	if lines := strings.Count(out, "\n"); lines != 3 {
		t.Errorf("expected one line per call, got %d in %q", lines, out)
	}
}

// A format string ending in a newline must not produce a blank line: callers
// migrated from fmt.Printf bring their trailing newlines with them.
func TestTerminalReporterTrimsTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	NewTerminalReporter(&buf).Infof("already terminated\n")

	if got := buf.String(); strings.HasSuffix(got, "\n\n") {
		t.Errorf("expected a single trailing newline, got %q", got)
	}
}

func TestTerminalReporterProgressEndsTheLineWhenDone(t *testing.T) {
	var buf bytes.Buffer
	r := NewTerminalReporter(&buf)

	r.Progress(1, 2, "working")
	if strings.HasSuffix(buf.String(), "\n") {
		t.Error("expected no line break while the job is in progress")
	}

	r.Progress(2, 2, "working")
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Error("expected a line break once the job is done")
	}
}

func TestQuietReporterKeepsWarningsOnly(t *testing.T) {
	var buf bytes.Buffer
	q := quietReporter{Inner: NewTerminalReporter(&buf)}

	q.Infof("chatter")
	q.Successf("more chatter")
	q.Progress(1, 1, "bar")
	if buf.Len() != 0 {
		t.Errorf("expected quiet output to be empty, got %q", buf.String())
	}

	q.Warnf("something is wrong")
	if !strings.Contains(buf.String(), "something is wrong") {
		t.Errorf("expected warnings to pass through, got %q", buf.String())
	}
}
