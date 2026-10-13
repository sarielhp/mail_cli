package mailclient

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"testing"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_g"
)

func contains(ids []email.ID, want email.ID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func newChecking(t *testing.T, cfg *cfg_g.Config, active bool) (*CheckingMailClient, *MockMailClient) {
	t.Helper()
	mock := &MockMailClient{Cfg: cfg, InboxFolderVal: "INBOX"}
	var store cache.Store = &cache.NoOpCacheStore{}
	if active {
		store = &cache.DiskCacheStore{Dir: cachedir.At(cfg.AccountCache.Path())}
	}
	return NewCheckingMailClient(mock, "acct", store), mock
}

// TestMoveEmailReindexesCache is the regression guard for cli/splice.go, which
// moved messages on the server and never touched the local folder index, so
// listings kept showing messages that had been moved away.
func TestMoveEmailReindexesCache(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}
	client, _ := newChecking(t, cfg, true)

	if err := folderindex.ReplaceAll(cachedir.At(dir), "src", []email.ID{"m1", "m2"}); err != nil {
		t.Fatalf("seeding source index: %v", err)
	}

	if err := client.MoveEmail([]email.ID{"m1"}, label.FromServer("src"), label.FromServer("dst")); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}

	srcIDs, _ := folderindex.IDs(cachedir.At(dir), "src")
	dstIDs, _ := folderindex.IDs(cachedir.At(dir), "dst")
	if contains(srcIDs, "m1") {
		t.Errorf("m1 still listed under the source folder: %v", srcIDs)
	}
	if !contains(srcIDs, "m2") {
		t.Errorf("m2 should be untouched, source is now %v", srcIDs)
	}
	if !contains(dstIDs, "m1") {
		t.Errorf("m1 missing from the destination folder: %v", dstIDs)
	}
}

// TestMoveEmailDoesNotReindexInReadOnly matters because the read-only decorator
// sits inside this one: the delegate returns nil having changed nothing, so
// reconciling would record a move that never happened.
func TestMoveEmailDoesNotReindexInReadOnly(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{ReadOnly: true, Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}
	mock := &MockMailClient{Cfg: cfg, InboxFolderVal: "INBOX"}
	client := NewCheckingMailClient(
		NewReadOnlyMailClient(mock, "acct"),
		"acct",
		&cache.DiskCacheStore{Dir: cachedir.At(dir)},
	)

	if err := folderindex.ReplaceAll(cachedir.At(dir), "src", []email.ID{"m1"}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := client.MoveEmail([]email.ID{"m1"}, label.FromServer("src"), label.FromServer("dst")); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}

	srcIDs, _ := folderindex.IDs(cachedir.At(dir), "src")
	if !contains(srcIDs, "m1") {
		t.Errorf("read-only run reindexed the cache; source is now %v", srcIDs)
	}
	if dstIDs, _ := folderindex.IDs(cachedir.At(dir), "dst"); contains(dstIDs, "m1") {
		t.Errorf("read-only run created a destination index entry: %v", dstIDs)
	}
}

func TestCopyEmailReindexesCache(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}
	client, _ := newChecking(t, cfg, true)

	if err := folderindex.ReplaceAll(cachedir.At(dir), "src", []email.ID{"m1", "m2"}); err != nil {
		t.Fatalf("seeding source index: %v", err)
	}

	if err := client.CopyEmail([]email.ID{"m1"}, label.FromServer("src"), label.FromServer("dst")); err != nil {
		t.Fatalf("CopyEmail: %v", err)
	}

	srcIDs, _ := folderindex.IDs(cachedir.At(dir), "src")
	dstIDs, _ := folderindex.IDs(cachedir.At(dir), "dst")
	if !contains(srcIDs, "m1") {
		t.Errorf("m1 should remain in source folder: %v", srcIDs)
	}
	if !contains(srcIDs, "m2") {
		t.Errorf("m2 should remain in source folder: %v", srcIDs)
	}
	if !contains(dstIDs, "m1") {
		t.Errorf("m1 missing from destination folder: %v", dstIDs)
	}
}

