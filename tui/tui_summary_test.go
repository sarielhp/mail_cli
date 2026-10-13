package tui

import (
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/tui/render"

	tea "github.com/charmbracelet/bubbletea"
)

// newSummaryServer returns an OpenAI-compatible endpoint that answers every
// request with a single assistant message carrying content.
func newSummaryServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": content,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestTUI_SummarizeEmail_Unconfigured(t *testing.T) {
	cacheDir := t.TempDir()
	cfg := &cfg_g.Config{
		LLM:   cfg_g.LLMConfig{}, // Unconfigured
		Paths: cfg_g.Paths{CacheRoot: cacheDir}, AccountCache: cachedir.At(cacheDir),
	}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.cfg = cfg
	m.emails = []email.Email{
		{ID: "msg-123", Subject: "Meeting Tomorrow", FromRaw: "boss@example.com"},
	}
	m.eIdx = 0
	m.selectedID = "msg-123"

	// Press F4
	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF4})
	tm := updatedModel.(*tuiModel)

	if cmd != nil {
		t.Errorf("expected no cmd when unconfigured, got %v", cmd)
	}
	if !tm.showError {
		t.Errorf("expected tm.showError=true when LLM not configured")
	}
	if tm.err == nil || !strings.Contains(tm.err.Error(), "LLM not configured") {
		t.Errorf("expected error mentioning LLM not configured, got %v", tm.err)
	}
}

func TestTUI_SummarizeEmail_ConfiguredFlow(t *testing.T) {
	cacheDir := t.TempDir()
	rawMsg := "From: boss@example.com\r\nSubject: Project Update\r\nDate: Mon, 24 Aug 2026 10:00:00 +0000\r\n\r\nPlease finalize the proposal by Friday."
	_ = msg.Store(cachedir.At(cacheDir), "msg-123", []byte(rawMsg), time.Now())

	server := newSummaryServer(t, "1. **TL;DR**: Proposal needs finalization.\n\n2. **Action Items**:\n- Finalize by Friday\n- Review budget with `finance` team.")

	cfg := &cfg_g.Config{LLM: cfg_g.LLMConfig{
		Endpoint: server.URL,
		APIKey:   "sk-test",
		Model:    "llama3.1",
	}, Paths: cfg_g.Paths{CacheRoot: cacheDir}, AccountCache: cachedir.At(cacheDir)}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.cfg = cfg
	m.width = 80
	m.height = 24
	m.emails = []email.Email{
		{ID: "msg-123", Subject: "Project Update", FromRaw: "boss@example.com", FormattedDate: "Aug 24"},
	}
	m.eIdx = 0
	m.selectedID = "msg-123"

	// 1. Press F4 -> trigger summary command
	updatedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF4})
	tm := updatedModel.(*tuiModel)
	if cmd == nil {
		t.Fatalf("expected cmd from F4 press")
	}
	if tm.showLoad == "" {
		t.Errorf("expected loading message, got empty")
	}

	// 2. Execute the cmd
	resMsg := cmd()
	sumMsg, ok := resMsg.(summaryLoadedMsg)
	if !ok {
		t.Fatalf("expected summaryLoadedMsg, got %T (%v)", resMsg, resMsg)
	}
	if sumMsg.err != nil {
		t.Fatalf("unexpected summary error: %v", sumMsg.err)
	}
	if !strings.Contains(sumMsg.summary, "Proposal needs finalization") {
		t.Errorf("unexpected summary content: %s", sumMsg.summary)
	}

	// Verify on-disk cache was written (.llm_summary)
	diskSummary, err := msg.ReadSummary(cachedir.At(cacheDir), "msg-123")
	if err != nil {
		t.Fatalf("failed to read on-disk summary: %v", err)
	}
	if !strings.Contains(diskSummary, "Proposal needs finalization") {
		t.Errorf("on-disk summary content mismatch: %s", diskSummary)
	}

	// 3. Update model with summaryLoadedMsg
	updatedModel2, _ := tm.Update(sumMsg)
	tm2 := updatedModel2.(*tuiModel)
	if !tm2.showSummary {
		t.Errorf("expected showSummary=true")
	}
	if tm2.summaryCache["msg-123"] == "" {
		t.Errorf("expected summaryCache to store summary")
	}

	// 4. Test View() when summary is active — check Header and colored content
	viewOutput := tm2.View()
	if !strings.Contains(viewOutput, "EMAIL SUMMARY") {
		t.Errorf("expected View() to contain 'EMAIL SUMMARY', got: %s", viewOutput)
	}
	if !strings.Contains(viewOutput, "From:") || !strings.Contains(viewOutput, "boss@example.com") {
		t.Errorf("expected View() to display sender in header, got: %s", viewOutput)
	}
	if !strings.Contains(viewOutput, "Subject:") || !strings.Contains(viewOutput, "Project Update") {
		t.Errorf("expected View() to display subject in header, got: %s", viewOutput)
	}
	if !strings.Contains(viewOutput, "Proposal needs finalization") {
		t.Errorf("expected View() to contain summary text, got: %s", viewOutput)
	}

	// 5. Test Arrow Key Navigation (Down, Up, PgDn, PgUp, Home, End)
	downModel, _ := tm2.Update(tea.KeyMsg{Type: tea.KeyDown})
	tmDown := downModel.(*tuiModel)
	if !tmDown.showSummary {
		t.Errorf("expected showSummary to stay true on KeyDown")
	}

	upModel, _ := tmDown.Update(tea.KeyMsg{Type: tea.KeyUp})
	tmUp := upModel.(*tuiModel)
	if !tmUp.showSummary {
		t.Errorf("expected showSummary to stay true on KeyUp")
	}

	// 6. Test dismissing summary with Esc, q, and F4
	updatedModel3, _ := tm2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	tm3 := updatedModel3.(*tuiModel)
	if tm3.showSummary {
		t.Errorf("expected showSummary=false after Esc")
	}

	// Re-open from disk cache directly without network call
	reopenModel, reopenCmd := tm3.Update(tea.KeyMsg{Type: tea.KeyF4})
	tmReopen := reopenModel.(*tuiModel)
	if reopenCmd != nil {
		t.Errorf("expected no network cmd when loading from cache, got %v", reopenCmd)
	}
	if !tmReopen.showSummary {
		t.Errorf("expected showSummary=true when loaded from disk cache")
	}
}

