package mailclient

import (
	"errors"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"slices"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// MockMailClient carried a single Err field, which made every method fail at
// once. Tests that need one operation to fail while the rest succeed, or a
// batch where some messages are rejected and others are not, were impossible
// to write. These tests cover the two mechanisms that replaced it.

var errBackend = errors.New("backend refused")

func TestMockFailOnTargetsOneMethod(t *testing.T) {
	m := &MockMailClient{
		Cfg:    &cfg_g.Config{},
		Labels: []string{"INBOX", "Receipts"},
		FailOn: map[string]error{"DeleteLabel": errBackend},
	}

	if err := m.DeleteLabel(label.FromServer("Receipts")); !errors.Is(err, errBackend) {
		t.Errorf("DeleteLabel error = %v, want %v", err, errBackend)
	}
	// Everything else must still work, or a test cannot tell which call failed.
	labels, err := m.GetMatchingLabels("")
	if err != nil {
		t.Fatalf("GetMatchingLabels after an unrelated failure: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("GetMatchingLabels returned %d labels, want 2", len(labels))
	}
	if err := m.MoveEmail([]email.ID{"a"}, label.FromServer("INBOX"), label.FromServer("Receipts")); err != nil {
		t.Errorf("MoveEmail after an unrelated failure: %v", err)
	}
}

func TestMockFailOnOverridesBlanketErr(t *testing.T) {
	specific := errors.New("just this one")
	m := &MockMailClient{
		Err:    errBackend,
		FailOn: map[string]error{"ListLabels": specific},
	}

	if err := m.ListLabels(); !errors.Is(err, specific) {
		t.Errorf("ListLabels error = %v, want the FailOn entry %v", err, specific)
	}
	if err := m.FixLabels(); !errors.Is(err, errBackend) {
		t.Errorf("FixLabels error = %v, want the blanket Err %v", err, errBackend)
	}
}

// A FailOn entry set to nil makes exactly one method succeed while Err fails
// the rest, which is how a test isolates the one call it wants to reach.
func TestMockFailOnNilExemptsAMethod(t *testing.T) {
	m := &MockMailClient{
		Err:    errBackend,
		Labels: []string{"INBOX"},
		FailOn: map[string]error{"GetMatchingLabels": nil},
	}

	labels, err := m.GetMatchingLabels("")
	if err != nil {
		t.Fatalf("GetMatchingLabels = %v, want nil", err)
	}
	if len(labels) != 1 {
		t.Errorf("returned %d labels, want 1", len(labels))
	}
	if err := m.ListLabels(); !errors.Is(err, errBackend) {
		t.Errorf("ListLabels = %v, want %v", err, errBackend)
	}
}

func TestMockMoveEmailRecordsOnlyAcceptedMessages(t *testing.T) {
	m := &MockMailClient{
		Cfg:           &cfg_g.Config{},
		FailOnMessage: map[email.ID]error{"bad1": errBackend, "bad2": errBackend},
	}

	err := m.MoveEmail([]email.ID{"ok1", "bad1", "ok2", "bad2", "ok3"}, label.FromServer("INBOX"), label.FromServer("Archive"))
	if !errors.Is(err, errBackend) {
		t.Fatalf("MoveEmail error = %v, want %v", err, errBackend)
	}
	if len(m.MovedEmails) != 1 {
		t.Fatalf("recorded %d move records, want 1", len(m.MovedEmails))
	}
	got := m.MovedEmails[0].MessageIDs
	want := []email.ID{"ok1", "ok2", "ok3"}
	if !slices.Equal(got, want) {
		t.Errorf("moved %v, want %v — rejected messages must not be recorded as moved", got, want)
	}
}

func TestMockCopyEmailRecordsOnlyAcceptedMessages(t *testing.T) {
	m := &MockMailClient{
		Cfg:           &cfg_g.Config{},
		FailOnMessage: map[email.ID]error{"bad": errBackend},
	}

	if err := m.CopyEmail([]email.ID{"ok1", "bad", "ok2"}, label.FromServer("INBOX"), label.FromServer("Archive")); !errors.Is(err, errBackend) {
		t.Fatalf("CopyEmail error = %v, want %v", err, errBackend)
	}
	if len(m.CopiedEmails) != 1 {
		t.Fatalf("recorded %d copy records, want 1", len(m.CopiedEmails))
	}
	if want := []email.ID{"ok1", "ok2"}; !slices.Equal(m.CopiedEmails[0].MessageIDs, want) {
		t.Errorf("copied %v, want %v", m.CopiedEmails[0].MessageIDs, want)
	}
	// The destination folder must only gain the messages that were accepted.
	if want := []email.ID{"ok1", "ok2"}; !slices.Equal(m.DownloadedIDs["Archive"], want) {
		t.Errorf("destination holds %v, want %v", m.DownloadedIDs["Archive"], want)
	}
}

func TestMockRecordsNothingWhenEveryMessageIsRejected(t *testing.T) {
	m := &MockMailClient{
		Cfg:           &cfg_g.Config{},
		FailOnMessage: map[email.ID]error{"a": errBackend, "b": errBackend},
	}

	if err := m.MoveEmail([]email.ID{"a", "b"}, label.FromServer("INBOX"), label.FromServer("Archive")); !errors.Is(err, errBackend) {
		t.Fatalf("MoveEmail error = %v, want %v", err, errBackend)
	}
	if len(m.MovedEmails) != 0 {
		t.Errorf("recorded %d move records, want 0", len(m.MovedEmails))
	}
}

func TestMockCheckAndApplyRulesDropsRejectedMessages(t *testing.T) {
	m := &MockMailClient{
		Cfg:           &cfg_g.Config{},
		FailOnMessage: map[email.ID]error{"bad": errBackend},
	}

	remaining, err := m.CheckAndApplyRules([]email.ID{"ok1", "bad", "ok2"}, label.FromServer("INBOX"), "")
	if !errors.Is(err, errBackend) {
		t.Fatalf("CheckAndApplyRules error = %v, want %v", err, errBackend)
	}
	if want := []email.ID{"ok1", "ok2"}; !slices.Equal(remaining, want) {
		t.Errorf("returned %v, want %v", remaining, want)
	}
}

// Without per-message failures configured, batch methods behave exactly as
// before, so existing tests are unaffected.
func TestMockBatchOpsUnchangedWithoutFailOnMessage(t *testing.T) {
	m := &MockMailClient{Cfg: &cfg_g.Config{}}
	ids := []email.ID{"a", "b", "c"}

	if err := m.MoveEmail(ids, label.FromServer("INBOX"), label.FromServer("Archive")); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}
	if len(m.MovedEmails) != 1 || !slices.Equal(m.MovedEmails[0].MessageIDs, ids) {
		t.Errorf("recorded %+v, want all of %v", m.MovedEmails, ids)
	}
	if err := m.MarkAsRead(ids); err != nil {
		t.Errorf("MarkAsRead: %v", err)
	}
	if err := m.MoveToInbox(ids, label.FromServer("Archive")); err != nil {
		t.Errorf("MoveToInbox: %v", err)
	}
}

func TestMockPerMessageFailureAppliesToEveryBatchMethod(t *testing.T) {
	tests := []struct {
		name string
		call func(m *MockMailClient) error
	}{
		{name: "MoveEmail", call: func(m *MockMailClient) error {
			return m.MoveEmail([]email.ID{"bad"}, label.FromServer("INBOX"), label.FromServer("Archive"))
		}},
		{name: "CopyEmail", call: func(m *MockMailClient) error {
			return m.CopyEmail([]email.ID{"bad"}, label.FromServer("INBOX"), label.FromServer("Archive"))
		}},
		{name: "MoveToInbox", call: func(m *MockMailClient) error {
			return m.MoveToInbox([]email.ID{"bad"}, label.FromServer("Archive"))
		}},
		{name: "MarkAsRead", call: func(m *MockMailClient) error {
			return m.MarkAsRead([]email.ID{"bad"})
		}},
		{name: "ReportSpam", call: func(m *MockMailClient) error {
			return m.ReportSpam([]email.ID{"bad"}, label.FromServer("INBOX"))
		}},
		{name: "CheckAndApplyRules", call: func(m *MockMailClient) error {
			_, err := m.CheckAndApplyRules([]email.ID{"bad"}, label.FromServer("INBOX"), "")
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &MockMailClient{
				Cfg:           &cfg_g.Config{},
				FailOnMessage: map[email.ID]error{"bad": errBackend},
			}
			if err := tc.call(m); !errors.Is(err, errBackend) {
				t.Errorf("%s error = %v, want %v", tc.name, err, errBackend)
			}
		})
	}
}
