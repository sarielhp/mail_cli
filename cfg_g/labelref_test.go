package cfg_g

import (
	"errors"
	"strings"
	"testing"
)

func TestParseLabelRef(t *testing.T) {
	tests := []struct {
		raw         string
		wantAccount string
		wantLabel   string
		wantSigil   bool
	}{
		{"%inbox", "", "inbox", true},
		{"%work:received", "work", "received", true},
		{"%Work:received", "Work", "received", true},
		{"%keep/2026/received", "", "keep/2026/received", true},
		{"%[Gmail]/Spam", "", "[Gmail]/Spam", true},

		// "%%foo" is the label literally named "%foo".
		{"%%foo", "", "%foo", true},

		// A numeric prefix was the removed `1:folder` form, which was always
		// written bare. Under a sigil it is just a label with a colon.
		{"%2026:notes", "", "2026:notes", true},

		// A prefix that looks like an account name is taken as one. Inherent to
		// an infix separator; Lookup's error is what shows the user why.
		{"%Re: something", "Re", " something", true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseLabelRef(tt.raw)
			if err != nil {
				t.Fatalf("ParseLabelRef(%q): %v", tt.raw, err)
			}
			if got.Account != tt.wantAccount || got.Label != tt.wantLabel {
				t.Errorf("ParseLabelRef(%q) = {%q, %q}, want {%q, %q}",
					tt.raw, got.Account, got.Label, tt.wantAccount, tt.wantLabel)
			}
			if got.Sigil != tt.wantSigil {
				t.Errorf("ParseLabelRef(%q).Sigil = %v, want %v", tt.raw, got.Sigil, tt.wantSigil)
			}
		})
	}
}

// A label carries its sigil. The bare form was accepted for two releases with
// a warning naming the new spelling; that window has closed, and the error now
// names it instead -- the whole difficulty of a grammar change being that the
// user knows what they meant and not what to type.
func TestParseLabelRefRejectsTheBareForm(t *testing.T) {
	for _, raw := range []string{"inbox", "work:received", "1:received"} {
		t.Run(raw, func(t *testing.T) {
			_, err := ParseLabelRef(raw)
			if err == nil {
				t.Fatalf("ParseLabelRef(%q) was accepted; labels carry a sigil", raw)
			}

			var bare *BareLabelError
			if !errors.As(err, &bare) {
				t.Fatalf("got %T (%v), want *BareLabelError", err, err)
			}
			if want := LabelSigil + raw; !strings.Contains(err.Error(), want) {
				t.Errorf("the error should name %q as the spelling to use, got %v", want, err)
			}
		})
	}

	// A sigilled reference is a label, whatever it contains -- including the
	// colon that used to mean the removed numeric-index form.
	if _, err := ParseLabelRef("%1:received"); err != nil {
		t.Errorf("a sigilled reference is a label, not the old index form: %v", err)
	}
}

// A reference must render back to something a user could retype.
func TestLabelRefRoundTrips(t *testing.T) {
	for _, raw := range []string{"%inbox", "%work:received", "%keep/2026/x"} {
		ref, err := ParseLabelRef(raw)
		if err != nil {
			t.Fatalf("ParseLabelRef(%q): %v", raw, err)
		}
		if got := ref.String(); got != raw {
			t.Errorf("%q round-tripped to %q", raw, got)
		}
	}
}

func TestNotALabelErrorNamesTheSyntax(t *testing.T) {
	err := &NotALabelError{Arg: "19f3a821"}
	if !strings.Contains(err.Error(), "%name") {
		t.Errorf("error should show the syntax, got %q", err)
	}
}

// "%" alone is the inbox: the folder a mail client is nearly always asked
// about, given the shortest name there is.
func TestParseLabelRefDefaultsToTheInbox(t *testing.T) {
	tests := []struct {
		raw         string
		wantAccount string
	}{
		{"%", ""},
		{"%work:", "work"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseLabelRef(tt.raw)
			if err != nil {
				t.Fatalf("ParseLabelRef(%q): %v", tt.raw, err)
			}
			if got.Label != DefaultLabel {
				t.Errorf("ParseLabelRef(%q).Label = %q, want %q", tt.raw, got.Label, DefaultLabel)
			}
			if got.Account != tt.wantAccount {
				t.Errorf("ParseLabelRef(%q).Account = %q, want %q", tt.raw, got.Account, tt.wantAccount)
			}
			if !got.Sigil {
				t.Errorf("ParseLabelRef(%q) should carry a sigil", tt.raw)
			}
		})
	}
}
