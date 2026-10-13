package mailclient

import (
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestReadOnlyMailClient_PassThrough(t *testing.T) {
	cfg := &cfg_g.Config{Credentials: cfg_g.Credentials{Username: "user@example.com"}, Paths: cfg_g.Paths{CacheRoot: "/tmp/test"}, AccountCache: cachedir.At("/tmp/test")}
	mock := &MockMailClient{
		Cfg:            cfg,
		Labels:         []string{"INBOX", "Archive", "Spam"},
		LabelItems:     []cfg_acc.Item{{Name: "INBOX", FullName: "INBOX"}},
		InboxFolderVal: "INBOX",
		BackendTypeVal: "jmap",
		DownloadedIDs: map[string][]email.ID{
			"INBOX": {"msg1", "msg2"},
		},
	}

	ro := NewReadOnlyMailClient(mock, "test-acc")

	if err := ro.Validate(); err != nil {
		t.Fatalf("unexpected Validate error: %v", err)
	}

	if ro.InboxFolder().String() != "INBOX" {
		t.Errorf("expected INBOX, got %q", ro.InboxFolder())
	}

	if ro.BackendType() != "jmap" {
		t.Errorf("expected jmap, got %q", ro.BackendType())
	}

	if ro.Config() != cfg {
		t.Errorf("expected config pass-through")
	}

	labels, err := ro.GetMatchingLabels("IN")
	if err != nil || len(labels) != 1 || labels[0] != "INBOX" {
		t.Errorf("unexpected GetMatchingLabels result: %v, err: %v", labels, err)
	}

	items, err := ro.GetLabelItems()
	if err != nil || len(items) != 1 {
		t.Errorf("unexpected GetLabelItems result: %v, err: %v", items, err)
	}

	ids, err := ro.FetchAndDownloadEmails(label.FromServer("INBOX"), "inbox")
	if err != nil || len(ids) != 2 {
		t.Errorf("unexpected FetchAndDownloadEmails result: %v, err: %v", ids, err)
	}

	// Listing labels is a read and must pass through. It used to be
	// ListLabels, which printed; the data now comes back from GetLabelItems
	// and the caller renders it.
	if _, err := ro.GetLabelItems(); err != nil {
		t.Errorf("unexpected GetLabelItems error: %v", err)
	}

	// Listing filters is a read and must pass through. It used to be
	// ListFilters, which printed; the rules now come back as data.
	if _, err := ro.GetFilters(); err != nil {
		t.Errorf("unexpected GetFilters error: %v", err)
	}

	if ro.RewriteRuleLabelCasing("Test") != "" {
		t.Errorf("unexpected RewriteRuleLabelCasing output")
	}
}

func TestReadOnlyMailClient_NoOps(t *testing.T) {
	mock := &MockMailClient{}
	ro := NewReadOnlyMailClient(mock, "test-acc")

	// MoveEmail should be simulated no-op and not modify mock
	if err := ro.MoveEmail([]email.ID{"msg1"}, label.FromServer("INBOX"), label.FromServer("Archive")); err != nil {
		t.Errorf("expected nil error on simulated MoveEmail, got: %v", err)
	}
	if len(mock.MovedEmails) != 0 {
		t.Errorf("expected no delegate MoveEmail call, got %d calls", len(mock.MovedEmails))
	}

	// CopyEmail should be simulated no-op
	if err := ro.CopyEmail([]email.ID{"msg1"}, label.FromServer("INBOX"), label.FromServer("Archive")); err != nil {
		t.Errorf("expected nil error on simulated CopyEmail, got: %v", err)
	}
	if len(mock.CopiedEmails) != 0 {
		t.Errorf("expected no delegate CopyEmail call")
	}

	// MoveToInbox
	if err := ro.MoveToInbox([]email.ID{"msg1"}, label.FromServer("Spam")); err != nil {
		t.Errorf("expected nil error on simulated MoveToInbox, got: %v", err)
	}

	// MarkAsRead
	if err := ro.MarkAsRead([]email.ID{"msg1"}); err != nil {
		t.Errorf("expected nil error on simulated MarkAsRead, got: %v", err)
	}

	// ReportSpam
	if err := ro.ReportSpam([]email.ID{"msg1"}, label.FromServer("INBOX")); err != nil {
		t.Errorf("expected nil error on simulated ReportSpam, got: %v", err)
	}

	// MoveAllSpam
	if err := ro.MoveAllSpam(label.FromServer("Spam")); err != nil {
		t.Errorf("expected nil error on simulated MoveAllSpam, got: %v", err)
	}

	// LearnSpam
	if err := ro.LearnSpam(); err != nil {
		t.Errorf("expected nil error on simulated LearnSpam, got: %v", err)
	}

	// RenameLabel
	if err := ro.RenameLabel(label.FromServer("old"), label.FromServer("new")); err != nil {
		t.Errorf("expected nil error on simulated RenameLabel, got: %v", err)
	}

	// FixLabels
	if err := ro.FixLabels(); err != nil {
		t.Errorf("expected nil error on simulated FixLabels, got: %v", err)
	}

	// EnsureLabelExists
	if err := ro.EnsureLabelExists(label.FromServer("newLabel")); err != nil {
		t.Errorf("expected nil error on simulated EnsureLabelExists, got: %v", err)
	}

	// CheckAndApplyRules
	ruleIDs, err := ro.CheckAndApplyRules([]email.ID{"msg1", "msg2"}, label.FromServer("INBOX"), "inbox")
	if err != nil || len(ruleIDs) != 2 {
		t.Errorf("unexpected CheckAndApplyRules result: %v, err: %v", ruleIDs, err)
	}
}

func TestReadOnlyMailClient_BlockedMutations(t *testing.T) {
	mock := &MockMailClient{}
	ro := NewReadOnlyMailClient(mock, "test-acc")

	// DeleteAllSpam
	if err := ro.DeleteAllSpam(); !errors.Is(err, ErrReadOnlyOperationBlocked) {
		t.Errorf("expected ErrReadOnlyOperationBlocked on DeleteAllSpam, got: %v", err)
	}

	// DeleteLabel
	if err := ro.DeleteLabel(label.FromServer("old")); !errors.Is(err, ErrReadOnlyOperationBlocked) {
		t.Errorf("expected ErrReadOnlyOperationBlocked on DeleteLabel, got: %v", err)
	}

	// UploadRawEmail
	if _, err := ro.UploadRawEmail([]byte("dummy"), label.FromServer("INBOX")); !errors.Is(err, ErrReadOnlyOperationBlocked) {
		t.Errorf("expected ErrReadOnlyOperationBlocked on UploadRawEmail, got: %v", err)
	}

	// SendEmail
	if err := ro.SendEmail([]byte("dummy")); !errors.Is(err, ErrReadOnlyOperationBlocked) {
		t.Errorf("expected ErrReadOnlyOperationBlocked on SendEmail, got: %v", err)
	}

	// ExportRules is a server mutation: on Gmail it deletes and creates real
	// server-side filters. It was previously asserted as a pass-through, which is
	// how the read-only bypass survived.
	if err := ro.ExportRules(); !errors.Is(err, ErrReadOnlyOperationBlocked) {
		t.Errorf("expected ErrReadOnlyOperationBlocked on ExportRules, got: %v", err)
	}
	if mock.ExportRulesCalls != 0 {
		t.Errorf("ExportRules reached the delegate %d time(s); must never happen in read-only mode", mock.ExportRulesCalls)
	}

	// DeleteFilter is a server mutation: it deletes server-side filters.
	if err := ro.DeleteFilter("filter123"); !errors.Is(err, ErrReadOnlyOperationBlocked) {
		t.Errorf("expected ErrReadOnlyOperationBlocked on DeleteFilter, got: %v", err)
	}
	if len(mock.DeletedFilters) != 0 {
		t.Errorf("DeleteFilter reached the delegate; must never happen in read-only mode")
	}

	// DeleteMessages is a permanent server deletion.
	if err := ro.DeleteMessages([]email.ID{"m1"}, label.FromServer("Drafts")); !errors.Is(err, ErrReadOnlyOperationBlocked) {
		t.Errorf("expected ErrReadOnlyOperationBlocked on DeleteMessages, got: %v", err)
	}
	if len(mock.DeletedMessages) != 0 {
		t.Errorf("DeleteMessages reached the delegate; must never happen in read-only mode")
	}
}

// The embedded Reader and Delegate must be the same client. If they drifted,
// reads would come from one backend and blocked writes be logged against
// another -- and a constructor change could silently leave Reader nil, which
// panics on the first read.
func TestReadOnlyEmbeddedReaderIsTheDelegate(t *testing.T) {
	mock := &MockMailClient{}
	ro := NewReadOnlyMailClient(mock, "acc")

	if ro.Reader == nil {
		t.Fatal("embedded Reader was not set")
	}
	if ro.Reader != Reader(ro.Delegate) {
		t.Error("embedded Reader and Delegate are different clients")
	}
}

// Reads pass through the embedding without being intercepted.
func TestReadOnlyForwardsReadsThroughTheEmbeddedReader(t *testing.T) {
	mock := &MockMailClient{Labels: []string{"INBOX"}}
	ro := NewReadOnlyMailClient(mock, "acc")

	got, err := ro.GetMatchingLabels("IN")
	if err != nil {
		t.Fatalf("GetMatchingLabels: %v", err)
	}
	if len(got) != 1 || got[0] != "INBOX" {
		t.Errorf("expected the delegate's labels, got %v", got)
	}
}
