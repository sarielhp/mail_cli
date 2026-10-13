package jmap

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	mailemail "github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/train"

	"git.sr.ht/~rockorager/go-jmap"
	jmapMail "git.sr.ht/~rockorager/go-jmap/mail"
	jmapEmail "git.sr.ht/~rockorager/go-jmap/mail/email"
)

func TestBuildMovePatch_DistinctSourceAndDest(t *testing.T) {
	destID := jmap.ID("dest-mailbox-1")
	sourceID := jmap.ID("source-mailbox-2")

	patch := buildMovePatch(destID, sourceID)

	if len(patch) != 2 {
		t.Fatalf("expected patch to have 2 entries, got %d", len(patch))
	}

	destVal, destExists := patch["mailboxIds/dest-mailbox-1"]
	if !destExists || destVal != true {
		t.Errorf("expected mailboxIds/dest-mailbox-1 to be true, got %v (exists: %v)", destVal, destExists)
	}

	srcVal, srcExists := patch["mailboxIds/source-mailbox-2"]
	if !srcExists || srcVal != nil {
		t.Errorf("expected mailboxIds/source-mailbox-2 to be nil, got %v (exists: %v)", srcVal, srcExists)
	}

	// Verify root mailboxIds is not overwritten
	if _, rootExists := patch["mailboxIds"]; rootExists {
		t.Errorf("patch must not overwrite root mailboxIds property")
	}
}

func TestBuildMovePatch_EmptySource(t *testing.T) {
	destID := jmap.ID("dest-mailbox-1")

	patch := buildMovePatch(destID, "")

	if len(patch) != 1 {
		t.Fatalf("expected patch to have 1 entry, got %d", len(patch))
	}

	destVal, destExists := patch["mailboxIds/dest-mailbox-1"]
	if !destExists || destVal != true {
		t.Errorf("expected mailboxIds/dest-mailbox-1 to be true, got %v", destVal)
	}
}

func TestBuildMovePatch_IdenticalSourceAndDest(t *testing.T) {
	destID := jmap.ID("same-mailbox")
	sourceID := jmap.ID("same-mailbox")

	patch := buildMovePatch(destID, sourceID)

	if len(patch) != 1 {
		t.Fatalf("expected patch to have 1 entry, got %d", len(patch))
	}

	destVal, destExists := patch["mailboxIds/same-mailbox"]
	if !destExists || destVal != true {
		t.Errorf("expected mailboxIds/same-mailbox to be true, got %v", destVal)
	}
}

func TestMoveEmail_EarlyReturns(t *testing.T) {
	client := &client{
		account: cfg_acc.AccountConfig{Name: "test"},
		config:  &cfg_g.Config{},
	}

	// Empty message IDs should return immediately with no error
	if err := client.MoveEmail(nil, label.FromServer("INBOX"), label.FromServer("Archive")); err != nil {
		t.Errorf("MoveEmail with nil IDs returned error: %v", err)
	}

	if err := client.MoveEmail([]mailemail.ID{}, label.FromServer("INBOX"), label.FromServer("Archive")); err != nil {
		t.Errorf("MoveEmail with empty IDs returned error: %v", err)
	}

	// Equal source and dest should skip and return nil
	if err := client.MoveEmail([]mailemail.ID{"msg-1"}, label.FromServer("Archive"), label.FromServer("archive")); err != nil {
		t.Errorf("MoveEmail with identical folders returned error: %v", err)
	}
}

func TestParseRecipients_IncludesBccAndDedups(t *testing.T) {
	client := &client{}
	raw := []byte("From: sender@example.com\r\n" +
		"To: alice@example.com, bob@example.com\r\n" +
		"Cc: Charlie <charlie@example.com>, Alice <alice@example.com>\r\n" +
		"Bcc: secret@example.com, bob@example.com\r\n" +
		"Subject: Test\r\n\r\nBody here")

	rcptTo, err := client.parseRecipients(raw)
	if err != nil {
		t.Fatalf("parseRecipients failed: %v", err)
	}

	expected := map[string]bool{
		"alice@example.com":   false,
		"bob@example.com":     false,
		"charlie@example.com": false,
		"secret@example.com":  false,
	}

	if len(rcptTo) != len(expected) {
		t.Fatalf("expected %d recipients, got %d", len(expected), len(rcptTo))
	}

	for _, addr := range rcptTo {
		if _, ok := expected[addr.Email]; !ok {
			t.Errorf("unexpected recipient: %s", addr.Email)
		}
		expected[addr.Email] = true
	}

	for email, seen := range expected {
		if !seen {
			t.Errorf("expected recipient was missing: %s", email)
		}
	}
}

func TestStripBccHeader_RemovesBccAndPreservesRest(t *testing.T) {
	raw := []byte("From: sender@example.com\r\n" +
		"To: alice@example.com\r\n" +
		"Bcc: secret1@example.com,\r\n" +
		"   secret2@example.com\r\n" +
		"Subject: Important Update\r\n\r\nHello Alice,\r\nThis is the message body.")

	clean := stripBccHeader(raw)
	cleanStr := string(clean)

	if strings.Contains(cleanStr, "secret1@example.com") || strings.Contains(cleanStr, "secret2@example.com") || strings.Contains(cleanStr, "Bcc:") {
		t.Errorf("stripBccHeader failed to remove Bcc header: %s", cleanStr)
	}
	if !strings.Contains(cleanStr, "To: alice@example.com\r\n") {
		t.Errorf("To header missing: %s", cleanStr)
	}
	if !strings.Contains(cleanStr, "Subject: Important Update\r\n") {
		t.Errorf("Subject header missing: %s", cleanStr)
	}
	if !strings.Contains(cleanStr, "\r\n\r\nHello Alice,\r\nThis is the message body.") {
		t.Errorf("body or CRLF delimiter corrupted: %s", cleanStr)
	}
}

