package tui

import "testing"

func TestProgramReporterSendsMessages(t *testing.T) {
	var got []ReportMsg
	r := NewProgramReporter(func(msg any) {
		got = append(got, msg.(ReportMsg))
	})

	r.Infof("downloading %d", 3)
	r.Progress(2, 5, "Downloading")

	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Level != reportInfo || got[0].Text != "downloading 3" {
		t.Errorf("unexpected info message: %+v", got[0])
	}
	if got[1].Level != reportProgress || got[1].Current != 2 || got[1].Total != 5 {
		t.Errorf("unexpected progress message: %+v", got[1])
	}
}

// A reporter built before the program is running has nowhere to send; it must
// not panic.
func TestProgramReporterWithoutSendIsSafe(_ *testing.T) {
	r := NewProgramReporter(nil)
	r.Infof("x")
	r.Progress(1, 1, "y")
}
