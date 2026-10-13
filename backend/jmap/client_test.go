package jmap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"

	"git.sr.ht/~rockorager/go-jmap"
)

func TestNewJMAPClient(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:       "test",
		Type:       "jmap",
		Username:   "test@example.com",
		Password:   "token",
		SessionURL: "https://jmap.example.com",
	}
	config := &cfg_g.Config{}

	client := NewClient(acc, config, report.Discard())

	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	// Verify it implements MailClient interface
	var _ mailclient.MailClient = client
}

func TestJMAPClient_Validate(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:           "test",
		Type:           "jmap",
		Username:       "test@example.com",
		Password:       "token",
		SessionURL:     "https://jmap.example.com",
		SpamFolder:     "Spam",
		ReceivedFolder: "Archive",
	}
	config := &cfg_g.Config{}

	client := NewClient(acc, config, report.Discard())

	// Validate should succeed with valid config (without actually connecting)
	// The validation happens in init() which is called by Validate()
	// For testing, we just verify the config is valid
	err := client.Validate()
	if err != nil {
		// If it's a network error, that's expected - the config is valid
		if !strings.Contains(err.Error(), "JMAP authentication failed for test@example.com") {
			t.Errorf("Validate() returned unexpected error: %v", err)
		}
	}
}

func TestJMAPClient_InboxFolder(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:       "test",
		Type:       "jmap",
		Username:   "test@example.com",
		Password:   "token",
		SessionURL: "https://jmap.example.com",
	}
	config := &cfg_g.Config{}

	client := NewClient(acc, config, report.Discard())

	inbox := client.InboxFolder()
	if inbox.String() != "Inbox" {
		t.Errorf("InboxFolder() = %q, want %q", inbox, "Inbox")
	}
}

func TestJMAPClient_BackendType(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:       "test",
		Type:       "jmap",
		Username:   "test@example.com",
		Password:   "token",
		SessionURL: "https://jmap.example.com",
	}
	config := &cfg_g.Config{}

	client := NewClient(acc, config, report.Discard())

	backendType := client.BackendType()
	if backendType != "jmap" {
		t.Errorf("BackendType() = %q, want %q", backendType, "jmap")
	}
}

func TestJMAPClient_Config(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:       "test",
		Type:       "jmap",
		Username:   "test@example.com",
		Password:   "token",
		SessionURL: "https://jmap.example.com",
	}
	config := &cfg_g.Config{
		SelectedAccount: "test",
	}

	client := NewClient(acc, config, report.Discard())

	cfg := client.Config()
	if cfg == nil {
		t.Fatal("Config() returned nil")
	}
	if cfg.SelectedAccount != "test" {
		t.Errorf("Config().SelectedAccount = %q, want %q", cfg.SelectedAccount, "test")
	}
}

func TestJMAPClient_MarkAsRead_Empty(t *testing.T) {
	acc := cfg_acc.AccountConfig{
		Name:       "test",
		Type:       "jmap",
		Username:   "test@example.com",
		Password:   "token",
		SessionURL: "https://jmap.example.com",
	}
	client := NewClient(acc, &cfg_g.Config{}, report.Discard())
	if err := client.MarkAsRead(nil); err != nil {
		t.Errorf("expected nil error on empty messageIDs, got: %v", err)
	}
	if err := client.MarkAsRead([]email.ID{}); err != nil {
		t.Errorf("expected nil error on empty slice, got: %v", err)
	}
}

