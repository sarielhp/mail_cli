package integration

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"net/mail"
	"os"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/mailops"
	"github.com/sarielhp/mail_cli/report"
)

// These tests drive the real backends against real accounts, which is the only
// way to exercise code that talks to a server: the download paths in
// backend/gmail, backend/jmap and backend/outlook cannot be reached by a unit
// test, and until now nothing covered them. tui_readonly_live_test.go is named
// "live" but wraps a MockMailClient and contacts nothing.
//
// Safety rests on three rules, and nothing here may break them.
//
// Live access goes through ReadOnlyMailClient and nothing else: it blocks or
// simulates every mutating method and passes only reads through. Every client
// built here is checked for that layer before it is used, and the check is
// structural, so a client that somehow lost the wrapper fails the test instead
// of reaching a server.
//
// No test here calls a mutating method at all, even one it expects to be
// refused. Whether mutations are blocked is settled in
// mailclient/readonly_test.go against a mock; asking the question of a live
// account means performing the mutation if the answer turns out to be no.
//
// The cache root is redirected to a temp directory, so the real cache is never
// touched either.
//
// They make network calls, so they are opt-in and never run by default:
//
//	MAIL_CLI_LIVE=1 go test ./integration/ -run Live -v

const liveEnvVar = "MAIL_CLI_LIVE"

// liveConfig loads the real configuration, skipping unless live testing was
// explicitly requested and an account actually exists.
func liveConfig(t *testing.T) *cfg_g.Config {
	t.Helper()
	if os.Getenv(liveEnvVar) == "" {
		t.Skipf("live tests are opt-in; set %s=1 to run them", liveEnvVar)
	}
	config, err := cfg_g.LoadConfig()
	if err != nil {
		t.Skipf("no usable configuration: %v", err)
	}
	if config == nil || len(config.Accounts) == 0 {
		t.Skip("no accounts configured")
	}
	return config
}

// readOnlyClientFor builds a real backend client for one account, forced into
// read-only mode and pointed at a throwaway cache directory.
func readOnlyClientFor(t *testing.T, base *cfg_g.Config, acc cfg_acc.AccountConfig) mailclient.MailClient {
	t.Helper()

	cfg := *base
	cfg.ReadOnly = true
	cfg.CacheRoot = t.TempDir()
	cfg.AccountCache = cachedir.At(cfg.CacheRoot)
	// Selecting the account under test keeps its cache and config lookups
	// pointed at the right place.
	//
	// It used to be load-bearing for a worse reason: the factory gave an
	// unselected account a no-op cache store, and CheckingMailClient read that
	// as "inactive" and returned nothing from every fetch -- so this test would
	// have passed while downloading nothing at all. That short-circuit is gone;
	// every account now gets a real store in its own directory.
	cfg.SelectedAccount = cfg_g.NameOf(acc)
	cfg.Limit = 5

	accCopy := acc
	accCopy.ReadOnly = true

	client, err := mailops.NewMailClient(accCopy, &cfg, report.Discard())
	if err != nil {
		t.Fatalf("building client for %q: %v", acc.Name, err)
	}
	assertReadOnlyLayerPresent(t, client)
	return client
}

// assertReadOnlyLayerPresent fails the test unless the client really is wrapped
// in the read-only decorator.
//
// This is checked structurally, by walking the decorator chain, rather than by
// calling a mutating method and expecting it to be refused. Probing behavior
// would mean invoking DeleteAllSpam or SendEmail against a live account on the
// chance that the wrapper is missing -- and if it were missing, the probe is
// the damage. Blocking semantics belong in mailclient/readonly_test.go, where a
// mock absorbs the call; here the only safe question is whether the layer is
// there, asked before any network call is made.
func assertReadOnlyLayerPresent(t *testing.T, client mailclient.MailClient) {
	t.Helper()

	checking, ok := client.(*mailclient.CheckingMailClient)
	if !ok {
		t.Fatalf("client is %T, not the expected CheckingMailClient wrapper", client)
	}
	if _, ok := checking.Delegate.(*mailclient.ReadOnlyMailClient); !ok {
		t.Fatalf("client delegate is %T, not ReadOnlyMailClient: refusing to touch a live account without the read-only layer", checking.Delegate)
	}
	if cfg := client.Config(); cfg == nil || !cfg.ReadOnly {
		t.Fatal("client config is not marked read-only")
	}
}

