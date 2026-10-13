package tui

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/llm"

	tea "github.com/charmbracelet/bubbletea"
)

type summaryLoadedMsg struct {
	id      email.ID
	from    string
	subject string
	date    string
	summary string
	err     error
}

func (m *tuiModel) startBackgroundSummaryQueue() tea.Cmd {
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg == nil || !cfg.LLM.IsAutoSummaryEnabled() || cfg.LLM.Endpoint == "" {
		return nil
	}

	cacheDir := cfg.AccountCache
	if cacheDir.IsZero() {
		return nil
	}

	var unsummarized []email.ID
	for _, em := range m.rawEmails {
		if !msg.HasSummary(cacheDir, em.ID) && m.summaryCache[em.ID] == "" {
			unsummarized = append(unsummarized, em.ID)
		}
	}

	m.summaryQueue = unsummarized
	if !m.summaryInFlight && len(m.summaryQueue) > 0 {
		return m.processNextSummaryQueueItem()
	}
	return nil
}

func (m *tuiModel) processNextSummaryQueueItem() tea.Cmd {
	if len(m.summaryQueue) == 0 {
		m.summaryInFlight = false
		m.currentSummarizingID = ""
		return nil
	}

	nextID := m.summaryQueue[0]
	m.summaryQueue = m.summaryQueue[1:]
	m.summaryInFlight = true
	m.currentSummarizingID = nextID
	return m.summarizeEmailCmd(nextID)
}

func (m *tuiModel) summarizeCurrentEmail() (tea.Model, tea.Cmd) {
	targetID := m.currentEmailID()
	if targetID == "" {
		return m, nil
	}

	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}

	if cfg == nil || cfg.LLM.Endpoint == "" {
		m.err = fmt.Errorf("LLM not configured. Run 'tools/setup_local_llm' or 'mail_cli config set llm_endpoint <url>'")
		m.showError = true
		return m, nil
	}

	if m.summaryCache == nil {
		m.summaryCache = make(map[email.ID]string)
	}

	// 1. Check in-memory cache
	if cachedSummary, ok := m.summaryCache[targetID]; ok && cachedSummary != "" {
		m.showSummary = true
		m.summaryContent = cachedSummary
		m.summaryEmailID = targetID
		m.populateSummaryMetadata(targetID)
		m.setupSummaryViewport()
		return m, nil
	}

	// 2. Check on-disk cache (.llm_summary.md)
	downloadDir := cfg.AccountCache
	if diskSummary, err := msg.ReadSummary(downloadDir, targetID); err == nil && strings.TrimSpace(diskSummary) != "" {
		m.summaryCache[targetID] = diskSummary
		m.showSummary = true
		m.summaryContent = diskSummary
		m.summaryEmailID = targetID
		m.populateSummaryMetadata(targetID)
		m.setupSummaryViewport()
		return m, nil
	}

	// 3. If currently in-flight with LLM
	if m.currentSummarizingID == targetID {
		m.waitingUserSummaryID = targetID
		m.showLoad = "Summarizing email with local LLM..."
		m.showError = false
		m.err = nil
		return m, nil
	}

	// 4. If another background item is in-flight, promote targetID to the front of queue
	if m.summaryInFlight {
		var newQueue []email.ID
		newQueue = append(newQueue, targetID)
		for _, qID := range m.summaryQueue {
			if qID != targetID {
				newQueue = append(newQueue, qID)
			}
		}
		m.summaryQueue = newQueue
		m.waitingUserSummaryID = targetID
		m.showLoad = "Waiting for local LLM (next in queue)..."
		m.showError = false
		m.err = nil
		return m, nil
	}

	// 5. Idle -> start immediately
	m.summaryInFlight = true
	m.currentSummarizingID = targetID
	m.waitingUserSummaryID = targetID
	m.showLoad = "Summarizing email with local LLM..."
	m.showError = false
	m.err = nil
	return m, m.summarizeEmailCmd(targetID)
}

func (m *tuiModel) populateSummaryMetadata(targetID email.ID) {
	for _, em := range m.emails {
		if em.ID == targetID {
			m.summaryFrom = em.FromRaw
			m.summarySubject = em.Subject
			m.summaryDate = em.FormattedDate
			return
		}
	}
	cfg := m.Config()
	if cfg == nil {
		cfg = m.cfg
	}
	if cfg != nil {
		if data, err := msg.Read(cfg.AccountCache, targetID); err == nil {
			if parsedMail, err := mail.ReadMessage(bytes.NewReader(data)); err == nil {
				dec := new(mime.WordDecoder)
				m.summarySubject = email.DecDecode(dec, parsedMail.Header.Get("Subject"))
				m.summaryFrom = email.DecDecode(dec, parsedMail.Header.Get("From"))
				m.summaryDate = parsedMail.Header.Get("Date")
			}
		}
	}
}

func (m *tuiModel) summarizeEmailCmd(id email.ID) tea.Cmd {
	return func() tea.Msg {
		cfg := m.Config()
		if cfg == nil {
			cfg = m.cfg
		}
		if cfg == nil {
			return summaryLoadedMsg{id: id, err: fmt.Errorf("configuration not loaded")}
		}

		downloadDir := cfg.AccountCache

		// Through the accessor: summarizing runs in the background, so a
		// download here costs nothing the user is waiting on.
		data, err := m.client.GetRawMessage(id)
		if err != nil {
			return summaryLoadedMsg{id: id, err: fmt.Errorf("could not retrieve email content: %w", err)}
		}

		parsedMail, err := mail.ReadMessage(bytes.NewReader(data))
		if err != nil {
			return summaryLoadedMsg{id: id, err: fmt.Errorf("failed to parse email: %w", err)}
		}

		dec := new(mime.WordDecoder)
		subj := email.DecDecode(dec, parsedMail.Header.Get("Subject"))
		frm := email.DecDecode(dec, parsedMail.Header.Get("From"))
		dateStr := parsedMail.Header.Get("Date")
		bodyText, _ := email.ExtractPlainBodyText(parsedMail)

		// Check on-disk cache first
		if cached, readErr := msg.ReadSummary(downloadDir, id); readErr == nil && strings.TrimSpace(cached) != "" {
			return summaryLoadedMsg{
				id:      id,
				from:    frm,
				subject: subj,
				date:    dateStr,
				summary: cached,
			}
		}

		emailPayload := fmt.Sprintf("From: %s\nSubject: %s\nDate: %s\n\n%s", frm, subj, dateStr, bodyText)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		summary, err := llm.SummarizeEmail(ctx, cfg.LLM, emailPayload)
		if err != nil {
			return summaryLoadedMsg{id: id, from: frm, subject: subj, date: dateStr, err: err}
		}

		// Store summary on disk in the cache directory
		_ = msg.StoreSummary(downloadDir, id, summary)

		return summaryLoadedMsg{
			id:      id,
			from:    frm,
			subject: subj,
			date:    dateStr,
			summary: summary,
		}
	}
}
