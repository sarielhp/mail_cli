package mailclient

import (
	"fmt"
	"strings"

	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"
)

// AmbiguityError reports that a label argument matched more than one folder.
//
// It is a typed error rather than a formatted string because the candidates are
// data: the CLI colours them, the TUI could list them, and a test can assert on
// them. It carries no ANSI, for the same reason the report package exists --
// presentation belongs at the edge.
type AmbiguityError struct {
	Input      string
	Candidates []string
}

func (e *AmbiguityError) Error() string {
	return fmt.Sprintf("ambiguous label %q (candidates: %s)", e.Input, strings.Join(e.Candidates, ", "))
}

// NotFoundError reports that a label argument matched no folder at all.
type NotFoundError struct {
	Input string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no label matches %q", e.Input)
}

// ResolveOne turns a label argument into exactly one folder name, or fails.
//
// This is the only label resolver. There used to be three, and which one ran
// depended on which command you typed: `scan`, `show`, `archive` and `rule add`
// got a permissive one that returned the user's input unchanged the moment the
// server reported any match at all, with a nil error and no uniqueness check.
// With folders `Work` and `Worklog`, `mail_cli scan wor` handed the string
// "wor" to the backend -- where Gmail matched zero messages and did nothing,
// and JMAP created a new empty mailbox literally named "wor" and filed messages
// into it. Guessing is worse than failing, so this returns a typed error where
// the old one returned a guess.
//
// The order is most-specific first: exact, then a unique "/"-suffix or prefix
// match, then a unique substring match. Every step that finds more than one
// candidate stops with an AmbiguityError rather than picking.
//
// The result is a label.Folder because every name it returns is one the server
// listed; this is where a typed argument becomes something a backend accepts.
// An empty input resolves to the zero Folder.
func ResolveOne(c LabelReader, input string) (label.Folder, error) {
	if input == "" {
		return label.Folder{}, nil
	}

	name := input
	if strings.EqualFold(name, "inbox") {
		name = c.InboxFolder().String()
	}

	if match, found, err := matchDirect(c, name); found || err != nil {
		return label.FromServer(match), err
	}

	all, err := c.GetMatchingLabels("")
	if err != nil {
		return label.Folder{}, err
	}

	match, candidates := label.MatchOne(all, name)
	switch {
	case match != "":
		return label.FromServer(match), nil
	case len(candidates) > 0:
		return label.Folder{}, &AmbiguityError{Input: input, Candidates: candidates}
	}

	return label.Folder{}, &NotFoundError{Input: input}
}

// ResolveAll returns every folder matching prefix, for the commands that fan
// out over a subtree instead of naming one folder.
func ResolveAll(c LabelReader, prefix string) ([]label.Folder, error) {
	all, err := c.GetMatchingLabels("")
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		return label.FromServerAll(all), nil
	}
	name := prefix
	if strings.EqualFold(name, "inbox") {
		name = c.InboxFolder().String()
	}
	lower := strings.ToLower(name)

	var matches []string
	for _, l := range all {
		ll := strings.ToLower(l)
		if ll == lower || strings.HasPrefix(ll, lower+"/") {
			matches = append(matches, l)
		}
	}
	if len(matches) == 0 {
		return nil, &NotFoundError{Input: prefix}
	}
	return label.FromServerAll(label.Unique(matches)), nil
}

// ResolveLabel is ResolveOne with a progress notice, for the commands that
// resolve a label as a step of a larger operation and want to say which folder
// they landed on.
func ResolveLabel(rep report.Reporter, client LabelReader, inputLabel string) (label.Folder, error) {
	resolved, err := ResolveOne(client, inputLabel)
	if err != nil {
		return label.Folder{}, err
	}
	if !resolved.IsZero() && !strings.EqualFold(resolved.String(), inputLabel) {
		report.Or(rep).Printf("[*] Using label: %s\n", resolved)
	}
	return resolved, nil
}

func matchDirect(c LabelReader, name string) (string, bool, error) {
	direct, err := c.GetMatchingLabels(name)
	if err != nil || len(direct) == 0 {
		// A backend that cannot answer the narrow question is not a failure
		// here: the full listing below can still resolve the name.
		return "", false, nil
	}
	for _, l := range direct {
		if strings.EqualFold(l, name) {
			return l, true, nil
		}
	}
	if direct = label.Unique(direct); len(direct) == 1 {
		return direct[0], true, nil
	}
	return "", false, nil
}
