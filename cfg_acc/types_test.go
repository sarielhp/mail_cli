package cfg_acc

import "testing"

// MatchRules decides where a message is filed, so a wrong match moves mail the
// user did not ask to move. The 2026-09-01 review flagged domain matching as a
// place false positives come from.

func TestMatchRulesBySender(t *testing.T) {
	rules := []Rule{
		{Sender: "alice@example.com", Label: "Alice"},
		{Sender: "@lists.example.org", Label: "Lists"},
		{Sender: "example.net", Label: "Net"},
	}

	tests := []struct {
		name   string
		sender string
		want   string
	}{
		{name: "exact address", sender: "alice@example.com", want: "Alice"},
		{name: "case insensitive", sender: "ALICE@EXAMPLE.COM", want: "Alice"},
		{name: "surrounding space", sender: "  alice@example.com  ", want: "Alice"},
		{name: "domain rule beginning with @", sender: "someone@lists.example.org", want: "Lists"},
		{name: "bare domain rule", sender: "bob@example.net", want: "Net"},
		{name: "bare domain rule matches a subdomain", sender: "bob@mail.example.net", want: "Net"},
		{name: "no rule matches", sender: "stranger@nowhere.test", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchRules(rules, tc.sender, "")
			if tc.want == "" {
				if got != nil {
					t.Errorf("matched %q, want no match", got.Label)
				}
				return
			}
			if got == nil || got.Label != tc.want {
				t.Errorf("got %v, want the %q rule", got, tc.want)
			}
		})
	}
}

// A domain rule must not match an address that merely ends with those
// characters -- "example.net" must not catch "notexample.net".
func TestMatchRulesDomainDoesNotMatchASuffixOfAnotherDomain(t *testing.T) {
	rules := []Rule{{Sender: "example.net", Label: "Net"}}

	for _, sender := range []string{
		"bob@notexample.net",
		"bob@myexample.net",
	} {
		if got := MatchRules(rules, sender, ""); got != nil {
			t.Errorf("%q matched the example.net rule; a domain must match on a boundary", sender)
		}
	}
}

func TestMatchRulesBySubjectPrefix(t *testing.T) {
	rules := []Rule{{Subject: "Invoice", Label: "Billing"}}

	tests := []struct {
		name    string
		subject string
		want    bool
	}{
		{"exact prefix", "Invoice 123", true},
		{"case insensitive", "invoice 123", true},
		{"whole subject", "Invoice", true},
		{"not at the start", "Your Invoice 123", false},
		{"unrelated", "Newsletter", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchRules(rules, "anyone@example.com", tc.subject)
			if (got != nil) != tc.want {
				t.Errorf("MatchRules(subject=%q) = %v, want match=%v", tc.subject, got, tc.want)
			}
		})
	}
}

// A rule carrying a subject is matched on the subject alone; its sender is not
// consulted. Two rules that both could match resolve in order.
func TestMatchRulesOrderAndPrecedence(t *testing.T) {
	rules := []Rule{
		{Subject: "Invoice", Label: "First"},
		{Subject: "Invoice", Label: "Second"},
	}
	if got := MatchRules(rules, "", "Invoice 1"); got == nil || got.Label != "First" {
		t.Errorf("got %v, want the first matching rule", got)
	}

	subjectThenSender := []Rule{
		{Subject: "Report", Label: "BySubject"},
		{Sender: "alice@example.com", Label: "BySender"},
	}
	if got := MatchRules(subjectThenSender, "alice@example.com", "Report Q3"); got == nil || got.Label != "BySubject" {
		t.Errorf("got %v, want the subject rule that appears first", got)
	}
	if got := MatchRules(subjectThenSender, "alice@example.com", "Something else"); got == nil || got.Label != "BySender" {
		t.Errorf("got %v, want the sender rule when the subject does not match", got)
	}
}

func TestMatchRulesEmptyInputs(t *testing.T) {
	if got := MatchRules(nil, "a@b.c", "subject"); got != nil {
		t.Errorf("got %v, want no match against no rules", got)
	}
	// A rule with neither criterion can never match, or it would swallow
	// everything that reached it.
	if got := MatchRules([]Rule{{Label: "Catchall"}}, "a@b.c", "subject"); got != nil {
		t.Errorf("a rule with no criteria matched %v", got)
	}
}