func TestRenderMarkdownSummary(t *testing.T) {
	theme := render.NewThemeManager().Theme()
	raw := "1. **TL;DR**: Core message is clear.\n\n2. **Action Items**:\n- Task one for **Alice**\n- Task two with `code_ref`\n\n3. **Key Dates & Deadlines**:\n- Friday *noon*"

	rendered := renderMarkdownSummary(raw, 70, theme)

	// Check that ** is converted (not remaining as raw **)
	if strings.Contains(rendered, "**TL;DR**") {
		t.Errorf("expected raw ** to be stripped and formatted, got: %s", rendered)
	}
	if !strings.Contains(rendered, "TL;DR") {
		t.Errorf("expected 'TL;DR' in output, got: %s", rendered)
	}
	if !strings.Contains(rendered, "•") {
		t.Errorf("expected bullet symbol '•' in output, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Task one") {
		t.Errorf("expected 'Task one' in output, got: %s", rendered)
	}
}

func TestTUI_BackgroundSummarization_Queue(t *testing.T) {
	cacheDir := t.TempDir()
	_ = msg.Store(cachedir.At(cacheDir), "e1", []byte("From: a@b.com\r\nSubject: Email 1\r\n\r\nContent 1"), time.Now())
	_ = msg.Store(cachedir.At(cacheDir), "e2", []byte("From: a@b.com\r\nSubject: Email 2\r\n\r\nContent 2"), time.Now())
	_ = msg.Store(cachedir.At(cacheDir), "e3", []byte("From: a@b.com\r\nSubject: Email 3\r\n\r\nContent 3"), time.Now())

	server := newSummaryServer(t, "**Classification**: Legitimate — Work email\n**Urgency**: Low | **Reply Required**: No\n**TL;DR**: Handled.")

	cfg := &cfg_g.Config{LLM: cfg_g.LLMConfig{
		Endpoint: server.URL,
		APIKey:   "sk-test",
	}, Paths: cfg_g.Paths{CacheRoot: cacheDir}, AccountCache: cachedir.At(cacheDir)}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.cfg = cfg
	m.rawEmails = []email.Email{
		{ID: "e1", Subject: "Email 1"},
		{ID: "e2", Subject: "Email 2"},
		{ID: "e3", Subject: "Email 3"},
	}
	m.emails = m.rawEmails

	// 1. Trigger background summary queue
	cmd := m.startBackgroundSummaryQueue()
	if cmd == nil {
		t.Fatalf("expected cmd from startBackgroundSummaryQueue")
	}
	if !m.summaryInFlight {
		t.Errorf("expected summaryInFlight=true")
	}
	if m.currentSummarizingID != "e1" {
		t.Errorf("expected currentSummarizingID=e1, got %s", m.currentSummarizingID)
	}

	// 2. User presses F4 on e3 while e1 is in-flight -> should promote e3 to front of queue
	m.eIdx = 2
	m.selectedID = "e3"
	updatedModel, _ := m.summarizeCurrentEmail()
	tm := updatedModel.(*tuiModel)

	if tm.waitingUserSummaryID != "e3" {
		t.Errorf("expected waitingUserSummaryID=e3, got %s", tm.waitingUserSummaryID)
	}
	if len(tm.summaryQueue) == 0 || tm.summaryQueue[0] != "e3" {
		t.Errorf("expected e3 at front of summaryQueue, got %v", tm.summaryQueue)
	}
}

func TestTUI_DeadlinesAgenda(t *testing.T) {
	cacheDir := t.TempDir()
	_ = msg.Store(cachedir.At(cacheDir), "e1", []byte("From: PI <pi@univ.edu>\r\nSubject: Grant Proposal\r\n\r\nContent"), time.Now())
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: cacheDir}, AccountCache: cachedir.At(cacheDir)}
	mockClient := &mailclient.MockMailClient{Cfg: cfg}
	m := newTuiModel(mockClient, "INBOX", nil)
	m.cfg = cfg
	m.width = 80
	m.height = 24

	summaryE1 := "**Classification**: Legitimate\n**Urgency**: High | **Reply Required**: Yes\n**Deadlines**:\n• Friday 5 PM: Proposal submission\n**TL;DR**: Must submit on time."
	_ = msg.StoreSummary(cachedir.At(cacheDir), "e1", summaryE1)

	m.rawEmails = []email.Email{
		{ID: "e1", Subject: "Grant Proposal", FromRaw: "PI <pi@univ.edu>"},
	}
	m.emails = m.rawEmails

	// Press 'D' to open Deadlines Agenda
	res, _ := m.openDeadlinesAgenda()
	tm := res.(*tuiModel)

	if !tm.showDeadlinesAgenda {
		t.Fatalf("expected showDeadlinesAgenda=true")
	}
	if len(tm.deadlinesList) != 1 {
		t.Fatalf("expected 1 deadline entry, got %d", len(tm.deadlinesList))
	}
	if !strings.Contains(tm.deadlinesList[0].Description, "Proposal submission") {
		t.Errorf("unexpected deadline description: %s", tm.deadlinesList[0].Description)
	}

	viewOutput := tm.View()
	if !strings.Contains(viewOutput, "UPCOMING ACTIONABLE DEADLINES") {
		t.Errorf("expected agenda title in View(), got: %s", viewOutput)
	}
	if !strings.Contains(viewOutput, "Proposal submission") {
		t.Errorf("expected deadline in View(), got: %s", viewOutput)
	}

	// Press Enter to jump to email
	res2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm2 := res2.(*tuiModel)
	if tm2.showDeadlinesAgenda {
		t.Errorf("expected showDeadlinesAgenda=false after jumping to email")
	}
	if tm2.selectedID != "e1" {
		t.Errorf("expected selectedID=e1, got %s", tm2.selectedID)
	}
}

