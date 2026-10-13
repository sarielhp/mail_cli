package tui

import (
	"fmt"
	"net/mail"
	"strings"

	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/tui/render"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The contacts picker: a searchable list over the same ranking Tab-completion
// uses -- stored contacts and aliases merged with the history of addresses
// sent to and archived from -- that inserts the chosen people into a
// recipient field. F5 opens it on the compose screen, from the review screen
// (adding to To, Tab switching to Cc or Bcc) or while a recipient field is
// being edited (adding to that field).

const contactPickerLimit = 200

type contactPicker struct {
	query   string
	results []contacts.ScoredContact
	cursor  int
	marked  map[string]bool // keyed by formatted recipient
	target  composeField
	// editing is set when the picker was opened from a field being edited:
	// choices go into FieldInput and the edit continues.
	editing bool

	book    *contacts.Book
	history *contacts.HistoryBook
}

// openContactPicker opens the picker for the field being edited, or for To
// when opened from the review screen. Subject has no recipients to pick.
func (m *tuiModel) openContactPicker() {
	if m.compose == nil {
		return
	}
	target, editing := fieldTo, false
	switch m.compose.FieldEditing {
	case fieldTo, fieldCc, fieldBcc:
		target, editing = m.compose.FieldEditing, true
	case fieldSubject:
		return
	}
	book, _ := contacts.LoadBook(contacts.DefaultPath(m.cfg))
	history, _ := contacts.LoadHistory(contacts.DefaultHistoryPath(m.cfg))
	m.contactPick = &contactPicker{
		target:  target,
		editing: editing,
		marked:  map[string]bool{},
		book:    book,
		history: history,
	}
	m.resetComposeSuggestions()
	m.refreshContactPicker()
}

func (m *tuiModel) refreshContactPicker() {
	p := m.contactPick
	var blacklist []string
	if m.cfg != nil {
		blacklist = m.cfg.Policy.Blacklist
	}
	p.results = contacts.Suggest(p.book, p.history, blacklist, p.query, contactPickerLimit)
	if p.cursor >= len(p.results) {
		p.cursor = max(0, len(p.results)-1)
	}
}

// kContactPicker handles keys while the picker is open. Letters go to the
// search, so movement is on the arrow and page keys.
func (m *tuiModel) kContactPicker(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.contactPick
	switch k.Type {
	case tea.KeyEsc:
		m.contactPick = nil
		return m, nil
	case tea.KeyEnter:
		return m.insertPickedContacts()
	case tea.KeyUp:
		p.cursor = max(0, p.cursor-1)
	case tea.KeyDown:
		p.cursor = min(len(p.results)-1, p.cursor+1)
	case tea.KeyPgUp:
		p.cursor = max(0, p.cursor-m.contactPickerRows())
	case tea.KeyPgDown:
		p.cursor = min(len(p.results)-1, p.cursor+m.contactPickerRows())
	case tea.KeyTab:
		if !p.editing {
			p.target = nextRecipientField(p.target)
		}
	case tea.KeySpace:
		m.toggleContactMark()
	case tea.KeyBackspace:
		if p.query != "" {
			r := []rune(p.query)
			p.query = string(r[:len(r)-1])
			m.refreshContactPicker()
		}
	case tea.KeyRunes:
		p.query += string(k.Runes)
		p.cursor = 0
		m.refreshContactPicker()
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	return m, nil
}

func nextRecipientField(f composeField) composeField {
	switch f {
	case fieldTo:
		return fieldCc
	case fieldCc:
		return fieldBcc
	}
	return fieldTo
}

func (m *tuiModel) toggleContactMark() {
	p := m.contactPick
	if p.cursor < 0 || p.cursor >= len(p.results) {
		return
	}
	r := formatCompletionString(p.results[p.cursor])
	if r == "" {
		return
	}
	if p.marked[r] {
		delete(p.marked, r)
	} else {
		p.marked[r] = true
	}
	p.cursor = min(len(p.results)-1, p.cursor+1)
}

// pickedRecipients is the marked contacts in list order, or the highlighted
// one when nothing is marked.
func (p *contactPicker) pickedRecipients() []string {
	var picked []string
	for _, sc := range p.results {
		if r := formatCompletionString(sc); r != "" && p.marked[r] {
			picked = append(picked, r)
		}
	}
	if len(picked) == 0 && p.cursor >= 0 && p.cursor < len(p.results) {
		if r := formatCompletionString(p.results[p.cursor]); r != "" {
			picked = append(picked, r)
		}
	}
	return picked
}

// insertPickedContacts adds the chosen recipients to the target field,
// skipping addresses already there, and closes the picker.
func (m *tuiModel) insertPickedContacts() (tea.Model, tea.Cmd) {
	p := m.contactPick
	m.contactPick = nil
	picked := p.pickedRecipients()
	if len(picked) == 0 || m.compose == nil {
		return m, nil
	}
	if p.editing {
		m.compose.FieldInput = appendRecipients(m.compose.FieldInput, picked)
		return m, nil
	}
	field := m.recipientField(p.target)
	*field = appendRecipients(*field, picked)
	return m, m.saveAndSyncDraftCmd()
}

func (m *tuiModel) recipientField(f composeField) *string {
	switch f {
	case fieldCc:
		return &m.compose.Cc
	case fieldBcc:
		return &m.compose.Bcc
	}
	return &m.compose.To
}

// appendRecipients adds each recipient whose address is not already in the
// comma-separated list. A trailing partial entry the user was typing -- text
// after the last comma that is not an address -- is replaced, since picking
// is how that entry gets finished.
func appendRecipients(list string, picked []string) string {
	entries := splitRecipients(list)
	if n := len(entries); n > 0 && !hasAddress(entries[n-1]) {
		entries = entries[:n-1]
	}
	have := map[string]bool{}
	for _, e := range entries {
		have[addressKey(e)] = true
	}
	for _, r := range picked {
		key := addressKey(r)
		if key == "" || have[key] {
			continue
		}
		have[key] = true
		entries = append(entries, r)
	}
	return strings.Join(entries, ", ")
}

func splitRecipients(list string) []string {
	var out []string
	for _, e := range strings.Split(list, ",") {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, e)
		}
	}
	return out
}

