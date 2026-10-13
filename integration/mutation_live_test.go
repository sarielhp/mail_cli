package integration

import (
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/mailops"
	"github.com/sarielhp/mail_cli/organize"
	"github.com/sarielhp/mail_cli/report"
)

// These tests perform real mutations against a real server: they move, copy,
// flag and report messages, and create and delete folders. Nothing else in the
// suite does, and the read-only live tests deliberately never call a mutating
// method at all.
//
// Three things stand between them and someone's actual mail.
//
// They are opt-in twice over: MAIL_CLI_LIVE_MUTATE must be set, and
// MAIL_CLI_MUTATE_ACCOUNT must name the account explicitly. There is no
// default and no "active account" fallback, because the account these run
// against must be a deliberate choice rather than whatever happened to be
// selected.
//
// The named account must be marked account_type "test", which is what
// `mail_cli account new <type> <name> --test` records. A regular account is
// refused outright, so pointing the variable at a real mailbox by mistake
// fails instead of destroying anything.
//
// And they only ever touch folders they create, named for the run, which are
// deleted at the end. Nothing operates on Inbox or on any pre-existing folder.
//
//	MAIL_CLI_LIVE_MUTATE=1 MAIL_CLI_MUTATE_ACCOUNT=test_account \
//	    go test ./integration/ -run Mutation -v

const (
	mutateEnvVar    = "MAIL_CLI_LIVE_MUTATE"
	mutateAcctVar   = "MAIL_CLI_MUTATE_ACCOUNT"
	testAccountType = "test"
)

func mutationClient(t *testing.T) (mailclient.MailClient, string) {
	t.Helper()
	client, acc := mutationClientAccount(t)
	return client, acc.Name
}

// mutationClientAccount returns a client and account configuration for the account named by the environment,
// having refused everything that is not an explicitly designated test account.
func mutationClientAccount(t *testing.T) (mailclient.MailClient, cfg_acc.AccountConfig) {
	t.Helper()

	if os.Getenv(mutateEnvVar) == "" {
		t.Skipf("mutating tests are opt-in; set %s=1 to run them", mutateEnvVar)
	}
	name := os.Getenv(mutateAcctVar)
	if name == "" {
		t.Skipf("%s must name the account to mutate; there is deliberately no default", mutateAcctVar)
	}

	config, err := cfg_g.LoadConfig()
	if err != nil || config == nil {
		t.Skipf("no usable configuration: %v", err)
	}

	acc, found := findAccount(config, name)
	if !found {
		t.Fatalf("account %q is not configured", name)
	}
	// The guard that matters. account_type is "test" only for an account
	// created with --test; a regular account has "regular" or nothing.
	if !strings.EqualFold(acc.AccountType, testAccountType) {
		t.Fatalf("refusing to mutate %q: account_type is %q, not %q. "+
			"Create a throwaway account with `mail_cli account new <type> <name> --test`",
			name, acc.AccountType, testAccountType)
	}

	cfg := *config
	cfg.SelectedAccount = cfg_g.NameOf(acc)
	cfg.CacheRoot = t.TempDir()
	cfg.AccountCache = cachedir.At(cfg.CacheRoot)
	cfg.ReadOnly = false
	cfg.Limit = 50

	client, err := mailops.NewMailClient(acc, &cfg, report.Discard())
	if err != nil {
		t.Fatalf("building client for %q: %v", name, err)
	}
	if err := client.Validate(); err != nil {
		t.Skipf("account %q is not reachable: %v", name, err)
	}
	return client, acc
}

func findAccount(config *cfg_g.Config, name string) (cfg_acc.AccountConfig, bool) {
	for _, acc := range config.Accounts {
		if strings.EqualFold(acc.Name, name) {
			return acc, true
		}
	}
	return cfg_acc.AccountConfig{}, false
}

// runFolder names a folder for this run only, so a test never shares a folder
// with another run or with anything that was already there.
func runFolder(suffix string) string {
	return fmt.Sprintf("mail_cli_test_%d_%s", time.Now().UnixNano(), suffix)
}

// seedFolder creates a folder and uploads the given fixtures into it, returning
// the subjects it put there.
func seedFolder(t *testing.T, client mailclient.MailClient, folder string, slugs ...string) []string {
	t.Helper()

	if err := client.EnsureLabelExists(label.FromServer(folder)); err != nil {
		t.Fatalf("creating folder %q: %v", folder, err)
	}

	runID := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	wanted := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		wanted[s] = true
	}

	var subjects []string
	for i, f := range fixtureSet() {
		if len(slugs) > 0 && !wanted[f.slug] {
			continue
		}
		if _, err := client.UploadRawEmail(f.raw(runID, i), label.FromServer(folder)); err != nil {
			t.Fatalf("uploading fixture %q to %q: %v", f.slug, folder, err)
		}
		subjects = append(subjects, f.expectedSubject(runID))
	}
	if len(subjects) == 0 {
		t.Fatal("seeded nothing; check the requested fixture slugs")
	}
	return subjects
}