func TestCopyEmailDoesNotReindexInReadOnly(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{ReadOnly: true, Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}
	mock := &MockMailClient{Cfg: cfg, InboxFolderVal: "INBOX"}
	client := NewCheckingMailClient(
		NewReadOnlyMailClient(mock, "acct"),
		"acct",
		&cache.DiskCacheStore{Dir: cachedir.At(dir)},
	)

	if err := folderindex.ReplaceAll(cachedir.At(dir), "src", []email.ID{"m1"}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := client.CopyEmail([]email.ID{"m1"}, label.FromServer("src"), label.FromServer("dst")); err != nil {
		t.Fatalf("CopyEmail: %v", err)
	}

	if dstIDs, _ := folderindex.IDs(cachedir.At(dir), "dst"); contains(dstIDs, "m1") {
		t.Errorf("read-only run created a destination index entry: %v", dstIDs)
	}
}

// The cache store no longer decides whether the client talks to the server.
//
// A store that does not write to disk used to make IsInactive() true, and every
// fetch returned (nil, nil) -- an empty folder, with no error to distinguish it
// from a real one. A non-selected account got such a store, so working on one
// showed every folder as empty and reported success. "Do not write to disk" and
// "do not talk to the server" are different instructions.
func TestFetchesAreNotGatedOnTheCacheStore(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}
	client, mock := newChecking(t, cfg, false) // the store writes nothing
	mock.DownloadedIDs = map[string][]email.ID{"src": {"m1", "m2"}}

	ids, err := client.FetchAndDownloadEmails(label.FromServer("src"), "src")
	if err != nil {
		t.Fatalf("FetchAndDownloadEmails: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("got %v, want the delegate's two messages; an inert cache store must not empty the folder", ids)
	}
}

// EnsureLabelExists was gated the same way, so the TUI's compose flow uploaded
// a draft into a "Drafts" folder it had just declined to create.
func TestEnsureLabelExistsIsNotGatedOnTheCacheStore(t *testing.T) {
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: t.TempDir()}, AccountCache: cachedir.At(t.TempDir())}
	client, mock := newChecking(t, cfg, false)

	// The mock ignores the label name, so reaching the delegate is what is
	// asserted: an injected failure can only surface if the call got there.
	mock.FailOn = map[string]error{"EnsureLabelExists": errTestMoveFailed}

	if err := client.EnsureLabelExists(label.FromServer("Drafts")); err == nil {
		t.Error("the call never reached the delegate; an inert cache store must not skip label creation")
	}
}

// TestMoveEmailFailureLeavesCacheAlone: a failed server move must not be
// recorded locally.
func TestMoveEmailFailureLeavesCacheAlone(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}
	client, mock := newChecking(t, cfg, true)
	mock.Err = errTestMoveFailed

	if err := folderindex.ReplaceAll(cachedir.At(dir), "src", []email.ID{"m1"}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := client.MoveEmail([]email.ID{"m1"}, label.FromServer("src"), label.FromServer("dst")); err == nil {
		t.Fatal("expected the move to fail")
	}
	srcIDs, _ := folderindex.IDs(cachedir.At(dir), "src")
	if !contains(srcIDs, "m1") {
		t.Errorf("failed move was reconciled anyway; source is now %v", srcIDs)
	}
}

var errTestMoveFailed = errTest("simulated move failure")

type errTest string

func (e errTest) Error() string { return string(e) }

// A batch move that partly fails must still reconcile what moved.
//
// JMAP's Email/set answers per message, so a request for several can move most
// and refuse a few. The refusal used to be flattened into one error, every
// caller read that as "nothing happened", and the messages that did move stayed
// listed in the folder they had left until something else refreshed the index.
func TestMoveEmailReconcilesThePartOfTheBatchThatMoved(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}
	client, mock := newChecking(t, cfg, true)

	if err := folderindex.ReplaceAll(cachedir.At(dir), "src", []email.ID{"m1", "m2", "m3"}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	mock.Err = &PartialMoveError{Failed: []email.ID{"m2"}, Err: errTestMoveFailed}

	err := client.MoveEmail([]email.ID{"m1", "m2", "m3"}, label.FromServer("src"), label.FromServer("dst"))
	if err == nil {
		t.Fatal("a partly failed move must still report the failure")
	}

	dstIDs, _ := folderindex.IDs(cachedir.At(dir), "dst")
	for _, id := range []string{"m1", "m3"} {
		if !contains(dstIDs, email.ID(id)) {
			t.Errorf("%s moved on the server but is missing from the destination index %v", id, dstIDs)
		}
	}
	if contains(dstIDs, "m2") {
		t.Errorf("m2 was refused by the server but appears in the destination index %v", dstIDs)
	}

	srcIDs, _ := folderindex.IDs(cachedir.At(dir), "src")
	if !contains(srcIDs, "m2") {
		t.Errorf("m2 was refused and must stay in the source index, which holds %v", srcIDs)
	}
}

// A backend that cannot report per-message outcomes gets the old, safe
// behaviour: an error means nothing is assumed to have moved, because the
// request may never have reached the server.
func TestMoveEmailAssumesNothingMovedWithoutAPartialReport(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: dir}, AccountCache: cachedir.At(dir)}
	client, mock := newChecking(t, cfg, true)

	if err := folderindex.ReplaceAll(cachedir.At(dir), "src", []email.ID{"m1", "m2"}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	mock.Err = errTestMoveFailed

	if err := client.MoveEmail([]email.ID{"m1", "m2"}, label.FromServer("src"), label.FromServer("dst")); err == nil {
		t.Fatal("expected the move to fail")
	}
	if dstIDs, _ := folderindex.IDs(cachedir.At(dir), "dst"); len(dstIDs) != 0 {
		t.Errorf("an opaque failure reconciled %v; nothing should have moved", dstIDs)
	}
}
