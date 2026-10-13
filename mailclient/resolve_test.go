package mailclient

import (
	"errors"
	"testing"

	"github.com/sarielhp/mail_cli/report"
)

// The failure this whole change exists to prevent. With folders `Work` and
// `Worklog`, the permissive resolver returned the string "wor" unchanged with a
// nil error, because the backend reported two matches and it only checked that
// the count was above zero. Gmail then matched no messages and did nothing;
// JMAP created a new empty mailbox literally named "wor" and filed messages
// into it. An ambiguous argument must fail, not resolve to something that names
// no folder.
func TestResolveOneRefusesToGuessBetweenSiblings(t *testing.T) {
	c := &MockMailClient{Labels: []string{"Work", "Worklog"}}

	got, err := ResolveOne(c, "wor")

	var ambiguous *AmbiguityError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("ResolveOne(%q) = (%q, %v); want an *AmbiguityError", "wor", got, err)
	}
	if got.String() != "" {
		t.Errorf("ResolveOne returned %q alongside an error; want the empty string", got)
	}
	if ambiguous.Input != "wor" {
		t.Errorf("AmbiguityError.Input = %q, want %q", ambiguous.Input, "wor")
	}
	if len(ambiguous.Candidates) != 2 {
		t.Errorf("AmbiguityError.Candidates = %v, want both siblings", ambiguous.Candidates)
	}
}

func TestResolveOne(t *testing.T) {
	labels := []string{"INBOX", "Work", "Worklog", "Work/ProjectA", "Archive/2025/Receipts"}

	tests := []struct {
		name  string
		input string
		want  string
		// wantErr is the error kind expected, or nil for success.
		wantErr any
	}{
		{name: "exact", input: "Work", want: "Work"},
		{name: "exact ignores case", input: "wORK", want: "Work"},
		{name: "exact beats the prefix it shares", input: "work", want: "Work"},
		{name: "unique path suffix", input: "ProjectA", want: "Work/ProjectA"},
		{name: "unique deep suffix", input: "Receipts", want: "Archive/2025/Receipts"},
		{name: "unique prefix", input: "Worklo", want: "Worklog"},
		{name: "unique substring", input: "2025", want: "Archive/2025/Receipts"},
		{name: "inbox maps to the backend's own name", input: "inbox", want: "INBOX"},
		{name: "empty resolves to empty", input: "", want: ""},
		{name: "ambiguous prefix", input: "wor", wantErr: &AmbiguityError{}},
		{name: "no match", input: "nowhere", wantErr: &NotFoundError{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &MockMailClient{Labels: labels, InboxFolderVal: "INBOX"}
			got, err := ResolveOne(c, tc.input)

			switch want := tc.wantErr.(type) {
			case nil:
				if err != nil {
					t.Fatalf("ResolveOne(%q) unexpected error: %v", tc.input, err)
				}
				if got.String() != tc.want {
					t.Errorf("ResolveOne(%q) = %q, want %q", tc.input, got, tc.want)
				}
			case *AmbiguityError:
				var target *AmbiguityError
				if !errors.As(err, &target) {
					t.Fatalf("ResolveOne(%q) = (%q, %v); want *AmbiguityError", tc.input, got, err)
				}
			case *NotFoundError:
				var target *NotFoundError
				if !errors.As(err, &target) {
					t.Fatalf("ResolveOne(%q) = (%q, %v); want *NotFoundError", tc.input, got, err)
				}
			default:
				t.Fatalf("unhandled want type %T", want)
			}
		})
	}
}

// Archiving and scanning fan out over a subtree, so they ask a different
// question: every folder under the prefix, not one folder. Asking the
// single-folder question made `archive all test` fail as ambiguous exactly when
// the command had something to do.
func TestResolveAllTakesTheSubtree(t *testing.T) {
	c := &MockMailClient{
		Labels:         []string{"test", "test/newsletter", "test/other", "testing", "Work"},
		InboxFolderVal: "INBOX",
	}

	got, err := ResolveAll(c, "test")
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}

	want := map[string]bool{"test": true, "test/newsletter": true, "test/other": true}
	if len(got) != len(want) {
		t.Fatalf("ResolveAll(%q) = %v, want exactly %v", "test", got, want)
	}
	for _, g := range got {
		if !want[g.String()] {
			t.Errorf("ResolveAll(%q) returned %q, which is not under the prefix", "test", g)
		}
	}
}

// "testing" is not under "test". Raw prefix matching says it is, which is what
// the backends' own GetMatchingLabels does; a subtree is delimited by "/".
func TestResolveAllDoesNotTakeASiblingSharingAPrefix(t *testing.T) {
	c := &MockMailClient{Labels: []string{"test/a", "testing"}, InboxFolderVal: "INBOX"}

	got, err := ResolveAll(c, "test")
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	for _, g := range got {
		if g.String() == "testing" {
			t.Errorf("ResolveAll(%q) = %v; %q is a sibling, not a child", "test", got, "testing")
		}
	}
}

func TestResolveAllReportsAnEmptySubtree(t *testing.T) {
	c := &MockMailClient{Labels: []string{"Work"}, InboxFolderVal: "INBOX"}

	var notFound *NotFoundError
	if _, err := ResolveAll(c, "nowhere"); !errors.As(err, &notFound) {
		t.Fatalf("ResolveAll on an unknown prefix returned %v; want *NotFoundError", err)
	}
}

// ResolveLabel narrates; ResolveOne does not. Both must give the same answer,
// which is the property that made three resolvers a defect.
func TestResolveLabelAgreesWithResolveOne(t *testing.T) {
	for _, input := range []string{"Work", "ProjectA", "inbox", "wor", "nowhere", ""} {
		c := &MockMailClient{
			Labels:         []string{"INBOX", "Work", "Worklog", "Work/ProjectA"},
			InboxFolderVal: "INBOX",
		}

		want, wantErr := ResolveOne(c, input)
		got, gotErr := ResolveLabel(report.Discard(), c, input)

		if got != want {
			t.Errorf("ResolveLabel(%q) = %q, ResolveOne = %q", input, got, want)
		}
		if (gotErr == nil) != (wantErr == nil) {
			t.Errorf("ResolveLabel(%q) error = %v, ResolveOne error = %v", input, gotErr, wantErr)
		}
	}
}