type recordReporter struct {
	errors []string
}

func (r *recordReporter) Infof(string, ...any)    {}
func (r *recordReporter) Successf(string, ...any) {}
func (r *recordReporter) Warnf(string, ...any)    {}
func (r *recordReporter) Errorf(format string, a ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, a...))
}
func (r *recordReporter) Printf(string, ...any)     {}
func (r *recordReporter) Progress(int, int, string) {}

func TestForEachNotDestroyedEmail(t *testing.T) {
	desc := "message locked"
	resp := &jmap.Response{
		Responses: []*jmap.Invocation{
			{
				Args: &jmapEmail.SetResponse{
					NotDestroyed: map[jmap.ID]*jmap.SetError{
						"msg-1": {
							Type:        "forbidden",
							Description: &desc,
						},
					},
				},
			},
		},
	}

	found := make(map[jmap.ID]string)
	forEachNotDestroyedEmail(resp, func(id jmap.ID, errSet *jmap.SetError) {
		found[id] = safeStr(errSet.Description)
	})

	if len(found) != 1 || found["msg-1"] != "message locked" {
		t.Fatalf("expected 1 not-destroyed message with description, got: %v", found)
	}
}

func TestDeleteUnsubscribed_NetworkError(t *testing.T) {
	rep := &recordReporter{}
	client := &client{
		config: &cfg_g.Config{},
		rep:    rep,
		accID:  "acc-1",
		client: &jmap.Client{
			Session: &jmap.Session{
				APIURL: "https://test.local/api",
			},
			HttpClient: &http.Client{
				Transport: mockTransport(func(_ *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(strings.NewReader("server down")),
						Header:     make(http.Header),
					}
				}),
			},
		},
	}

	client.deleteUnsubscribed([]string{"id-1", "id-2"}, true)
	if len(rep.errors) == 0 {
		t.Fatal("expected error reported on network failure, got none")
	}
	if !strings.Contains(rep.errors[0], "Failed to delete unsubscribed messages") {
		t.Errorf("unexpected error message: %s", rep.errors[0])
	}
}

func newTestMailSession(apiURL string) *jmap.Session {
	return &jmap.Session{
		APIURL: apiURL,
		RawCapabilities: map[jmap.URI]json.RawMessage{
			jmap.CoreURI: []byte("{}"),
			jmapMail.URI: []byte("{}"),
		},
		PrimaryAccounts: map[jmap.URI]jmap.ID{
			jmapMail.URI: "acc-1",
		},
		Accounts: map[jmap.ID]jmap.Account{
			"acc-1": {
				Capabilities: map[jmap.URI]jmap.Capability{
					jmapMail.URI: &jmapMail.Mail{},
				},
			},
		},
	}
}

func TestDeleteUnsubscribed_NotDestroyedReported(t *testing.T) {
	rep := &recordReporter{}
	client := &client{
		config: &cfg_g.Config{},
		rep:    rep,
		accID:  "acc-1",
		client: &jmap.Client{
			Session:    newTestMailSession("https://test.local/api"),
			HttpClient: jsonTransportClient(`{"sessionState":"state","methodResponses":[["Email/set",{"notDestroyed":{"msg-bad":{"type":"notFound","description":"already deleted"}}},"0"]]}`),
		},
	}

	client.deleteUnsubscribed([]string{"msg-bad"}, true)
	if len(rep.errors) == 0 {
		t.Fatal("expected per-message deletion error reported, got none")
	}
	if !strings.Contains(rep.errors[0], "Failed to delete message msg-bad") || !strings.Contains(rep.errors[0], "already deleted") {
		t.Errorf("unexpected error message: %s", rep.errors[0])
	}
}

func TestJMAPClient_LearnSpam_CorruptedLedger(t *testing.T) {
	tempDir := t.TempDir()
	corruptedPath := train.SpamUIDPath(cachedir.At(tempDir))
	if err := os.WriteFile(corruptedPath, []byte(`{invalid-json`), 0600); err != nil {
		t.Fatal(err)
	}

	sess := newTestMailSession("https://test.local/api")
	sess.DownloadURL = "https://test.local/download/{blobId}"

	client := &client{
		config:  &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: tempDir}, AccountCache: cachedir.At(tempDir)},
		account: cfg_acc.AccountConfig{Name: "test", SpamFolder: "Spam"},
		rep:     &recordReporter{},
		accID:   "acc-1",
		mailboxID: map[string]jmap.ID{
			"spam": "mb-spam",
		},
		client: &jmap.Client{
			Session: sess,
			HttpClient: &http.Client{
				Transport: mockTransport(func(req *http.Request) *http.Response {
					if req.Method == http.MethodGet {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader("Subject: spam\r\n\r\nbody")),
							Header:     make(http.Header),
						}
					}
					bodyBytes, _ := io.ReadAll(req.Body)
					reqStr := string(bodyBytes)
					var resBody string
					if strings.Contains(reqStr, "Email/query") {
						resBody = `{"sessionState":"state","methodResponses":[["Email/query",{"ids":["msg-1"]},"0"]]}`
					} else {
						resBody = `{"sessionState":"state","methodResponses":[["Email/get",{"list":[{"id":"msg-1","blobId":"blob-1"}]},"0"]]}`
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(resBody)),
						Header:     make(http.Header),
					}
				}),
			},
		},
	}

	err := client.LearnSpam()
	if err == nil {
		t.Fatal("expected error on corrupted ledger, got nil")
	}
	if !strings.Contains(err.Error(), "delete the file to start the trained-spam record over") {
		t.Errorf("expected remediation hint in error, got: %v", err)
	}
}
