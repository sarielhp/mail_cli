package cli

import (
	"context"
	"errors"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/virtual"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

// resetSearchFlags clears the package-level flag variables, which persist
// between runs because clihelp binds them once.
func resetSearchFlags(t *testing.T) {
	t.Helper()
	flagSearchOutput = ""
	flagSearchLimit = defaultSearchLimit
	flagSearchAll = false
	flagSearchReverse = false
	t.Cleanup(func() {
		flagSearchOutput = ""
		flagSearchLimit = defaultSearchLimit
		flagSearchAll = false
		flagSearchReverse = false
	})
}

func searchMessage(subject string, date time.Time) []byte {
	return []byte("From: sender@example.com\r\nTo: me@example.com\r\nSubject: " + subject +
		"\r\nDate: " + date.Format(time.RFC1123Z) + "\r\n\r\nbody\r\n")
}

// searchSession wires a mock backend in behind a session, the way runSearch
// reaches it.
func searchSession(t *testing.T, mock *mailclient.MockMailClient) *Session {
	t.Helper()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}
	mock.Cfg = cfg
	return &Session{
		Config: cfg,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(*cfg_g.Config) (mailclient.MailClient, error) {
					return mock, nil
				},
			},
		},
	}
}

func threeMatchMock() *mailclient.MockMailClient {
	return &mailclient.MockMailClient{
		SearchResults: map[string][]email.ID{"": {"newest", "oldest", "middle"}},
		RawMessages: map[email.ID][]byte{
			"oldest": searchMessage("first", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
			"middle": searchMessage("second", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)),
			"newest": searchMessage("third", time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)),
		},
	}
}

// withQuietStdout swallows the table runSearch prints, so test output stays
// readable. The listing itself is exercised by ui's own rendering.
func withQuietStdout(t *testing.T, fn func()) {
	t.Helper()
	orig := os.Stdout
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("opening %s: %v", os.DevNull, err)
	}
	os.Stdout = devNull
	defer func() {
		os.Stdout = orig
		_ = devNull.Close()
	}()
	fn()
}

func TestRunSearchSavesResultsOldestFirst(t *testing.T) {
	resetSearchFlags(t)
	mock := threeMatchMock()
	session := searchSession(t, mock)

	withQuietStdout(t, func() {
		if err := runSearch(session, "anything"); err != nil {
			t.Fatalf("runSearch: %v", err)
		}
	})

	vm, err := virtual.Load(session.Config.AccountCache, "search")
	if err != nil {
		t.Fatalf("the result set was not saved: %v", err)
	}
	want := []email.ID{"oldest", "middle", "newest"}
	if len(vm.MessageIDs) != len(want) {
		t.Fatalf("saved %v, want %v", vm.MessageIDs, want)
	}
	for i, id := range want {
		if vm.MessageIDs[i] != id {
			t.Errorf("saved order %v, want %v (oldest first)", vm.MessageIDs, want)
			break
		}
	}
}

func TestRunSearchReverseOrdersNewestFirst(t *testing.T) {
	resetSearchFlags(t)
	flagSearchReverse = true
	mock := threeMatchMock()
	session := searchSession(t, mock)

	withQuietStdout(t, func() {
		if err := runSearch(session, "anything"); err != nil {
			t.Fatalf("runSearch: %v", err)
		}
	})

	vm, err := virtual.Load(session.Config.AccountCache, "search")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if vm.MessageIDs[0] != "newest" {
		t.Errorf("saved order %v, want newest first under --reverse", vm.MessageIDs)
	}
}

// --all must ask the backend for everything, not fetch a page and trim.
func TestRunSearchAllRequestsEveryMatch(t *testing.T) {
	resetSearchFlags(t)
	flagSearchAll = true
	mock := threeMatchMock()
	session := searchSession(t, mock)

	withQuietStdout(t, func() {
		if err := runSearch(session, "taxes"); err != nil {
			t.Fatalf("runSearch: %v", err)
		}
	})

	if mock.LastSearchLimit != 0 {
		t.Errorf("backend was asked for limit %d, want 0 (unbounded) under --all", mock.LastSearchLimit)
	}
	if mock.LastSearchQuery != "taxes" {
		t.Errorf("backend received query %q, want %q", mock.LastSearchQuery, "taxes")
	}
}

func TestRunSearchPassesTheLimitThrough(t *testing.T) {
	resetSearchFlags(t)
	flagSearchLimit = 7
	mock := threeMatchMock()
	session := searchSession(t, mock)

	withQuietStdout(t, func() {
		if err := runSearch(session, "invoice"); err != nil {
			t.Fatalf("runSearch: %v", err)
		}
	})
	if mock.LastSearchLimit != 7 {
		t.Errorf("backend was asked for limit %d, want 7", mock.LastSearchLimit)
	}
}

func TestRunSearchNoMatchesIsNotAnError(t *testing.T) {
	resetSearchFlags(t)
	mock := &mailclient.MockMailClient{SearchResults: map[string][]email.ID{"": nil}}
	session := searchSession(t, mock)

	withQuietStdout(t, func() {
		if err := runSearch(session, "nothing matches this"); err != nil {
			t.Errorf("runSearch with no matches = %v, want nil", err)
		}
	})

	// Nothing found means nothing to browse afterwards.
	if _, err := virtual.Load(session.Config.AccountCache, "search"); err == nil {
		t.Error("an empty search saved a result set")
	}
}

func TestRunSearchReportsBackendFailure(t *testing.T) {
	resetSearchFlags(t)
	boom := errors.New("server said no")
	mock := threeMatchMock()
	mock.FailOn = map[string]error{"SearchEmails": boom}
	session := searchSession(t, mock)

	err := runSearch(session, "anything")
	if !errors.Is(err, boom) {
		t.Errorf("runSearch error = %v, want %v", err, boom)
	}
}

func TestRunSearchExportsToMbox(t *testing.T) {
	resetSearchFlags(t)
	mock := threeMatchMock()
	session := searchSession(t, mock)

	// The export reads bodies back from the cache, so seed it the way a real
	// PrefetchMessages would have.
	for id, raw := range mock.RawMessages {
		if err := storeForExport(t, session.Config.AccountCache.Path(), string(id), raw); err != nil {
			t.Fatalf("seeding %s: %v", id, err)
		}
	}

	dest := filepath.Join(t.TempDir(), "results.mbox")
	flagSearchOutput = dest

	withQuietStdout(t, func() {
		if err := runSearch(session, "anything"); err != nil {
			t.Fatalf("runSearch: %v", err)
		}
	})

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading the archive: %v", err)
	}
	text := string(data)
	if strings.Index(text, "Subject: first") > strings.Index(text, "Subject: third") {
		t.Error("the archive is not in chronological order")
	}
	var envelopes int
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "From ") {
			envelopes++
		}
	}
	if envelopes != 3 {
		t.Errorf("archive holds %d messages, want 3", envelopes)
	}
}

func TestRunSearchContext_Cancellation(t *testing.T) {
	resetSearchFlags(t)
	mock := &mailclient.MockMailClient{
		SearchResults: map[string][]email.ID{"anything": {"m1", "m2"}},
		RawMessages: map[email.ID][]byte{
			"m1": searchMessage("first", day(1)),
			"m2": searchMessage("second", day(2)),
		},
	}
	session := searchSession(t, mock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	err := runSearchContext(ctx, session, "anything")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runSearchContext error = %v, want context.Canceled", err)
	}
}
