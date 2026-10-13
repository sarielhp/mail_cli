package scan

import (
	"strings"
	"testing"
)

// TestBogofilterMissingBinaryIsAnError guards the fail-open bug: the run error
// was discarded, so a missing or broken bogofilter produced a confident
// "Ham, score 0.0" and silently disabled spam classification while every scan
// reported success.
func TestBogofilterMissingBinaryIsAnError(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no bogofilter on PATH

	res, err := classifyWithBogofilter([]byte("Subject: test\r\n\r\nbody"))
	if err == nil {
		t.Fatalf("expected an error when bogofilter is unavailable, got result %+v", res)
	}
	if res.IsSpam {
		t.Error("a failed classification must not report spam")
	}
	if res.Score != 0 {
		t.Errorf("expected a zero score on failure, got %v", res.Score)
	}
	if !strings.Contains(err.Error(), "bogofilter") {
		t.Errorf("error should name the tool, got %q", err)
	}
}