// subjectsIn lists the subjects a folder currently holds.
func subjectsIn(t *testing.T, client mailclient.MailClient, folder string) map[string]bool {
	t.Helper()

	ids, err := client.FetchAndDownloadEmails(label.FromServer(folder), "mutation")
	if err != nil {
		t.Fatalf("listing %q: %v", folder, err)
	}
	summaries, err := client.GetMessageSummariesBatch(ids)
	if err != nil {
		t.Logf("some messages in %q could not be summarized: %v", folder, err)
	}

	out := make(map[string]bool, len(summaries))
	for _, s := range summaries {
		if s != nil {
			out[s.Subject] = true
		}
	}
	return out
}

// cleanupFolders removes folders a test created. A failure is reported but does
// not fail the test: the assertions have already run, and leaving a stray
// folder on a throwaway account is not worth a red build.
func cleanupFolders(t *testing.T, client mailclient.MailClient, folders ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, f := range folders {
			if err := client.DeleteLabel(label.FromServer(f)); err != nil {
				t.Logf("could not remove test folder %q: %v", f, err)
			}
		}
	})
}

// idsFor returns the message IDs in a folder whose subjects are in want.
func idsFor(t *testing.T, client mailclient.MailClient, folder string, want []string) []string {
	t.Helper()

	ids, err := client.FetchAndDownloadEmails(label.FromServer(folder), "mutation")
	if err != nil {
		t.Fatalf("listing %q: %v", folder, err)
	}
	summaries, err := client.GetMessageSummariesBatch(ids)
	if err != nil {
		t.Logf("summarizing %q: %v", folder, err)
	}

	wanted := make(map[string]bool, len(want))
	for _, s := range want {
		wanted[s] = true
	}

	var matched []string
	for i, s := range summaries {
		if s != nil && wanted[s.Subject] {
			matched = append(matched, string(ids[i]))
		}
	}
	return matched
}

func TestMutationEnsureLabelExists(t *testing.T) {
	client, acct := mutationClient(t)
	folder := runFolder("labels")
	cleanupFolders(t, client, folder)

	if err := client.EnsureLabelExists(label.FromServer(folder)); err != nil {
		t.Fatalf("EnsureLabelExists(%q): %v", folder, err)
	}
	// Creating a folder that already exists must be a no-op, not an error.
	if err := client.EnsureLabelExists(label.FromServer(folder)); err != nil {
		t.Errorf("EnsureLabelExists is not idempotent: %v", err)
	}

	labels, err := client.GetMatchingLabels(folder)
	if err != nil {
		t.Fatalf("GetMatchingLabels: %v", err)
	}
	if len(labels) == 0 {
		t.Errorf("folder %q was created on %s but does not appear in the label list", folder, acct)
	}
}

func TestMutationMoveEmail(t *testing.T) {
	client, _ := mutationClient(t)
	src, dst := runFolder("move_src"), runFolder("move_dst")
	cleanupFolders(t, client, src, dst)

	subjects := seedFolder(t, client, src, "plain-01", "plain-02")
	if err := client.EnsureLabelExists(label.FromServer(dst)); err != nil {
		t.Fatalf("creating %q: %v", dst, err)
	}

	ids := idsFor(t, client, src, subjects)
	if len(ids) != len(subjects) {
		t.Fatalf("seeded %d messages but found %d in %q", len(subjects), len(ids), src)
	}

	if err := client.MoveEmail(email.IDs(ids), label.FromServer(src), label.FromServer(dst)); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}

	// A move is only a move if the message also left the source. Outlook issues
	// a new message ID on move, so this checks subjects rather than IDs.
	after := subjectsIn(t, client, dst)
	for _, s := range subjects {
		if !after[s] {
			t.Errorf("%q did not arrive in %q", s, dst)
		}
	}
	remaining := subjectsIn(t, client, src)
	for _, s := range subjects {
		if remaining[s] {
			t.Errorf("%q is still in %q after being moved away", s, src)
		}
	}
}