// TestLiveFolderDownload exercises FetchAndDownloadEmails end to end against a
// real server: it is the path that now runs through msgstore instead of each
// backend's own worker pool.
func TestLiveFolderDownload(t *testing.T) {
	config := liveConfig(t)

	for _, acc := range config.Accounts {
		t.Run(acc.Name, func(t *testing.T) {
			client := readOnlyClientFor(t, config, acc)
			if err := client.Validate(); err != nil {
				t.Skipf("account %q not reachable: %v", acc.Name, err)
			}

			folder, ids, err := firstNonEmptyFolder(client)
			if err != nil {
				t.Fatalf("fetching a folder on %s: %v", acc.Name, err)
			}
			if len(ids) == 0 {
				t.Skipf("no folder on %s holds any message", acc.Name)
			}
			t.Logf("%s: %d message(s) in %q", acc.Name, len(ids), folder)

			assertCachedAndParsable(t, client.Config().AccountCache.Path(), ids)
		})
	}
}

// TestLiveAccountLatest exercises the account-wide query, the other path that
// had its own duplicated download loop in every backend.
func TestLiveAccountLatest(t *testing.T) {
	config := liveConfig(t)

	for _, acc := range config.Accounts {
		t.Run(acc.Name, func(t *testing.T) {
			client := readOnlyClientFor(t, config, acc)
			if err := client.Validate(); err != nil {
				t.Skipf("account %q not reachable: %v", acc.Name, err)
			}

			refs, err := client.FetchLatestAccountEmails(5)
			if err != nil {
				t.Fatalf("FetchLatestAccountEmails: %v", err)
			}
			if len(refs) == 0 {
				t.Skipf("no recent messages on %s", acc.Name)
			}

			ids := make([]string, 0, len(refs))
			for _, ref := range refs {
				if ref.MessageID == "" {
					t.Error("a message reference has an empty ID")
					continue
				}
				if ref.Folder == "" {
					t.Errorf("message %s has no folder", ref.MessageID)
				}
				ids = append(ids, string(ref.MessageID))
			}
			t.Logf("%s: %d recent message(s)", acc.Name, len(ids))

			assertCachedAndParsable(t, client.Config().AccountCache.Path(), ids)
		})
	}
}

// firstNonEmptyFolder downloads the inbox, or the first other folder holding
// anything if the inbox is empty.
//
// An account whose inbox happens to be empty would otherwise skip, leaving the
// folder download path unexercised on that backend and quietly reporting
// success -- which is what happened to the Outlook account.
func firstNonEmptyFolder(client mailclient.MailClient) (string, []string, error) {
	inbox := client.InboxFolder()
	ids, err := client.FetchAndDownloadEmails(inbox, "inbox")
	if err != nil {
		return "", nil, fmt.Errorf("fetching %q: %w", inbox, err)
	}
	if len(ids) > 0 {
		return inbox.String(), email.IDStrings(ids), nil
	}

	items, err := client.GetLabelItems()
	if err != nil {
		return inbox.String(), nil, nil
	}
	for _, item := range items {
		name := item.FullName
		if name == "" {
			name = item.Name
		}
		if name == "" || strings.EqualFold(name, inbox.String()) {
			continue
		}
		ids, err := client.FetchAndDownloadEmails(label.FromServer(name), "live")
		if err != nil {
			continue
		}
		if len(ids) > 0 {
			return name, email.IDStrings(ids), nil
		}
	}
	return inbox.String(), nil, nil
}