func TestJMAPClient_InitConfiguresHTTPTimeout(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"capabilities": map[string]interface{}{
				"urn:ietf:params:jmap:core": map[string]interface{}{
					"maxSizeUpload": 10000000,
				},
			},
			"accounts": map[string]interface{}{
				"acc1": map[string]interface{}{
					"name": "test@example.com",
				},
			},
			"primaryAccounts": map[string]interface{}{
				"urn:ietf:params:jmap:mail": "acc1",
			},
			"apiUrl":      ts.URL + "/api",
			"downloadUrl": ts.URL + "/download/{accountId}/{blobId}/{name}?type={type}",
		})
	}))
	defer ts.Close()

	acc := cfg_acc.AccountConfig{
		Name:       "test",
		Type:       "jmap",
		Username:   "test@example.com",
		Password:   "secret",
		SessionURL: ts.URL,
	}
	client := &client{
		account: acc,
		config:  &cfg_g.Config{},
	}
	_ = client.init()
	if client.client == nil || client.client.HttpClient == nil {
		t.Fatal("expected JMAP client and HttpClient to be initialized")
	}
	if client.client.HttpClient.Timeout != 60*time.Second {
		t.Errorf("expected 60s timeout, got %v", client.client.HttpClient.Timeout)
	}
}

func TestJMAPClient_GetMailboxID_ExactAndCaseInsensitive(t *testing.T) {
	c := &client{
		client: &jmap.Client{},
		mailboxID: map[string]jmap.ID{
			"Inbox":          "id-inbox",
			"Archive":        "id-archive",
			"INBOX/Projects": "id-proj",
		},
	}

	tests := []struct {
		lookup string
		wantID jmap.ID
	}{
		{"Inbox", "id-inbox"},
		{"inbox", "id-inbox"},
		{"INBOX", "id-inbox"},
		{"Archive", "id-archive"},
		{"archive", "id-archive"},
		{"ARCHIVE", "id-archive"},
		{"INBOX/Projects", "id-proj"},
		{"inbox/projects", "id-proj"},
		{"Projects", "id-proj"},
		{"projects", "id-proj"},
	}

	for _, tc := range tests {
		id, err := c.getMailboxID(tc.lookup)
		if err != nil {
			t.Errorf("getMailboxID(%q) unexpected error: %v", tc.lookup, err)
			continue
		}
		if id != tc.wantID {
			t.Errorf("getMailboxID(%q) = %q, want %q", tc.lookup, id, tc.wantID)
		}
	}

	if _, err := c.getMailboxID("NonExistent"); err == nil {
		t.Errorf("expected error for NonExistent, got nil")
	}
}

func TestJMAPClient_GetMailboxID_CasingCollision(t *testing.T) {
	c := &client{
		client: &jmap.Client{},
		mailboxID: map[string]jmap.ID{
			"Work": "id-work-upper",
			"work": "id-work-lower",
		},
	}

	// Exact matches must pick the exact key
	if id, err := c.getMailboxID("Work"); err != nil || id != "id-work-upper" {
		t.Errorf("getMailboxID(Work) = %q, err: %v, want id-work-upper", id, err)
	}
	if id, err := c.getMailboxID("work"); err != nil || id != "id-work-lower" {
		t.Errorf("getMailboxID(work) = %q, err: %v, want id-work-lower", id, err)
	}

	// Ambiguous casing lookup handles gracefully without error
	id, err := c.getMailboxID("WORK")
	if err != nil {
		t.Fatalf("getMailboxID(WORK) unexpected error: %v", err)
	}
	if id != "id-work-lower" && id != "id-work-upper" {
		t.Errorf("getMailboxID(WORK) unexpected ID: %q", id)
	}
}

func TestJMAPClient_GetMatchingLabels(t *testing.T) {
	c := &client{
		client: &jmap.Client{},
		mailboxID: map[string]jmap.ID{
			"Inbox":   "id1",
			"Archive": "id2",
			"Apple":   "id3",
			"Drafts":  "id4",
		},
	}

	matches, err := c.GetMatchingLabels("a")
	if err != nil {
		t.Fatalf("GetMatchingLabels failed: %v", err)
	}
	if len(matches) != 2 || matches[0] != "Apple" || matches[1] != "Archive" {
		t.Errorf("unexpected matches: %v", matches)
	}
}