// An AI summary is an opinion shown in the summary pane; it must not become the
// message's cached verdict. It did, and a small model's habit of answering
// "Marketing / Political" labelled the user's own sent replies [POLI].
func TestTUI_SummaryDoesNotClassifyMessage(t *testing.T) {
	cacheDir := t.TempDir()
	account := cachedir.At(cacheDir)
	_ = msg.Store(account, "e1", []byte("From: me@example.com\r\nSubject: Re: talk\r\n\r\nSee you there."), time.Now())
	cfg := &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: cacheDir}, AccountCache: account}
	m := newTuiModel(&mailclient.MockMailClient{Cfg: cfg}, "Sent", nil)
	m.cfg = cfg
	m.rawEmails = []email.Email{{ID: "e1", Subject: "Re: talk"}}
	m.emails = m.rawEmails

	summary := "**Classification**: Marketing / Political — a reply about a talk.\n**TL;DR**: Will attend."
	m.handleSummaryLoaded(summaryLoadedMsg{id: "e1", summary: summary})

	if m.emails[0].IsPolitical || m.emails[0].IsSpam {
		t.Error("the summary relabelled the listed message")
	}
	info, err := msg.GetInfo(account, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if info.Scanned || info.IsPolitical() || info.IsSpam() {
		t.Errorf("the summary wrote a cached verdict: %+v", info)
	}
}

// Summaries cached before the prompt dropped its classification still carry
// the line; the pane must not show the model's spam/political verdict.
func TestRenderMarkdownSummary_HidesLegacyClassification(t *testing.T) {
	raw := "**Classification**: Marketing / Political — a seminar.\n**TL;DR**: Talk on Friday."
	out := renderMarkdownSummary(raw, 80, nil)
	if strings.Contains(out, "Political") {
		t.Errorf("classification shown:\n%s", out)
	}
	if !strings.Contains(out, "Talk on Friday") {
		t.Errorf("TL;DR lost:\n%s", out)
	}
}
