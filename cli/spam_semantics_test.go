package cli

import (
	"testing"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
)

// sessionWithMock builds a Session whose client is the supplied mock.
func sessionWithMock(mock *mailclient.MockMailClient, cfg *cfg_g.Config) *Session {
	mock.Cfg = cfg
	return &Session{
		Config: cfg,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mock, nil
				},
			},
		},
	}
}

// TestSpamLearn_ActuallyTrains guards against the regression where "spam learn"
// only called MoveAllSpam and trained nothing, despite its description.
func TestSpamLearn_ActuallyTrains(t *testing.T) {
	mock := &mailclient.MockMailClient{}
	session := sessionWithMock(mock, &cfg_g.Config{Policy: cfg_g.Policy{SpamLearn: "LearnSpam"}})

	if err := InitCLI(session).Execute([]string{"spam", "learn"}); err != nil {
		t.Fatalf("spam learn returned error: %v", err)
	}
	if mock.LearnSpamCalls != 1 {
		t.Errorf("expected LearnSpam to be called once, got %d", mock.LearnSpamCalls)
	}
	if mock.MoveAllSpamCalls != 1 {
		t.Errorf("expected MoveAllSpam to be called once, got %d", mock.MoveAllSpamCalls)
	}
	if mock.LastMoveAllSpamDest != "LearnSpam" {
		t.Errorf("expected default MoveAllSpam destination %q, got %q", "LearnSpam", mock.LastMoveAllSpamDest)
	}
}

// TestSpamLearn_CustomDestLabel guards against the regression where a positional
// folder argument was ignored and replaced with the configured default.
func TestSpamLearn_CustomDestLabel(t *testing.T) {
	mock := &mailclient.MockMailClient{}
	session := sessionWithMock(mock, &cfg_g.Config{Policy: cfg_g.Policy{SpamLearn: "DefaultSpam"}})

	if err := InitCLI(session).Execute([]string{"spam", "learn", "%CustomSpamFolder"}); err != nil {
		t.Fatalf("spam learn returned error: %v", err)
	}
	if mock.MoveAllSpamCalls != 1 {
		t.Errorf("expected MoveAllSpam to be called once, got %d", mock.MoveAllSpamCalls)
	}
	if mock.LastMoveAllSpamDest != "CustomSpamFolder" {
		t.Errorf("expected MoveAllSpam destination %q, got %q", "CustomSpamFolder", mock.LastMoveAllSpamDest)
	}
}

// TestSpamLearnHam_ResolvesLabelRef guards against the regression where --ham
// handed the backend the raw reference, so "%archive" named no folder while
// "mail_cli %archive" found it.
func TestSpamLearnHam_ResolvesLabelRef(t *testing.T) {
	mock := &mailclient.MockMailClient{Labels: []string{"Archive", "Archive/2024"}}
	session := sessionWithMock(mock, &cfg_g.Config{})
	var got string
	session.Deps.Spam = stubSpam{LearnHamFn: func(_ *cfg_g.Config, f label.Folder, _ bool) error {
		got = f.String()
		return nil
	}}

	if err := InitCLI(session).Execute([]string{"spam", "learn", "--ham", "%archive"}); err != nil {
		t.Fatalf("spam learn --ham returned error: %v", err)
	}
	if got != "Archive" {
		t.Errorf("LearnHam folder = %q, want %q", got, "Archive")
	}
}

// TestSpamPolUnsub_ActuallyUnsubscribes guards against the regression where
// "spam pol unsub" called MoveAllSpam instead of performing any opt-out — which
// could escalate to a permanent purge of the whole spam folder.
func TestSpamPolUnsub_ActuallyUnsubscribes(t *testing.T) {
	mock := &mailclient.MockMailClient{}
	session := sessionWithMock(mock, &cfg_g.Config{Policy: cfg_g.Policy{SpamLearn: "LearnSpam"}})

	if err := InitCLI(session).Execute([]string{"spam", "pol", "unsub"}); err != nil {
		t.Fatalf("spam pol unsub returned error: %v", err)
	}
	if mock.ShowPoliticalSpamCalls != 1 {
		t.Errorf("expected ShowPoliticalSpam to be called once, got %d", mock.ShowPoliticalSpamCalls)
	}
	if mock.MoveAllSpamCalls != 0 {
		t.Errorf("spam pol unsub must not move or purge the spam folder, got %d MoveAllSpam calls", mock.MoveAllSpamCalls)
	}
	if mock.DeleteAllSpamCalls != 0 {
		t.Errorf("spam pol unsub must never delete, got %d DeleteAllSpam calls", mock.DeleteAllSpamCalls)
	}
}

