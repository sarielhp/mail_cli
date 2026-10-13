package spam

import (
	"strings"
	"testing"
)

func TestCheckSpamScore_NoBogofilter(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no bogofilter on PATH

	score, isSpam, err := CheckSpamScore([]byte("Subject: Test\n\nHello world"))
	if err == nil {
		t.Fatalf("expected error when bogofilter is missing, got score %f, isSpam %v", score, isSpam)
	}
	if !strings.Contains(err.Error(), "bogofilter") {
		t.Errorf("expected error message to mention bogofilter, got: %v", err)
	}
}

// assertBogofilterMissingError checks that a training call failed because
// bogofilter is not installed on PATH.
func assertBogofilterMissingError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error when bogofilter is missing")
	}
	if !strings.Contains(err.Error(), "bogofilter") {
		t.Errorf("expected error message to mention bogofilter, got: %v", err)
	}
}

func TestTrainHam_NoBogofilter(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	assertBogofilterMissingError(t, TrainHam([]byte("Subject: Test\n\nHello world")))
}

func TestTrainSpam_NoBogofilter(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	assertBogofilterMissingError(t, TrainSpam([]byte("Subject: Test\n\nHello world")))
}
