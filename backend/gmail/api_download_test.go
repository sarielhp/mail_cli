package gmail

import (
	"context"
	"errors"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/report"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type fakeNetError struct {
	timeout bool
}

func (e *fakeNetError) Error() string   { return "fake network error" }
func (e *fakeNetError) Timeout() bool   { return e.timeout }
func (e *fakeNetError) Temporary() bool { return true }

// newGmailTestService builds a Gmail service bound to a test server.
func newGmailTestService(t *testing.T, ts *httptest.Server) *gmailapi.Service {
	t.Helper()
	srv, err := gmailapi.NewService(context.Background(), option.WithEndpoint(ts.URL), option.WithHTTPClient(ts.Client()))
	if err != nil {
		t.Fatalf("failed to create gmail service: %v", err)
	}
	return srv
}

// retry429Server starts a test server that answers its first request with a 429
// rate-limit error and every later request with successBody, counting attempts.
func retry429Server(attempts *int, successBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*attempts++
		if *attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"error": {"code": 429, "message": "rate limit"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(successBody))
	}))
}

// canceledContextService builds a Gmail service bound to an already-canceled
// context, so the next call reports context.Canceled before any request.
func canceledContextService(t *testing.T) (context.Context, *gmailapi.Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv, err := gmailapi.NewService(ctx, option.WithHTTPClient(http.DefaultClient))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	return ctx, srv
}

