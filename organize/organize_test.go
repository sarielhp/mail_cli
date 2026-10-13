package organize_test

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/organize"
	"github.com/sarielhp/mail_cli/report"
)

func TestResolveArchiveTarget(t *testing.T) {
	tests := []struct {
		name           string
		labels         []string
		receivedFolder string
		want           string
		wantErr        bool
	}{
		{
			name:    "Archive exact match",
			labels:  []string{"Inbox", "Archive", "Spam"},
			want:    "Archive",
			wantErr: false,
		},
		{
			name:    "Archive case-insensitive exact match",
			labels:  []string{"Inbox", "archive", "Spam"},
			want:    "archive",
			wantErr: false,
		},
		{
			name:    "Archive suffix match",
			labels:  []string{"Inbox", "[Gmail]/Archive", "Spam"},
			want:    "[Gmail]/Archive",
			wantErr: false,
		},
		{
			name:           "Configured receivedFolder exact match",
			labels:         []string{"Inbox", "custom-received", "Spam"},
			receivedFolder: "custom-received",
			want:           "custom-received",
			wantErr:        false,
		},
		{
			name:           "Configured receivedFolder suffix match",
			labels:         []string{"Inbox", "Personal/custom-received", "Spam"},
			receivedFolder: "custom-received",
			want:           "Personal/custom-received",
			wantErr:        false,
		},
		{
			name:    "Default received exact match",
			labels:  []string{"Inbox", "Received", "Spam"},
			want:    "Received",
			wantErr: false,
		},
		{
			name:    "Default received case-insensitive match",
			labels:  []string{"Inbox", "received", "Spam"},
			want:    "received",
			wantErr: false,
		},
		{
			name:    "Default received suffix match",
			labels:  []string{"Inbox", "Mail/received", "Spam"},
			want:    "Mail/received",
			wantErr: false,
		},
		{
			name:    "Neither exists error",
			labels:  []string{"Inbox", "Spam", "Sent"},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mailclient.MockMailClient{
				Labels: tt.labels,
				Cfg:    &cfg_g.Config{Policy: cfg_g.Policy{ReceivedFolder: tt.receivedFolder}},
			}
			got, err := organize.ResolveArchiveTarget(client)
			if (err != nil) != tt.wantErr {
				t.Fatalf("organize.ResolveArchiveTarget() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if got.String() != tt.want {
				t.Errorf("organize.ResolveArchiveTarget() got = %v, want = %v", got, tt.want)
			}
		})
	}
}

func TestArchiveAll(t *testing.T) {
	tempDir := t.TempDir()

	config := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir), Prefs: cfg_g.Prefs{Verbose: true}}

	client := &mailclient.MockMailClient{
		Labels: []string{"Inbox", "archive", "test/newsletter", "test/other", "spam"},
		Cfg:    config,
		DownloadedIDs: map[string][]email.ID{
			"Inbox":           {"msg1", "msg2"},
			"test/newsletter": {"msg3"},
			"test/other":      {"msg4", "msg5"},
		},
	}

	err := organize.All(report.Discard(), config, client, "inbox", label.FromServer("archive"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.MovedEmails) != 1 {
		t.Fatalf("expected 1 move operation, got %d", len(client.MovedEmails))
	}
	move := client.MovedEmails[0]
	if move.SourceLabel != "Inbox" || move.DestLabel != "archive" {
		t.Errorf("unexpected move source/dest: got %s -> %s, want Inbox -> archive", move.SourceLabel, move.DestLabel)
	}
	if len(move.MessageIDs) != 2 || move.MessageIDs[0] != "msg1" || move.MessageIDs[1] != "msg2" {
		t.Errorf("unexpected message IDs moved: %v", move.MessageIDs)
	}

	client.MovedEmails = nil

	err = organize.All(report.Discard(), config, client, "test", label.FromServer("archive"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.MovedEmails) != 2 {
		t.Fatalf("expected 2 move operations, got %d", len(client.MovedEmails))
	}

	var foundNewsletter, foundOther bool
	for _, m := range client.MovedEmails {
		switch {
		case m.SourceLabel == "test/newsletter":
			foundNewsletter = true
			if len(m.MessageIDs) != 1 || m.MessageIDs[0] != "msg3" {
				t.Errorf("unexpected newsletter IDs: %v", m.MessageIDs)
			}
		case m.SourceLabel == "test/other":
			foundOther = true
			if len(m.MessageIDs) != 2 || m.MessageIDs[0] != "msg4" || m.MessageIDs[1] != "msg5" {
				t.Errorf("unexpected other IDs: %v", m.MessageIDs)
			}
		default:
			t.Errorf("unexpected source label: %s", m.SourceLabel)
		}
		if m.DestLabel != "archive" {
			t.Errorf("unexpected dest label: %s", m.DestLabel)
		}
	}

	if !foundNewsletter || !foundOther {
		t.Errorf("did not find expected newsletter and other moves: newsletter=%v, other=%v", foundNewsletter, foundOther)
	}
}

func TestResolveArchiveTargetGmailFallback(t *testing.T) {
	tempDir := t.TempDir()

	configContent := `{
		"accounts": [
			{
				"name": "gmail-test",
				"type": "gmail",
				"received_folder": "INBOX"
			}
		]
	}`
	configPath := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("failed to write dummy config: %v", err)
	}

	config := &cfg_g.Config{Paths: cfg_g.Paths{ConfigDir: tempDir}, Policy: cfg_g.Policy{ReceivedFolder: "INBOX"}}

	client := &mailclient.MockMailClient{
		Labels:         []string{"Inbox", "Spam", "Sent"},
		Cfg:            config,
		BackendTypeVal: "gmail",
	}

	got, err := organize.ResolveArchiveTarget(client)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.String() != "Archive" {
		t.Errorf("expected archive target 'Archive', got %q", got)
	}
}

func TestResolveDraftsTarget(t *testing.T) {
	client := &mailclient.MockMailClient{
		Labels:         []string{"INBOX", "Drafts", "Sent"},
		BackendTypeVal: "jmap",
	}
	got, err := organize.ResolveDraftsTarget(client)
	if err != nil || got.String() != "Drafts" {
		t.Errorf("expected 'Drafts', got %q, err %v", got, err)
	}

	gmailClient := &mailclient.MockMailClient{
		Labels:         []string{"INBOX"},
		BackendTypeVal: "gmail",
	}
	gotGmail, err := organize.ResolveDraftsTarget(gmailClient)
	if err != nil || gotGmail.String() != "DRAFTS" {
		t.Errorf("expected 'DRAFTS', got %q, err %v", gotGmail, err)
	}
}

func TestResolveSentTarget(t *testing.T) {
	client := &mailclient.MockMailClient{
		Labels:         []string{"INBOX", "Sent Items"},
		BackendTypeVal: "outlook",
	}
	got, err := organize.ResolveSentTarget(client)
	if err != nil || got.String() != "Sent Items" {
		t.Errorf("expected 'Sent Items', got %q, err %v", got, err)
	}

	gmailClient := &mailclient.MockMailClient{
		Labels:         []string{"INBOX"},
		BackendTypeVal: "gmail",
	}
	gotGmail, err := organize.ResolveSentTarget(gmailClient)
	if err != nil || gotGmail.String() != "SENT" {
		t.Errorf("expected 'SENT', got %q, err %v", gotGmail, err)
	}
}
