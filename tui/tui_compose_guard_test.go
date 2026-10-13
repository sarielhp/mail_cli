package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A draft was once saved with no subject and no body. Whatever emptied the
// editor's file, an empty file must not replace a body that existed.
func TestEditorReturningEmptyFileKeepsBody(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{To: "x@example.com", Subject: "s", Body: "the text"}
	empty := filepath.Join(t.TempDir(), "edit.txt")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}

	m2, _ := m.handleComposeEditorFinished(composeEditorFinishedMsg{path: empty, initialState: m.compose, isReEdit: true})
	m = m2.(*tuiModel)

	if m.compose == nil || m.compose.Body != "the text" {
		t.Fatalf("body replaced: %+v", m.compose)
	}
	if !strings.Contains(m.saveStatus, "empty file") {
		t.Errorf("saveStatus %q should say why nothing changed", m.saveStatus)
	}
}

func TestEditorReturningEmptyFileCancelsNewCompose(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.standaloneCompose = true
	empty := filepath.Join(t.TempDir(), "compose.txt")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	state := &composeState{To: "x@example.com", InitialTemplate: "\n-- \nsig\n"}

	m2, cmd := m.handleComposeEditorFinished(composeEditorFinishedMsg{path: empty, initialState: state})
	m = m2.(*tuiModel)
	if m.compose != nil || cmd == nil {
		t.Fatalf("an empty file should cancel, got compose=%v cmd=%v", m.compose != nil, cmd != nil)
	}
	if entries, _ := m.draftStore().List(); len(entries) != 0 {
		t.Errorf("an empty compose left drafts behind: %+v", entries)
	}
}

func TestEmptyMessageIsNeverWrittenAsDraft(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.mode = modeCompose
	m.compose = &composeState{To: "x@example.com"}

	if _, err := m.saveLocalDraft(); err != errDraftEmpty {
		t.Fatalf("err = %v, want errDraftEmpty", err)
	}
	if entries, _ := m.draftStore().List(); len(entries) != 0 {
		t.Errorf("empty message written: %+v", entries)
	}

	m.standaloneCompose = true
	m2, _ := m.saveDraftAndExit()
	m = m2.(*tuiModel)
	if !strings.Contains(m.saveStatus, "Empty message") || !m.done {
		t.Errorf("exit on an empty message: status %q done=%v", m.saveStatus, m.done)
	}
}

// Every draft copy and the sent message must carry the same Message-ID, so
// the server prune can tell a sent draft from one still being written.
func TestComposeMessageIDIsStableAcrossBuilds(t *testing.T) {
	m, _, _ := setupTestComposeModel(t, false)
	m.compose = &composeState{From: "me@example.com", To: "x@example.com", Subject: "s", Body: "b"}

	first, err := m.buildComposeRFC822()
	if err != nil {
		t.Fatal(err)
	}
	m.compose.Body = "b2"
	second, err := m.buildComposeRFC822()
	if err != nil {
		t.Fatal(err)
	}
	id := headerOf(first, "Message-ID")
	if id == "" || !strings.HasSuffix(strings.TrimSuffix(id, ">"), "@example.com") {
		t.Errorf("Message-ID %q should be under the sender's domain", id)
	}
	if headerOf(second, "Message-ID") != id {
		t.Errorf("Message-ID changed between builds: %q vs %q", id, headerOf(second, "Message-ID"))
	}

	// A resumed draft keeps the ID it was saved with.
	path, err := m.saveLocalDraft()
	if err != nil {
		t.Fatal(err)
	}
	m.compose = nil
	m.composeOpts.DraftPath = path
	m.standaloneCompose = true
	m.initStandaloneComposeCmd()
	if m.compose == nil || m.compose.MessageID != id {
		t.Errorf("resumed Message-ID = %q, want %q", m.compose.MessageID, id)
	}
}

func headerOf(raw []byte, name string) string {
	for _, line := range strings.Split(string(raw), "\r\n") {
		if strings.HasPrefix(line, name+": ") {
			return strings.TrimPrefix(line, name+": ")
		}
	}
	return ""
}

// The compose help listed f/A/I, which no longer do anything, and said
// Ctrl+C quits immediately, which now saves and uploads the draft.
func TestComposeHelpMatchesTheBindings(t *testing.T) {
	help := renderComposeHelp("#888888", "#000000")
	if !strings.Contains(help, "F3") || strings.Contains(help, "f/A/I") {
		t.Errorf("compose help does not name F3 alone:\n%s", help)
	}
	if strings.Contains(help, "Quit immediately") || !strings.Contains(help, "Save draft & exit") {
		t.Errorf("compose help misdescribes Ctrl+C:\n%s", help)
	}
}