type fakeJMAPNetError struct {
	timeout bool
}

func (e *fakeJMAPNetError) Error() string   { return "fake jmap network error" }
func (e *fakeJMAPNetError) Timeout() bool   { return e.timeout }
func (e *fakeJMAPNetError) Temporary() bool { return true }

func TestIsRetryableJMAPError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "generic error", err: errors.New("something broke"), want: false},
		{name: "jmap 429", err: &jmap.RequestError{Status: http.StatusTooManyRequests}, want: true},
		{name: "jmap 503", err: &jmap.RequestError{Status: http.StatusServiceUnavailable}, want: true},
		{name: "jmap 502", err: &jmap.RequestError{Status: http.StatusBadGateway}, want: true},
		{name: "jmap 504", err: &jmap.RequestError{Status: http.StatusGatewayTimeout}, want: true},
		{name: "jmap 400", err: &jmap.RequestError{Status: http.StatusBadRequest}, want: false},
		{name: "jmap 404", err: &jmap.RequestError{Status: http.StatusNotFound}, want: false},
		{name: "net.Error timeout", err: &fakeJMAPNetError{timeout: true}, want: true},
		{name: "net.Error no timeout", err: &fakeJMAPNetError{timeout: false}, want: false},
		{name: "string HTTP 429", err: errors.New("server returned HTTP 429 Too Many Requests"), want: true},
		{name: "string HTTP 503", err: errors.New("server returned HTTP 503 Service Unavailable"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableJMAPError(tt.err)
			if got != tt.want {
				t.Errorf("isRetryableJMAPError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExecuteJMAPOpWithRetry(t *testing.T) {
	origDelay := jmapRetryBaseDelay
	jmapRetryBaseDelay = time.Millisecond
	defer func() { jmapRetryBaseDelay = origDelay }()

	t.Run("succeeds on first attempt", func(t *testing.T) {
		calls := 0
		resp, err := executeJMAPOpWithRetry(context.Background(), "test op", func() (*jmap.Response, error) {
			calls++
			return &jmap.Response{}, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 1 || resp == nil {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("retries on 429 and succeeds", func(t *testing.T) {
		calls := 0
		resp, err := executeJMAPOpWithRetry(context.Background(), "test 429", func() (*jmap.Response, error) {
			calls++
			if calls < 3 {
				return nil, &jmap.RequestError{Status: http.StatusTooManyRequests}
			}
			return &jmap.Response{}, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 3 || resp == nil {
			t.Errorf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("retries on 503 exhausted", func(t *testing.T) {
		calls := 0
		_, err := executeJMAPOpWithRetry(context.Background(), "test 503", func() (*jmap.Response, error) {
			calls++
			return nil, &jmap.RequestError{Status: http.StatusServiceUnavailable}
		})
		if err == nil {
			t.Fatal("expected error after retries exhausted, got nil")
		}
		if calls != maxJMAPAttempts {
			t.Errorf("expected %d calls, got %d", maxJMAPAttempts, calls)
		}
	})

	t.Run("fails immediately on 400 without retry", func(t *testing.T) {
		calls := 0
		_, err := executeJMAPOpWithRetry(context.Background(), "test 400", func() (*jmap.Response, error) {
			calls++
			return nil, &jmap.RequestError{Status: http.StatusBadRequest}
		})
		if err == nil {
			t.Fatal("expected error on 400, got nil")
		}
		if calls != 1 {
			t.Errorf("expected 1 call for 400, got %d", calls)
		}
	})

	t.Run("aborts on context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		_, err := executeJMAPOpWithRetry(ctx, "test canceled", func() (*jmap.Response, error) {
			calls++
			return &jmap.Response{}, nil
		})
		if err == nil {
			t.Fatal("expected context canceled error, got nil")
		}
		if calls != 0 {
			t.Errorf("expected 0 calls when context already canceled, got %d", calls)
		}
	})
}