func TestGetDisplayName(t *testing.T) {
	tests := []struct {
		name string
		acc  AccountConfig
		want string
	}{
		{name: "display name wins", acc: AccountConfig{Name: "acc1", DisplayName: "Work"}, want: "Work"},
		{name: "falls back to the name", acc: AccountConfig{Name: "acc1"}, want: "acc1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.acc.GetDisplayName(); got != tc.want {
				t.Errorf("GetDisplayName = %q, want %q", got, tc.want)
			}
		})
	}
}

// The sender name is the person, not the account label. Reading DisplayName
// as the sender is how every message went out as "FastMail".
func TestGetSenderName(t *testing.T) {
	tests := []struct {
		name string
		acc  AccountConfig
		want string
	}{
		{name: "user name is the sender", acc: AccountConfig{Name: "work", DisplayName: "FastMail", UserName: "Sariel Har-Peled"}, want: "Sariel Har-Peled"},
		{name: "the account label is never the sender", acc: AccountConfig{Name: "work", DisplayName: "FastMail"}, want: ""},
		{name: "unset everywhere", acc: AccountConfig{Name: "work"}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.acc.GetSenderName(); got != tc.want {
				t.Errorf("GetSenderName = %q, want %q", got, tc.want)
			}
		})
	}
}

// The send address may differ from the login username: Fastmail authenticates
// as one address and writes from an alias.
func TestGetSendAddress(t *testing.T) {
	tests := []struct {
		name string
		acc  AccountConfig
		want string
	}{
		{name: "user email wins", acc: AccountConfig{Username: "sarielhp@sarielhp.org", UserEmail: "sariel@sarielhp.org"}, want: "sariel@sarielhp.org"},
		{name: "falls back to the login", acc: AccountConfig{Username: "sarielhp@sarielhp.org"}, want: "sarielhp@sarielhp.org"},
		{name: "unset everywhere", acc: AccountConfig{}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.acc.GetSendAddress(); got != tc.want {
				t.Errorf("GetSendAddress = %q, want %q", got, tc.want)
			}
		})
	}
}

// assertBoolCase reports a mismatch between what check returns for input and the
// expected want. MatchesName and IsFolderReadOnly are the two boolean predicates
// run over a table of inputs.
func assertBoolCase(t *testing.T, input string, want bool, check func(string) bool) {
	t.Helper()
	if got := check(input); got != want {
		t.Errorf("%q = %v, want %v", input, got, want)
	}
}

// An account is addressed by either of its names, case-insensitively, which is
// what -A accepts.
func TestMatchesName(t *testing.T) {
	acc := AccountConfig{Name: "work-jmap", DisplayName: "FastMail"}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "exact name", input: "work-jmap", want: true},
		{name: "different case", input: "WORK-JMAP", want: true},
		{name: "display name", input: "FastMail", want: true},
		{name: "display name, different case", input: "fastmail", want: true},
		{name: "unrelated", input: "gmail", want: false},
		{name: "empty", input: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertBoolCase(t, tc.input, tc.want, acc.MatchesName)
		})
	}
}

func TestIsFolderReadOnly(t *testing.T) {
	accReadOnly := AccountConfig{
		Name:     "archived-acc",
		ReadOnly: true,
	}
	if !accReadOnly.IsFolderReadOnly("INBOX") {
		t.Errorf("expected true when account is ReadOnly")
	}

	accWithFolders := AccountConfig{
		Name:            "work",
		ReadOnly:        false,
		ReadOnlyFolders: []string{"Archive", "Receipts", "Sent"},
	}
	tests := []struct {
		folder string
		want   bool
	}{
		{folder: "Archive", want: true},
		{folder: "archive", want: true},
		{folder: "RECEIPTS", want: true},
		{folder: "INBOX", want: false},
		{folder: "Drafts", want: false},
		{folder: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.folder, func(t *testing.T) {
			assertBoolCase(t, tc.folder, tc.want, accWithFolders.IsFolderReadOnly)
		})
	}
}