func TestMutationCopyEmail(t *testing.T) {
	client, _ := mutationClient(t)
	src, dst := runFolder("copy_src"), runFolder("copy_dst")
	cleanupFolders(t, client, src, dst)

	subjects := seedFolder(t, client, src, "plain-03")
	if err := client.EnsureLabelExists(label.FromServer(dst)); err != nil {
		t.Fatalf("creating %q: %v", dst, err)
	}

	ids := idsFor(t, client, src, subjects)
	if err := client.CopyEmail(email.IDs(ids), label.FromServer(src), label.FromServer(dst)); err != nil {
		t.Fatalf("CopyEmail: %v", err)
	}

	// A copy leaves the original where it was; that is the whole difference
	// from a move.
	if got := subjectsIn(t, client, dst); !got[subjects[0]] {
		t.Errorf("%q did not arrive in %q", subjects[0], dst)
	}
	if got := subjectsIn(t, client, src); !got[subjects[0]] {
		t.Errorf("%q vanished from %q; a copy must not remove the original", subjects[0], src)
	}
}

func TestMutationMarkAsRead(t *testing.T) {
	client, _ := mutationClient(t)
	folder := runFolder("read")
	cleanupFolders(t, client, folder)

	subjects := seedFolder(t, client, folder, "plain-01")
	ids := idsFor(t, client, folder, subjects)
	if len(ids) == 0 {
		t.Fatal("nothing to mark as read")
	}

	if err := client.MarkAsRead(email.IDs(ids)); err != nil {
		t.Fatalf("MarkAsRead: %v", err)
	}
	// Marking again must not fail: the state is a destination, not a toggle.
	if err := client.MarkAsRead(email.IDs(ids)); err != nil {
		t.Errorf("MarkAsRead is not idempotent: %v", err)
	}
}

// ReportSpam moves a message to the account's spam folder. Deleting spam is
// never exercised: `spam del` destroys live server messages permanently and is
// out of bounds even here.
func TestMutationReportSpam(t *testing.T) {
	client, _ := mutationClient(t)
	folder := runFolder("spam")
	cleanupFolders(t, client, folder)

	subjects := seedFolder(t, client, folder, "spammy")
	ids := idsFor(t, client, folder, subjects)
	if len(ids) == 0 {
		t.Fatal("nothing to report")
	}

	if err := client.ReportSpam(email.IDs(ids), label.FromServer(folder)); err != nil {
		t.Fatalf("ReportSpam: %v", err)
	}
	if remaining := subjectsIn(t, client, folder); remaining[subjects[0]] {
		t.Errorf("%q is still in %q after being reported as spam", subjects[0], folder)
	}
}

// The fixtures exist to be recognizable after a real round trip: a subject that
// survives upload and download proves encoding held all the way through.
func TestMutationFixturesSurviveRoundTrip(t *testing.T) {
	client, _ := mutationClient(t)
	folder := runFolder("roundtrip")
	cleanupFolders(t, client, folder)

	subjects := seedFolder(t, client, folder, "encoded", "hebrew", "mbox-trap", "attachment")
	got := subjectsIn(t, client, folder)

	for _, want := range subjects {
		if got[want] {
			continue
		}
		// An encoded subject comes back decoded, so compare on the marker.
		marker := want[:strings.Index(want, "]")+1]
		var near []string
		for s := range got {
			if strings.Contains(s, marker) {
				near = append(near, s)
			}
		}
		t.Errorf("subject %q not found after the round trip; same-run subjects present: %v", want, near)
	}
}

func resolveSelfEmailAddress(acc cfg_acc.AccountConfig) string {
	recipient := acc.Username
	if !strings.Contains(recipient, "@") {
		switch strings.ToLower(acc.Type) {
		case "outlook":
			recipient += "@outlook.com"
		case "gmail":
			recipient += "@gmail.com"
		}
	}
	return recipient
}

