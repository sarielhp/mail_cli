package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/contacts"

	tea "github.com/charmbracelet/bubbletea"
)

func composeWithContacts(t *testing.T) *tuiModel {
	t.Helper()
	m, _ := composeWithTrash(t)
	m.cfg.ConfigDir = t.TempDir()
	book := contacts.NewBook()
	book.Contacts = []contacts.Contact{
		{ID: "1", Name: "Alice Adams", Emails: []string{"alice@example.com"}},
		{ID: "2", Name: "Bob Brown", Emails: []string{"bob@example.com"}, Nicknames: []string{"bobby"}},
		{ID: "3", Name: "Carol Chen", Emails: []string{"carol@example.com"}},
	}
	if err := contacts.SaveBook(contacts.DefaultPath(m.cfg), book); err != nil {
		t.Fatal(err)
	}
	m.compose.To = ""
	return m
}

func typeInto(m *tuiModel, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestContactPicker_SearchAndAddToTo(t *testing.T) {
	m := composeWithContacts(t)
	press(m, "f5")
	if m.contactPick == nil || m.contactPick.target != fieldTo || len(m.contactPick.results) != 3 {
		t.Fatalf("picker = %+v", m.contactPick)
	}
	if !strings.Contains(m.View(), "CONTACTS") {
		t.Fatal("picker not drawn over the compose screen")
	}
	typeInto(m, "bob")
	if len(m.contactPick.results) != 1 || m.contactPick.results[0].PrimaryEmail() != "bob@example.com" {
		t.Fatalf("results for bob = %+v", m.contactPick.results)
	}
	press(m, "enter")
	if m.contactPick != nil || m.compose.To != "Bob Brown <bob@example.com>" {
		t.Fatalf("To = %q, picker open = %v", m.compose.To, m.contactPick != nil)
	}
}

func TestContactPicker_MarkSeveralIntoCcWithoutDuplicates(t *testing.T) {
	m := composeWithContacts(t)
	m.compose.Cc = "alice@example.com"
	press(m, "f5")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.contactPick.target != fieldCc {
		t.Fatalf("target = %v, want Cc", m.contactPick.target)
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace}) // Alice, already in Cc
	m.Update(tea.KeyMsg{Type: tea.KeySpace}) // Bob
	m.Update(tea.KeyMsg{Type: tea.KeySpace}) // Carol
	press(m, "enter")
	if m.compose.Cc != "alice@example.com, Bob Brown <bob@example.com>, Carol Chen <carol@example.com>" {
		t.Fatalf("Cc = %q", m.compose.Cc)
	}
	if m.compose.To != "" {
		t.Errorf("To changed: %q", m.compose.To)
	}
}

// Opened while a recipient field is being typed, the picker finishes the
// entry in progress and the edit continues.
func TestContactPicker_FromFieldEditReplacesPartialEntry(t *testing.T) {
	m := composeWithContacts(t)
	m.compose.FieldEditing = fieldTo
	m.compose.FieldInput = "alice@example.com, car"
	press(m, "f5")
	if m.contactPick == nil || !m.contactPick.editing {
		t.Fatal("picker did not open for the field being edited")
	}
	typeInto(m, "carol")
	press(m, "enter")
	if m.compose.FieldEditing != fieldTo || m.compose.FieldInput != "alice@example.com, Carol Chen <carol@example.com>" {
		t.Fatalf("editing %v input %q", m.compose.FieldEditing, m.compose.FieldInput)
	}
}

func TestContactPicker_EscChangesNothing(t *testing.T) {
	m := composeWithContacts(t)
	press(m, "f5")
	typeInto(m, "alice")
	press(m, "esc")
	if m.contactPick != nil || m.compose.To != "" || m.compose == nil {
		t.Fatalf("picker open %v, To %q", m.contactPick != nil, m.compose.To)
	}
}

func TestContactPicker_NotForSubject(t *testing.T) {
	m := composeWithContacts(t)
	m.compose.FieldEditing = fieldSubject
	press(m, "f5")
	if m.contactPick != nil {
		t.Fatal("the picker opened for the subject line")
	}
}

func TestAppendRecipients(t *testing.T) {
	cases := []struct {
		list, want string
		picked     []string
	}{
		{"", "a@x", []string{"a@x"}},
		{"A <a@x>", "A <a@x>", []string{"a@x"}},
		{"a@x, bo", "a@x, Bob <b@x>", []string{"Bob <b@x>"}},
		{"a@x,", "a@x, b@x", []string{"b@x", "B@X"}},
	}
	for _, tc := range cases {
		if got := appendRecipients(tc.list, tc.picked); got != tc.want {
			t.Errorf("appendRecipients(%q, %v) = %q, want %q", tc.list, tc.picked, got, tc.want)
		}
	}
}