// assertCachedAndParsable checks that the download actually populated the cache
// and that what landed there is a real message. A store that silently wrote
// nothing, or wrote a truncated body, would otherwise look like success.
func assertCachedAndParsable(t *testing.T, downloadDir string, ids []string) {
	t.Helper()

	var cached, parsed int
	var absent []string
	for _, id := range ids {
		exists, err := msg.Exists(cachedir.At(downloadDir), email.ID(id))
		if err != nil || !exists {
			absent = append(absent, fmt.Sprintf("%s (err=%v)", id, err))
			continue
		}
		cached++

		raw, err := msg.Read(cachedir.At(downloadDir), email.ID(id))
		if err != nil {
			t.Errorf("message %s is indexed but unreadable: %v", id, err)
			continue
		}
		if len(raw) == 0 {
			t.Errorf("message %s was cached empty", id)
			continue
		}
		if _, err := mail.ReadMessage(bytes.NewReader(raw)); err != nil {
			t.Errorf("message %s does not parse as RFC 5322: %v", id, err)
			continue
		}
		parsed++

		// The store files a message under its own Date header, so a cached
		// message must carry a date the index agrees with.
		if info, err := msg.GetInfo(cachedir.At(downloadDir), email.ID(id)); err == nil && info.Date.IsZero() {
			t.Errorf("message %s cached with a zero date", id)
		}
	}

	if len(absent) > 0 {
		t.Logf("%d message(s) not in cache: %s", len(absent), strings.Join(absent, ", "))
	}
	if cached == 0 {
		t.Fatalf("none of the %d returned message(s) reached the cache", len(ids))
	}
	if parsed != cached {
		t.Errorf("%d of %d cached message(s) parsed", parsed, cached)
	}
	t.Logf("cached %d/%d, all parsable", cached, len(ids))
}

// TestLiveMessageAccessorColdCache is the point of the read-through cache: it
// asks a real server for a message by ID alone, with nothing cached and without
// naming the folder that holds it.
//
// It deliberately builds a second client with its own empty cache directory, so
// the message it asks for is known to be absent. A test that reused the warm
// cache from the query above would pass without the download path ever running.
func TestLiveMessageAccessorColdCache(t *testing.T) {
	config := liveConfig(t)

	for _, acc := range config.Accounts {
		t.Run(acc.Name, func(t *testing.T) {
			discovery := readOnlyClientFor(t, config, acc)
			if err := discovery.Validate(); err != nil {
				t.Skipf("account %q not reachable: %v", acc.Name, err)
			}

			refs, err := discovery.FetchLatestAccountEmails(3)
			if err != nil || len(refs) == 0 {
				t.Skipf("no recent messages on %s to address by ID: %v", acc.Name, err)
			}
			targetID := refs[0].MessageID

			// A fresh client, so the cache cannot already hold the message.
			cold := readOnlyClientFor(t, config, acc)
			coldDir := cold.Config().AccountCache
			if exists, _ := msg.Exists(coldDir, targetID); exists {
				t.Fatalf("cache at %s already holds %s; the cold-cache premise is broken", coldDir, targetID)
			}

			raw, err := cold.GetRawMessage(targetID)
			if err != nil {
				t.Fatalf("GetRawMessage(%s) on a cold cache: %v", targetID, err)
			}
			if _, parseErr := mail.ReadMessage(bytes.NewReader(raw)); parseErr != nil {
				t.Fatalf("message %s does not parse as RFC 5322: %v", targetID, parseErr)
			}

			// The fetch must have populated the cache, or nothing was cached
			// through and every later read pays for the network again.
			if exists, _ := msg.Exists(coldDir, targetID); !exists {
				t.Error("GetRawMessage returned the message but did not cache it")
			}

			summary, err := cold.GetMessageSummary(targetID)
			if err != nil {
				t.Fatalf("GetMessageSummary(%s): %v", targetID, err)
			}
			if summary.Subject == "" {
				t.Error("summary has an empty subject")
			}
			if summary.EmailDate.IsZero() {
				t.Error("summary has a zero date")
			}
			t.Logf("%s: fetched %s by ID alone (%d bytes, subject %q)",
				acc.Name, targetID, len(raw), summary.Subject)
		})
	}
}