func TestIsRetryableGmailError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic error",
			err:  errors.New("plain network error"),
			want: false,
		},
		{
			name: "googleapi 429 rate limit",
			err:  &googleapi.Error{Code: http.StatusTooManyRequests, Message: "rateLimitExceeded"},
			want: true,
		},
		{
			name: "googleapi 503 service unavailable",
			err:  &googleapi.Error{Code: http.StatusServiceUnavailable, Message: "backendError"},
			want: true,
		},
		{
			name: "googleapi 400 bad request",
			err:  &googleapi.Error{Code: http.StatusBadRequest, Message: "badRequest"},
			want: false,
		},
		{
			name: "googleapi 404 not found",
			err:  &googleapi.Error{Code: http.StatusNotFound, Message: "notFound"},
			want: false,
		},
		{
			name: "net.Error with timeout",
			err:  &fakeNetError{timeout: true},
			want: true,
		},
		{
			name: "net.Error without timeout",
			err:  &fakeNetError{timeout: false},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableGmailError(tt.err)
			if got != tt.want {
				t.Errorf("isRetryableGmailError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExecuteGmailCallWithRetry(t *testing.T) {
	origDelay := gmailListRetryBaseDelay
	gmailListRetryBaseDelay = time.Millisecond
	defer func() { gmailListRetryBaseDelay = origDelay }()

	t.Run("succeeds on first attempt", func(t *testing.T) {
		calls := 0
		expected := &gmailapi.ListMessagesResponse{
			Messages: []*gmailapi.Message{{Id: "msg1"}},
		}
		res, err := executeGmailCallWithRetry(context.Background(), func() (*gmailapi.ListMessagesResponse, error) {
			calls++
			return expected, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
		if len(res.Messages) != 1 || res.Messages[0].Id != "msg1" {
			t.Errorf("unexpected response: %+v", res)
		}
	})

	t.Run("retries on 429 and succeeds", func(t *testing.T) {
		calls := 0
		res, err := executeGmailCallWithRetry(context.Background(), func() (*gmailapi.ListMessagesResponse, error) {
			calls++
			if calls < 3 {
				return nil, &googleapi.Error{Code: http.StatusTooManyRequests}
			}
			return &gmailapi.ListMessagesResponse{Messages: []*gmailapi.Message{{Id: "msg2"}}}, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
		if len(res.Messages) != 1 || res.Messages[0].Id != "msg2" {
			t.Errorf("unexpected response: %+v", res)
		}
	})

	t.Run("retries on 503 exhausted", func(t *testing.T) {
		calls := 0
		_, err := executeGmailCallWithRetry(context.Background(), func() (*gmailapi.ListMessagesResponse, error) {
			calls++
			return nil, &googleapi.Error{Code: http.StatusServiceUnavailable}
		})
		if err == nil {
			t.Fatal("expected error after retries exhausted, got nil")
		}
		if calls != maxGmailListAttempts {
			t.Errorf("expected %d calls, got %d", maxGmailListAttempts, calls)
		}
	})

	t.Run("fails immediately on 400 without retry", func(t *testing.T) {
		calls := 0
		_, err := executeGmailCallWithRetry(context.Background(), func() (*gmailapi.ListMessagesResponse, error) {
			calls++
			return nil, &googleapi.Error{Code: http.StatusBadRequest}
		})
		if err == nil {
			t.Fatal("expected error on 400, got nil")
		}
		if calls != 1 {
			t.Errorf("expected 1 call for 400, got %d", calls)
		}
	})
}

func TestExecuteGmailOpWithRetry(t *testing.T) {
	origDelay := gmailListRetryBaseDelay
	gmailListRetryBaseDelay = time.Millisecond
	defer func() { gmailListRetryBaseDelay = origDelay }()

	t.Run("generic retry succeeds after 429", func(t *testing.T) {
		calls := 0
		lbl, err := executeGmailOpWithRetry(context.Background(), "test op", func() (*gmailapi.Label, error) {
			calls++
			if calls < 3 {
				return nil, &googleapi.Error{Code: http.StatusTooManyRequests}
			}
			return &gmailapi.Label{Id: "L_test", Name: "Test"}, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
		if lbl.Id != "L_test" {
			t.Errorf("expected label ID L_test, got %s", lbl.Id)
		}
	})

	t.Run("generic retry succeeds after net.Error timeout", func(t *testing.T) {
		calls := 0
		lbl, err := executeGmailOpWithRetry(context.Background(), "test net timeout", func() (*gmailapi.Label, error) {
			calls++
			if calls < 2 {
				return nil, &fakeNetError{timeout: true}
			}
			return &gmailapi.Label{Id: "L_net", Name: "NetTest"}, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 2 {
			t.Errorf("expected 2 calls, got %d", calls)
		}
		if lbl.Id != "L_net" {
			t.Errorf("expected label ID L_net, got %s", lbl.Id)
		}
	})
}

func TestFetchFolderUnreadIDs_Retry(t *testing.T) {
	origDelay := gmailListRetryBaseDelay
	gmailListRetryBaseDelay = time.Millisecond
	defer func() { gmailListRetryBaseDelay = origDelay }()

	attempts := 0
	ts := retry429Server(&attempts, `{"messages": [{"id": "msg_unread_1"}]}`)
	defer ts.Close()

	srv := newGmailTestService(t, ts)

	unreadIDs, err := fetchFolderUnreadIDs(context.Background(), srv, "INBOX", "INBOX", 10)
	if err != nil {
		t.Fatalf("fetchFolderUnreadIDs failed: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts (1 retry), got %d", attempts)
	}
	if len(unreadIDs) != 1 || unreadIDs[0] != "msg_unread_1" {
		t.Errorf("unexpected unread IDs: %v", unreadIDs)
	}
}

func TestRewriteRuleLabelCasing_Fallback(t *testing.T) {
	// Nil config fallback
	resultNil := rewriteRuleLabelCasing(report.Discard(), nil, "Work/Invoices")
	if resultNil != "Work/Invoices" {
		t.Errorf("expected original label 'Work/Invoices', got %q", resultNil)
	}

	// Non-nil config with missing credentials causes GetGmailService to fail
	cfg := &Config{Paths: cfg_g.Paths{ConfigDir: t.TempDir()}}
	resultMissing := rewriteRuleLabelCasing(report.Discard(), cfg, "Work/Invoices")
	if resultMissing != "Work/Invoices" {
		t.Errorf("expected original label 'Work/Invoices', got %q", resultMissing)
	}
}

func TestListFolderMessageIDs_ContextCanceled(t *testing.T) {
	ctx, srv := canceledContextService(t)

	_, err := listFolderMessageIDs(ctx, srv, "INBOX", "INBOX", 10)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestFetchFolderUnreadIDs_ContextCanceled(t *testing.T) {
	ctx, srv := canceledContextService(t)

	_, err := fetchFolderUnreadIDs(ctx, srv, "INBOX", "INBOX", 10)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestEnsureLabelHierarchyREST_RetryOn429(t *testing.T) {
	origDelay := gmailListRetryBaseDelay
	gmailListRetryBaseDelay = time.Millisecond
	defer func() { gmailListRetryBaseDelay = origDelay }()

	createAttempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/labels") {
			createAttempts++
			if createAttempts == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"error": {"code": 429, "message": "rate limit"}}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id": "L_new", "name": "Clients"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	srv := newGmailTestService(t, ts)

	labelNameToID := make(map[string]string)
	labelIDToName := make(map[string]string)

	targetID, err := ensureLabelHierarchyREST(report.Discard(), srv, "Clients", labelNameToID, labelIDToName)
	if err != nil {
		t.Fatalf("ensureLabelHierarchyREST failed: %v", err)
	}
	if targetID != "L_new" {
		t.Errorf("expected target ID L_new, got %s", targetID)
	}
	if createAttempts != 2 {
		t.Errorf("expected 2 creation attempts (1 retry), got %d", createAttempts)
	}
}

func TestEnsureLabelHierarchyREST_ContextCanceled(t *testing.T) {
	ctx, srv := canceledContextService(t)

	labelNameToID := make(map[string]string)
	labelIDToName := make(map[string]string)
	_, err := ensureLabelHierarchyRESTContext(ctx, report.Discard(), srv, "NewFolder", labelNameToID, labelIDToName)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestExecuteGmailOpWithRetry_ContextCanceled(t *testing.T) {
	origDelay := gmailListRetryBaseDelay
	gmailListRetryBaseDelay = 500 * time.Millisecond
	defer func() { gmailListRetryBaseDelay = origDelay }()

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	start := time.Now()
	_, err := executeGmailOpWithRetry(ctx, "test cancel", func() (*gmailapi.Label, error) {
		calls++
		if calls == 1 {
			cancel()
			return nil, &googleapi.Error{Code: http.StatusTooManyRequests}
		}
		return &gmailapi.Label{Id: "L1"}, nil
	})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if elapsed >= 450*time.Millisecond {
		t.Errorf("expected early return on canceled context, took %v", elapsed)
	}
}
