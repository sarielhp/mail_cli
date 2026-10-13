package app

import (
	"strings"
	"testing"

	"github.com/fatih/color"
)

// The summary must be one line: printed in pieces through a reporter that ends
// every call with a newline, the counts landed on a line of their own.
func TestTrainingSummaryIsOneLine(t *testing.T) {
	color.NoColor = true
	got := TrainingSummary("ham message(s)", 2, 1, 0)
	want := "Successfully trained Bogofilter on 2 new ham message(s). 1 already trained. 0 ignored."
	if got != want {
		t.Errorf("TrainingSummary() = %q, want %q", got, want)
	}
	if strings.Contains(got, "\n") {
		t.Error("summary contains a line break")
	}
}
