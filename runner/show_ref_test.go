package runner

import (
	"bytes"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/show"
)

const refTestRaw = "From: a@example.com\r\nSubject: Hello\r\n\r\nbody\r\n"

func refTestClient(t *testing.T, cached []string, server map[email.ID][]byte) *mailclient.MockMailClient {
	t.Helper()
	dir := t.TempDir()
	for _, id := range cached {
		if err := msg.Store(cachedir.At(dir), email.ID(id), []byte(refTestRaw), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &cfg_g.Config{}
	cfg.CacheRoot = dir
	cfg.AccountCache = cachedir.At(cfg.CacheRoot)
	return &mailclient.MockMailClient{Cfg: cfg, RawMessages: server}
}

// `msg show <id>` must find a cached message by its short ID, typed in any
// case, in whichever account holds it.
func TestShowRef_ShortIDFromCache(t *testing.T) {
	empty := refTestClient(t, nil, nil)
	holder := refTestClient(t, []string{"full-id-1"}, map[email.ID][]byte{"full-id-1": []byte(refTestRaw)})
	short := strings.ToLower(string(email.ComputeShortID("full-id-1")))

	var out bytes.Buffer
	clients := []mailclient.MailClient{empty, holder}
	if err := showRefFromClients(show.Options{Raw: true}, &out, clients, short); err != nil {
		t.Fatalf("show by short ID: %v", err)
	}
	if out.String() != refTestRaw {
		t.Errorf("shown %q, want the raw message", out.String())
	}
}

// A short ID that nothing local can expand must not be sent to a server,
// which cannot resolve a hash; the error has to say what happened.
func TestShowRef_UncachedShortIDExplains(t *testing.T) {
	client := refTestClient(t, nil, nil)
	err := showRefFromClients(show.Options{}, &bytes.Buffer{}, []mailclient.MailClient{client}, "1234ABCD")
	if err == nil || !strings.Contains(err.Error(), "short ID") {
		t.Errorf("err = %v, want a short-ID explanation", err)
	}
}

// A full ID that is not cached is fetched from the server.
func TestShowRef_FullIDFromServer(t *testing.T) {
	client := refTestClient(t, nil, map[email.ID][]byte{"server-only-id": []byte(refTestRaw)})
	var out bytes.Buffer
	if err := showRefFromClients(show.Options{Raw: true}, &out, []mailclient.MailClient{client}, "server-only-id"); err != nil {
		t.Fatalf("show by full ID: %v", err)
	}
	if out.String() != refTestRaw {
		t.Errorf("shown %q", out.String())
	}
}

// The regression itself: a bare ID used to reach the label parser and be
// rejected for lacking the sigil.
func TestRunShowOnAccounts_BareIDIsNotALabel(t *testing.T) {
	err := RunShowOnAccounts(show.Options{}, &bytes.Buffer{}, &cfg_g.Config{}, "1234ABCD", "", nil)
	if err != nil && strings.Contains(err.Error(), "not a label reference") {
		t.Errorf("a message ID was parsed as a label: %v", err)
	}
}