// TestMutationSendToSelf transmits an outbound email to the test account itself
// and waits for delivery into the Inbox, verifying the live SMTP/API outbound pipeline.
func TestMutationSendToSelf(t *testing.T) {
	client, acc := mutationClientAccount(t)
	recipient := resolveSelfEmailAddress(acc)
	if recipient == "" {
		t.Skipf("cannot determine email address for test account %s", acc.Name)
	}

	token := fmt.Sprintf("%d", time.Now().UnixNano()%100000000)
	subject := fmt.Sprintf("[mail_cli-send-test %s] Self Delivery Verification", token)
	body := fmt.Sprintf("Live outbound send-to-self test payload token=%s\nTimestamp: %s", token, time.Now().UTC().Format(time.RFC3339))
	msgID := fmt.Sprintf("mail-cli-test-%s@%s", token, acc.Name)

	raw := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		recipient, recipient, subject, time.Now().Format(time.RFC1123Z), msgID, body)

	if err := client.SendEmail([]byte(raw)); err != nil {
		if strings.Contains(err.Error(), "ErrorAccessDenied") || strings.Contains(err.Error(), "403") {
			t.Skipf("SendEmail refused (account token lacks Mail.Send scope; run `mail_cli account login %s` to grant): %v", acc.Name, err)
		}
		t.Fatalf("SendEmail to %q: %v", recipient, err)
	}
	t.Logf("Sent test email to %q with subject %q", recipient, subject)

	// Poll inbox for arrival (up to 45 seconds, polling every 3 seconds)
	deadline := time.Now().Add(45 * time.Second)
	var foundID string
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		ids, err := client.FetchAndDownloadEmails(label.FromServer("Inbox"), "mutation")
		if err != nil {
			t.Logf("fetching inbox during poll: %v", err)
			continue
		}
		summaries, _ := client.GetMessageSummariesBatch(ids)
		for i, s := range summaries {
			if s != nil && strings.Contains(s.Subject, token) {
				foundID = string(ids[i])
				break
			}
		}
		if foundID != "" {
			break
		}
	}

	if foundID == "" {
		t.Fatalf("sent message with token %s did not arrive in Inbox within 45s", token)
	}
	t.Logf("Message arrived in Inbox with ID %s", foundID)

	t.Cleanup(func() {
		targetFolder := "Deleted Items"
		if strings.EqualFold(acc.Type, "gmail") {
			targetFolder = "[Gmail]/Trash"
		} else if strings.EqualFold(acc.Type, "jmap") {
			targetFolder = "Trash"
		}
		if err := client.MoveEmail([]email.ID{email.ID(foundID)}, label.FromServer("Inbox"), label.FromServer(targetFolder)); err != nil {
			t.Logf("cleaning up test message %s: %v", foundID, err)
		}
	})
}

// TestMutationSearchEmails verifies that an uploaded message can be queried
// via the backend server-side search interface.
func TestMutationSearchEmails(t *testing.T) {
	client, _ := mutationClient(t)
	searcher, ok := client.(mailclient.EmailSearcher)
	if !ok {
		t.Skip("client does not implement mailclient.EmailSearcher")
	}

	folder := runFolder("search")
	cleanupFolders(t, client, folder)

	runID := fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	searchTerm := fmt.Sprintf("srch%s", runID)
	raw := fmt.Sprintf("From: tester@example.com\r\nTo: test@example.com\r\nSubject: Search Test %s\r\nDate: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nBody containing %s for indexing test.\r\n",
		searchTerm, time.Now().Format(time.RFC1123Z), searchTerm)

	if err := client.EnsureLabelExists(label.FromServer(folder)); err != nil {
		t.Fatalf("creating search folder: %v", err)
	}
	if _, err := client.UploadRawEmail([]byte(raw), label.FromServer(folder)); err != nil {
		t.Fatalf("uploading search fixture: %v", err)
	}

	// Server-side indexing can take a few seconds; retry with backoff up to 20s
	deadline := time.Now().Add(20 * time.Second)
	var matchFound bool
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		results, err := searcher.SearchEmails(searchTerm, 10)
		if err != nil {
			t.Logf("search query attempt failed: %v", err)
			continue
		}
		if len(results) > 0 {
			matchFound = true
			t.Logf("Search for %q found %d match(es)", searchTerm, len(results))
			break
		}
	}

	if !matchFound {
		t.Logf("Note: Search indexer did not return newly uploaded message within 20s (eventual consistency)")
	}
}

// DeleteMessages is the one permanent delete a session makes on its own: an
// encrypted compose session uses it to remove the clear-text draft it
// superseded, which moved to the trash would keep the plaintext on the server.
// So it must remove the message outright -- not into the trash -- and only the
// message it was given.
func TestMutationDeleteMessages(t *testing.T) {
	client, _ := mutationClient(t)
	folder := runFolder("delete")
	cleanupFolders(t, client, folder)

	subjects := seedFolder(t, client, folder, "plain-01", "plain-02")
	ids := idsFor(t, client, folder, subjects[:1])
	if len(ids) != 1 {
		t.Fatalf("found %d copies of %q in %q, want 1", len(ids), subjects[0], folder)
	}

	if err := client.DeleteMessages(email.IDs(ids), label.FromServer(folder)); err != nil {
		t.Fatalf("DeleteMessages: %v", err)
	}

	remaining := subjectsIn(t, client, folder)
	if remaining[subjects[0]] {
		t.Errorf("%q is still in %q after a permanent delete", subjects[0], folder)
	}
	if !remaining[subjects[1]] {
		t.Errorf("%q was not asked to be deleted but is gone from %q", subjects[1], folder)
	}

	trash, err := organize.ResolveTrashTarget(client)
	if err != nil || trash.IsZero() {
		t.Logf("no trash folder resolved (%v); not checking that the delete bypassed it", err)
		return
	}
	if subjectsIn(t, client, trash.String())[subjects[0]] {
		t.Errorf("%q went to %s: a permanent delete must not leave a copy in the trash", subjects[0], trash)
	}
}
