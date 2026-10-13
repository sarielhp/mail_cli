package cfg_g

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sarielhp/mail_cli/label"
)

// A label reference is written `%[account:]label`.
//
// The sigil exists because without it the tool has to guess. `show
// quarterly-invoices` is eighteen alphanumerics, which is also the shape of a
// message ID, and runner.looksLikeMessageID decided between them by counting
// characters. `%quarterly-invoices` says which was meant, and `19f3a8216d52dfc6`
// says the other.
//
// `%%` at the start escapes a literal `%`, for a label actually named "%foo".
// No such label can be created any more (label.ValidateNewName); the escape
// stays for the ones that predate that rule.

// LabelSigil marks an argument as naming a label. It is label.Sigil, which
// the backends consult to refuse creating a folder whose name begins with it.
const LabelSigil = label.Sigil

// DefaultLabel is what a reference with no label names. Each backend spells its
// inbox differently, so this is the word, not the folder; resolution happens
// against a client.
const DefaultLabel = "inbox"

// LabelRef is a parsed label reference.
type LabelRef struct {
	// Account is the account the label belongs to, empty for the selected one.
	Account string

	// Label is the label itself, with no sigil and no account prefix.
	Label string

	// Sigil records whether the reference carried one. During the deprecation
	// window a bare label still parses, and the caller warns.
	Sigil bool
}

// NotALabelError reports an argument that does not carry the sigil, so a caller
// can tell "the user meant a message ID" from "the user typed a label wrong".
type NotALabelError struct {
	Arg string
}

func (e *NotALabelError) Error() string {
	return fmt.Sprintf("%q is not a label reference; labels are written %sname", e.Arg, LabelSigil)
}

// ParseLabelRef reads a label reference.
//
// A bare label -- no sigil -- is still accepted and comes back with Sigil
// false, so the deprecation window can warn rather than fail. Once the window
// closes this returns a NotALabelError instead.
func ParseLabelRef(raw string) (LabelRef, error) {
	ref := LabelRef{}

	body := raw
	switch {
	case strings.HasPrefix(raw, LabelSigil+LabelSigil):
		// "%%foo" is the label literally named "%foo".
		return LabelRef{Label: raw[1:], Sigil: true}, nil
	case strings.HasPrefix(raw, LabelSigil):
		ref.Sigil = true
		body = raw[len(LabelSigil):]

		// "%" alone is the inbox, and "%work:" is that account's. The inbox is
		// the folder a mail client is nearly always asked about, so it gets the
		// shortest possible name. DefaultLabel resolves per backend later.
		if body == "" || strings.HasSuffix(body, ":") {
			body += DefaultLabel
		}
	}

	// A label carries its sigil. The bare form was accepted for two releases so
	// muscle memory had somewhere to land, with a warning naming the new
	// spelling; that window has closed, and accepting it silently for ever
	// would have meant two grammars indefinitely -- the thing the sigil exists
	// to end.
	// An empty spec is not a bare label: it means no label was given at all,
	// which is how the account-spanning commands ask for "everywhere".
	if !ref.Sigil && raw != "" {
		return LabelRef{}, &BareLabelError{Raw: raw}
	}

	// The numeric-index form was `1:folder`, written without a sigil. A sigil
	// says the whole argument is a label reference, so `%2026:notes` is a label
	// with a colon in it, not the removed syntax.
	account, label, err := splitAccountLabel(body, !ref.Sigil)
	if err != nil {
		return LabelRef{}, err
	}
	ref.Account, ref.Label = account, label
	return ref, nil
}

// splitAccountLabel separates an "account:" prefix from the label.
//
// rejectNumericIndex reports the removed `1:folder` form. It applies only to a
// bare reference, because that is the only place the old syntax could appear.
//
// A prefix that looks like an account name is taken as one, so a label named
// "Re: something" resolves as account "Re". That is inherent to an infix
// separator and is why Lookup's error lists the configured accounts: being told
// `account "Re" not found; configured accounts: ...` is how a user sees what
// happened.
func splitAccountLabel(body string, rejectNumericIndex bool) (account, label string, err error) {
	idx := strings.Index(body, ":")
	if idx <= 0 {
		return "", body, nil
	}

	prefix := body[:idx]
	if num, convErr := strconv.Atoi(prefix); convErr == nil && num > 0 {
		if rejectNumericIndex {
			return "", "", fmt.Errorf(
				"numeric account index (%d:) is no longer supported — use the account name (e.g. %q)",
				num, body[idx+1:])
		}
		return "", body, nil
	}

	if !startsWithLetter(prefix) {
		return "", body, nil
	}
	return prefix, body[idx+1:], nil
}

func startsWithLetter(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// String renders a reference the way a user would type it.
func (r LabelRef) String() string {
	if r.Account == "" {
		return LabelSigil + r.Label
	}
	return LabelSigil + r.Account + ":" + r.Label
}

// BareLabelError reports a label argument written without its sigil.
//
// It names the spelling that would have worked, because the whole difficulty
// of a grammar change is that the user knows what they meant and not what to
// type instead.
type BareLabelError struct {
	Raw string
}

func (e *BareLabelError) Error() string {
	return fmt.Sprintf("%q is not a label reference: write %q (labels carry a %s)",
		e.Raw, LabelSigil+e.Raw, LabelSigil)
}