func hasAddress(entry string) bool {
	return strings.Contains(entry, "@")
}

func addressKey(entry string) string {
	if a, err := mail.ParseAddress(entry); err == nil {
		return strings.ToLower(a.Address)
	}
	return strings.ToLower(strings.TrimSpace(entry))
}

func (m *tuiModel) contactPickerRows() int {
	return max(5, m.height-12)
}

func fieldLabel(f composeField) string {
	switch f {
	case fieldCc:
		return "Cc"
	case fieldBcc:
		return "Bcc"
	}
	return "To"
}

func renderContactPicker(m *tuiModel) string {
	t := m.theme.Theme()
	p := m.contactPick
	dialogWidth := min(100, max(50, m.width-4))
	inner := dialogWidth - 4
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(t.Get(render.ColorHighlightFg)))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorDate)))
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Get(render.ColorFg)))

	lines := []string{
		head.Render(fmt.Sprintf("CONTACTS  → %s", fieldLabel(p.target))),
		"",
		fmt.Sprintf("Search: %s▏", p.query),
		"",
	}
	lines = append(lines, contactRows(p, m.contactPickerRows(), inner, head, fg, dim)...)

	help := "type: search  ↑/↓: move  Space: mark  Enter: add  Tab: To/Cc/Bcc  Esc: cancel"
	if p.editing {
		help = "type: search  ↑/↓: move  Space: mark  Enter: add  Esc: cancel"
	}
	if len(p.marked) > 0 {
		help = fmt.Sprintf("%d marked  │  %s", len(p.marked), help)
	}
	lines = append(lines, "", dim.Render(markHints(truncateTo(help, inner),
		hint{"Enter: add", "enter"}, hint{"Tab: To/Cc/Bcc", "tab"}, hint{"Esc: cancel", "esc"})))

	dialog := lipgloss.NewStyle().
		Width(dialogWidth).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Get(render.ColorBorder))).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(m.width).Height(max(10, m.height)).
		Align(lipgloss.Center, lipgloss.Center).Render(dialog)
}

// contactRows renders the visible window of results around the cursor.
func contactRows(p *contactPicker, rows, width int, sel, fg, dim lipgloss.Style) []string {
	if len(p.results) == 0 {
		return []string{dim.Render("  No matching contacts.")}
	}
	start := 0
	if p.cursor >= rows {
		start = p.cursor - rows + 1
	}
	end := min(len(p.results), start+rows)
	var out []string
	for i := start; i < end; i++ {
		sc := p.results[i]
		mark := "  "
		if p.marked[formatCompletionString(sc)] {
			mark = "✓ "
		}
		row := truncateTo(mark+contactLine(sc), width-2)
		if i == p.cursor {
			out = append(out, rowZone(frameContacts, i, 0, sel.Render("> "+row)))
			continue
		}
		out = append(out, rowZone(frameContacts, i, 0, fg.Render("  "+row)))
	}
	if len(p.results) > rows {
		out = append(out, dim.Render(fmt.Sprintf("  %d-%d of %d", start+1, end, len(p.results))))
	}
	return out
}

func contactLine(sc contacts.ScoredContact) string {
	name := sc.Name
	if name == "" {
		name = sc.DisplayAlias()
	}
	addr := sc.PrimaryEmail()
	if len(sc.Emails) > 1 && strings.Contains(sc.ID, "alias:group") {
		addr = fmt.Sprintf("group of %d", len(sc.Emails))
	}
	extra := ""
	if sc.Frequency > 0 {
		extra = fmt.Sprintf("  (%d×)", sc.Frequency)
	}
	return fmt.Sprintf("%-28s %s%s", truncateTo(name, 28), addr, extra)
}

func truncateTo(s string, width int) string {
	r := []rune(s)
	if width <= 1 || len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}
