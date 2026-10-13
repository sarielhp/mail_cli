package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

func TestMarshalReportsEmptyIsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := MarshalReports(&buf, nil); err != nil {
		t.Fatalf("MarshalReports: %v", err)
	}
	if got := buf.String(); got != "[]\n" {
		t.Errorf("empty reports should marshal to [], got %q", got)
	}
}

// An empty folder returns before its messages are built; its report must
// still carry "messages": [], not null.
func TestMarshalReportsEmptyFolderMessagesIsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := MarshalReports(&buf, []FolderReport{{Folder: "INBOX"}}); err != nil {
		t.Fatalf("MarshalReports: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"messages": []`)) {
		t.Errorf("an empty folder should report \"messages\": [], got %s", buf.String())
	}
}

func TestBuildMessageReportsFlags(t *testing.T) {
	accountDir := filepath.Join(t.TempDir(), "acct")
	if err := os.MkdirAll(accountDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dir := cachedir.At(accountDir)
	date := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	_ = msg.Store(dir, "spam1", []byte("From: spammer@example.com\nSubject: Buy now\n\nbody"), date)
	_ = msg.Store(dir, "ham1", []byte("From: friend@example.com\nSubject: Lunch\n\nbody"), date)

	config := &cfg_g.Config{AccountCache: dir}
	results := map[email.ID]*Response{
		"spam1": {
			Action:  "Blacklisted Sender",
			Score:   12.5,
			Symbols: map[string]Symbol{"CUSTOM_POLITICAL_BLOCK": {}},
		},
	}

	reports := buildMessageReports(config, true, []email.ID{"spam1", "ham1"}, []email.ID{"spam1"}, []email.ID{"spam1"}, results)
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	if !reports[0].IsSpam || !reports[0].IsBlacklisted || !reports[0].IsPolitical {
		t.Errorf("spam1 flags wrong: %+v", reports[0])
	}
	if reports[0].Subject != "Buy now" || reports[0].From != "spammer@example.com" {
		t.Errorf("spam1 headers wrong: %+v", reports[0])
	}
	if reports[0].Score != 12.5 || reports[0].Action != "Blacklisted Sender" {
		t.Errorf("spam1 verdict wrong: %+v", reports[0])
	}
	if reports[1].IsSpam || reports[1].IsBlacklisted || reports[1].IsPolitical {
		t.Errorf("ham1 should carry no flags: %+v", reports[1])
	}
}

// JSON mode must return the structured report and leave the product writer
// empty -- the human table and the JSON document are the same channel, so
// writing both would corrupt the machine-readable output.
func TestPerformJSONReturnsReportsNotTable(t *testing.T) {
	accountDir := filepath.Join(t.TempDir(), "acct")
	if err := os.MkdirAll(accountDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dir := cachedir.At(accountDir)
	date := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	_ = msg.Store(dir, "m1", []byte("From: a@example.com\nSubject: One\n\nbody"), date)

	config := &cfg_g.Config{SelectedAccount: "acct", Paths: cfg_g.Paths{CacheRoot: accountDir}, AccountCache: dir}
	client := &mailclient.MockMailClient{
		Cfg:           config,
		Labels:        []string{"received"},
		DownloadedIDs: map[string][]email.ID{"received": {"m1"}},
	}
	// A current stamp means the cached verdict is reused, so the test never
	// shells out to bogofilter.
	if err := msg.SetEvidence(dir, "m1", msg.Evidence{BogoScore: 0.0, Stamp: Stamp(config)}); err != nil {
		t.Fatalf("set evidence: %v", err)
	}

	var out bytes.Buffer
	moved, reports, err := Perform(&out, Options{JSON: true}, report.Discard(), client, config, "received")
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if moved != 0 {
		t.Errorf("expected no moves, got %d", moved)
	}
	if len(reports) != 1 || reports[0].Folder != "received" || reports[0].Total != 1 {
		t.Fatalf("unexpected reports: %+v", reports)
	}
	if out.Len() != 0 {
		t.Errorf("JSON mode must not write the human table, got %q", out.String())
	}
}