// TestSpamBye_ComposesAllThree pins the intended composition of "spam bye".
func TestSpamBye_ComposesAllThree(t *testing.T) {
	mock := &mailclient.MockMailClient{}
	session := sessionWithMock(mock, &cfg_g.Config{Policy: cfg_g.Policy{SpamLearn: "LearnSpam"}})

	if err := InitCLI(session).Execute([]string{"spam", "bye"}); err != nil {
		t.Fatalf("spam bye returned error: %v", err)
	}
	if mock.ShowPoliticalSpamCalls != 1 || mock.LearnSpamCalls != 1 || mock.MoveAllSpamCalls != 1 {
		t.Errorf("expected spam bye to call each of ShowPoliticalSpam/LearnSpam/MoveAllSpam once, got %d/%d/%d",
			mock.ShowPoliticalSpamCalls, mock.LearnSpamCalls, mock.MoveAllSpamCalls)
	}
}

// TestSpamPolAudit_DoesNotAct pins the distinction the two subcommands exist
// for. "audit" reports; "unsub" acts.
//
// The flag used to be called autoBlacklist and gated only blacklisting. The
// unsubscribe and delete paths consulted config.AutoUnsubscribe and
// config.ReadOnly instead, so with auto_unsubscribe enabled, a command whose
// help text promises to "print scoring details" would send opt-outs and
// permanently delete from the Spam folder.
func TestSpamPolAudit_DoesNotAct(t *testing.T) {
	mock := &mailclient.MockMailClient{}
	session := sessionWithMock(mock, &cfg_g.Config{Policy: cfg_g.Policy{SpamLearn: "LearnSpam"}})

	if err := InitCLI(session).Execute([]string{"spam", "pol", "audit"}); err != nil {
		t.Fatalf("spam pol audit returned error: %v", err)
	}
	if mock.ShowPoliticalSpamCalls != 1 {
		t.Errorf("expected ShowPoliticalSpam once, got %d", mock.ShowPoliticalSpamCalls)
	}
	if mock.PoliticalSpamActed {
		t.Error("spam pol audit asked the backend to act; an audit must only report")
	}
	if mock.MoveAllSpamCalls != 0 || mock.DeleteAllSpamCalls != 0 {
		t.Error("spam pol audit touched the spam folder")
	}
}

// The acting form must still act, or the guard has broken the feature.
func TestSpamPolUnsub_Acts(t *testing.T) {
	mock := &mailclient.MockMailClient{}
	session := sessionWithMock(mock, &cfg_g.Config{Policy: cfg_g.Policy{SpamLearn: "LearnSpam"}})

	if err := InitCLI(session).Execute([]string{"spam", "pol", "unsub"}); err != nil {
		t.Fatalf("spam pol unsub returned error: %v", err)
	}
	if !mock.PoliticalSpamActed {
		t.Error("spam pol unsub did not ask the backend to act")
	}
}

// Under --read-only, even the acting form must be downgraded to a report.
func TestSpamPolUnsub_ReadOnlyDoesNotAct(t *testing.T) {
	mock := &mailclient.MockMailClient{}
	ro := mailclient.NewReadOnlyMailClient(mock, "test")

	if err := ro.ShowPoliticalSpam(true); err != nil {
		t.Fatalf("ShowPoliticalSpam: %v", err)
	}
	if mock.PoliticalSpamActed {
		t.Error("--read-only passed the acting flag through to the backend")
	}
}

// TestSpamPrefixDoesNotInvokeDelete verifies that invoking "spam de" does not resolve
// via command abbreviation to "spam del" and does not delete all spam.
func TestSpamPrefixDoesNotInvokeDelete(t *testing.T) {
	mock := &mailclient.MockMailClient{}
	markedIDs := []string{}
	session := sessionWithMock(mock, &cfg_g.Config{})
	session.Deps.Spam = stubSpam{
		MarkByIDFn: func(_ *cfg_g.Config, id string) error {
			markedIDs = append(markedIDs, id)
			return nil
		},
	}

	if err := InitCLI(session).Execute([]string{"spam", "de"}); err != nil {
		t.Fatalf("unexpected error executing spam de: %v", err)
	}

	if mock.DeleteAllSpamCalls != 0 {
		t.Errorf("spam de must NOT invoke DeleteAllSpam; got %d calls", mock.DeleteAllSpamCalls)
	}
	if len(markedIDs) != 1 || markedIDs[0] != "de" {
		t.Errorf("expected 'de' to be treated as message ID to MarkSpam, got %v", markedIDs)
	}
}
