package mailclient

import (
	"bytes"
	"context"
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
)

type movedEmailRecord struct {
	MessageIDs  []email.ID
	SourceLabel string
	DestLabel   string
}

type copiedEmailRecord struct {
	MessageIDs  []email.ID
	SourceLabel string
	DestLabel   string
}

type uploadedEmailRecord struct {
	RawBytes    []byte
	TargetLabel string
}

// MockMailClient is an in-memory MailClient for tests: it serves canned data and
// records what it was asked to do so a test can assert on it. A method fails
// when FailOn names it, falling back to Err; FailOnMessage rejects individual
// IDs inside a batch, and the batch methods record only the IDs they accepted.
type MockMailClient struct {
	LabelItems        []cfg_acc.Item
	Labels            []string
	Cfg               *cfg_g.Config
	DownloadedIDs     map[string][]email.ID
	MovedEmails       []movedEmailRecord
	CopiedEmails      []copiedEmailRecord
	UploadedEmails    []uploadedEmailRecord
	SentRawEmails     [][]byte
	AccountLatestRefs []cfg_acc.MessageFolderRef
	Err               error
	InboxFolderVal    string
	BackendTypeVal    string

	// FailOn makes one named method fail while the rest keep working, so a
	// test can prove a command survives (or correctly aborts on) a single
	// backend failure. Keys are Go method names, e.g. "MoveEmail". A method
	// with no entry falls back to Err.
	FailOn map[string]error

	// FailOnMessage makes a batch operation fail for specific message IDs
	// only. The mock records the IDs that succeeded and returns the first
	// error, which is what a loop-based backend does mid-batch. Without this,
	// a partial failure cannot be expressed at all: Err fails everything.
	FailOnMessage map[email.ID]error

	// RawMessages supplies message bodies by ID for the MessageAccessor methods.
	RawMessages map[email.ID][]byte

	// Filters is returned by GetFilters.
	Filters FilterListing

	// DeletedMessages records IDs passed to DeleteMessages.
	DeletedMessages []email.ID
	// DeletedFilters records IDs passed to DeleteFilter.
	DeletedFilters []string

	// SearchResults supplies search hits by query string; the empty-string key
	// is a catch-all.
	SearchResults map[string][]email.ID

	// LastSearchQuery and LastSearchLimit record the most recent search, so a
	// test can prove --all asked the backend for everything rather than
	// trimming the results afterwards.
	LastSearchQuery string
	LastSearchLimit int

	// PoliticalSpamActed records the argument the last ShowPoliticalSpam call
	// was given: false audits, true acts.
	PoliticalSpamActed bool

	// Mutation call counters. Tests assert these are zero to prove a command
	// performed no server mutation (e.g. under --read-only).
	ExportRulesCalls       int
	ShowPoliticalSpamCalls int
	LearnSpamCalls         int
	ReportSpamCalls        int
	MoveAllSpamCalls       int
	LastMoveAllSpamDest    string
	DeleteAllSpamCalls     int
}

// failure resolves the error for a method: its FailOn entry if present,
// otherwise the blanket Err.
func (m *MockMailClient) failure(method string) error {
	if err, ok := m.FailOn[method]; ok {
		return err
	}
	return m.Err
}

// partition splits message IDs into those the mock accepts and the first error
// among those it rejects. With no per-message failures configured it returns
// the input untouched, so behavior is unchanged for tests that do not use it.
func (m *MockMailClient) partition(messageIDs []email.ID) ([]email.ID, error) {
	if len(m.FailOnMessage) == 0 {
		return messageIDs, nil
	}
	var accepted []email.ID
	var firstErr error
	for _, id := range messageIDs {
		err, rejected := m.FailOnMessage[id]
		if !rejected {
			accepted = append(accepted, id)
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return accepted, firstErr
}

// Validate reports the failure configured for it via FailOn or Err, or nil.
func (m *MockMailClient) Validate() error { return m.failure("Validate") }

// GetMatchingLabels returns the labels whose names begin with prefix,
// case-insensitively; an empty prefix returns them all.
func (m *MockMailClient) GetMatchingLabels(prefix string) ([]string, error) {
	if err := m.failure("GetMatchingLabels"); err != nil {
		return nil, err
	}
	if prefix == "" {
		return m.Labels, nil
	}
	var matched []string
	prefixLower := strings.ToLower(prefix)
	for _, l := range m.Labels {
		if strings.HasPrefix(strings.ToLower(l), prefixLower) {
			matched = append(matched, l)
		}
	}
	return matched, nil
}

// FetchAndDownloadEmails returns the IDs recorded under the folder's name in
// DownloadedIDs, or nil when nothing is recorded for that folder.
func (m *MockMailClient) FetchAndDownloadEmails(folder label.Folder, _ string) ([]email.ID, error) {
	if err := m.failure("FetchAndDownloadEmails"); err != nil {
		return nil, err
	}
	if m.DownloadedIDs == nil {
		return nil, nil
	}
	return m.DownloadedIDs[folder.String()], nil
}

// FetchLatestAccountEmails returns AccountLatestRefs truncated to limit, or,
// when none are set, refs derived from DownloadedIDs.
func (m *MockMailClient) FetchLatestAccountEmails(limit int) ([]cfg_acc.MessageFolderRef, error) {
	if err := m.failure("FetchLatestAccountEmails"); err != nil {
		return nil, err
	}
	if len(m.AccountLatestRefs) > 0 {
		refs := m.AccountLatestRefs
		if limit > 0 && len(refs) > limit {
			refs = refs[:limit]
		}
		return refs, nil
	}
	var refs []cfg_acc.MessageFolderRef
	for folder, ids := range m.DownloadedIDs {
		for _, id := range ids {
			refs = append(refs, cfg_acc.MessageFolderRef{
				MessageID: id,
				Folder:    folder,
			})
		}
	}
	if limit > 0 && len(refs) > limit {
		refs = refs[:limit]
	}
	return refs, nil
}

// CheckAndApplyRules returns the IDs it accepts, rejecting only those listed in
// FailOnMessage.
func (m *MockMailClient) CheckAndApplyRules(messageIDs []email.ID, _ label.Folder, _ string) ([]email.ID, error) {
	if err := m.failure("CheckAndApplyRules"); err != nil {
		return nil, err
	}
	accepted, err := m.partition(messageIDs)
	return accepted, err
}

// ReportSpam counts the call and fails for any ID in FailOnMessage, but records
// no move: the mock keeps no spam folder.
func (m *MockMailClient) ReportSpam(messageIDs []email.ID, _ label.Folder) error {
	m.ReportSpamCalls++
	if err := m.failure("ReportSpam"); err != nil {
		return err
	}
	_, err := m.partition(messageIDs)
	return err
}

// MoveToInbox moves nothing and counts nothing; it only fails for IDs listed in
// FailOnMessage.
func (m *MockMailClient) MoveToInbox(messageIDs []email.ID, _ label.Folder) error {
	if err := m.failure("MoveToInbox"); err != nil {
		return err
	}
	_, err := m.partition(messageIDs)
	return err
}

// MoveEmail records the accepted IDs in MovedEmails with their source and
// destination, and returns the first per-message error.
func (m *MockMailClient) MoveEmail(messageIDs []email.ID, source, dest label.Folder) error {
	if err := m.failure("MoveEmail"); err != nil {
		return err
	}
	accepted, err := m.partition(messageIDs)
	if len(accepted) > 0 {
		m.MovedEmails = append(m.MovedEmails, movedEmailRecord{
			MessageIDs:  accepted,
			SourceLabel: source.String(),
			DestLabel:   dest.String(),
		})
	}
	return err
}

// CopyEmail records the accepted IDs in CopiedEmails and adds them to the
// destination's DownloadedIDs, so a later fetch sees the copy.
func (m *MockMailClient) CopyEmail(messageIDs []email.ID, source, dest label.Folder) error {
	if err := m.failure("CopyEmail"); err != nil {
		return err
	}
	accepted, err := m.partition(messageIDs)
	if len(accepted) == 0 {
		return err
	}
	m.CopiedEmails = append(m.CopiedEmails, copiedEmailRecord{
		MessageIDs:  accepted,
		SourceLabel: source.String(),
		DestLabel:   dest.String(),
	})
	if m.DownloadedIDs == nil {
		m.DownloadedIDs = make(map[string][]email.ID)
	}
	m.DownloadedIDs[dest.String()] = append(m.DownloadedIDs[dest.String()], accepted...)
	return err
}

// MoveAllSpam increments MoveAllSpamCalls and records dest in
// LastMoveAllSpamDest.
func (m *MockMailClient) MoveAllSpam(dest label.Folder) error {
	m.MoveAllSpamCalls++
	m.LastMoveAllSpamDest = dest.String()
	return m.failure("MoveAllSpam")
}

// DeleteAllSpam increments DeleteAllSpamCalls, letting a test prove a purge was
// requested.
func (m *MockMailClient) DeleteAllSpam() error {
	m.DeleteAllSpamCalls++
	return m.failure("DeleteAllSpam")
}

// ShowPoliticalSpam counts the call and records act in PoliticalSpamActed, so a
// test can tell an audit from an action.
func (m *MockMailClient) ShowPoliticalSpam(act bool) error {
	m.ShowPoliticalSpamCalls++
	m.PoliticalSpamActed = act
	return m.failure("ShowPoliticalSpam")
}

// LearnSpam increments LearnSpamCalls and returns the configured failure.
func (m *MockMailClient) LearnSpam() error {
	m.LearnSpamCalls++
	return m.failure("LearnSpam")
}

// ListLabels returns the failure configured for it and keeps no label listing.
func (m *MockMailClient) ListLabels() error { return m.failure("ListLabels") }

// RenameLabel returns the failure configured for it and changes no label.
func (m *MockMailClient) RenameLabel(_, _ label.Folder) error { return m.failure("RenameLabel") }

// FixLabels returns the failure configured for it and repairs nothing.
func (m *MockMailClient) FixLabels() error { return m.failure("FixLabels") }

// DeleteMessages records the IDs in DeletedMessages before returning any
// configured failure.
func (m *MockMailClient) DeleteMessages(messageIDs []email.ID, _ label.Folder) error {
	if err := m.failure("DeleteMessages"); err != nil {
		return err
	}
	m.DeletedMessages = append(m.DeletedMessages, messageIDs...)
	return nil
}

// DeleteLabel returns the failure configured for it and deletes nothing.
func (m *MockMailClient) DeleteLabel(_ label.Folder) error { return m.failure("DeleteLabel") }

// ExportRules increments ExportRulesCalls, letting a test prove no rules were
// pushed to the server.
func (m *MockMailClient) ExportRules() error {
	m.ExportRulesCalls++
	return m.failure("ExportRules")
}

// GetFilters returns the Filters configured on the mock.
func (m *MockMailClient) GetFilters() (FilterListing, error) {
	if err := m.failure("GetFilters"); err != nil {
		return FilterListing{}, err
	}
	return m.Filters, nil
}

// DeleteFilter records id in DeletedFilters before returning any configured
// failure.
func (m *MockMailClient) DeleteFilter(id string) error {
	m.DeletedFilters = append(m.DeletedFilters, id)
	return m.failure("DeleteFilter")
}

// Config returns the config the test installed, which may be nil.
func (m *MockMailClient) Config() *cfg_g.Config { return m.Cfg }

// RewriteRuleLabelCasing returns an empty string; the mock does not model label
// casing.
func (m *MockMailClient) RewriteRuleLabelCasing(_ string) string { return "" }

// EnsureLabelExists returns the failure configured for it and creates nothing.
func (m *MockMailClient) EnsureLabelExists(_ label.Folder) error {
	return m.failure("EnsureLabelExists")
}

// GetLabelItems returns the LabelItems configured on the mock.
func (m *MockMailClient) GetLabelItems() ([]cfg_acc.Item, error) {
	if err := m.failure("GetLabelItems"); err != nil {
		return nil, err
	}
	return m.LabelItems, nil
}

// MarkAsRead returns the first per-message failure from FailOnMessage, or nil.
func (m *MockMailClient) MarkAsRead(messageIDs []email.ID) error {
	if err := m.failure("MarkAsRead"); err != nil {
		return err
	}
	_, err := m.partition(messageIDs)
	return err
}

// UploadRawEmail records the message in UploadedEmails, stores it in the cache
// when one is configured, and returns the generated ID.
func (m *MockMailClient) UploadRawEmail(rawBytes []byte, target label.Folder) (email.ID, error) {
	targetLabel := target.String()
	if err := m.failure("UploadRawEmail"); err != nil {
		return "", err
	}
	m.UploadedEmails = append(m.UploadedEmails, uploadedEmailRecord{
		RawBytes:    rawBytes,
		TargetLabel: targetLabel,
	})
	if m.DownloadedIDs == nil {
		m.DownloadedIDs = make(map[string][]email.ID)
	}
	mockID := email.ID(fmt.Sprintf("upload_msg_%d", len(m.UploadedEmails)))
	if m.Cfg != nil && !m.Cfg.AccountCache.IsZero() {
		_ = msg.Store(m.Cfg.AccountCache, mockID, rawBytes, time.Now())
	}
	m.DownloadedIDs[targetLabel] = append(m.DownloadedIDs[targetLabel], mockID)
	return mockID, nil
}

// SendEmail appends the raw message to SentRawEmails.
func (m *MockMailClient) SendEmail(rawBytes []byte) error {
	if err := m.failure("SendEmail"); err != nil {
		return err
	}
	m.SentRawEmails = append(m.SentRawEmails, rawBytes)
	return nil
}

// InboxFolder returns InboxFolderVal, else the config's ReceivedFolder, else
// "Inbox".
func (m *MockMailClient) InboxFolder() label.Folder {
	if m.InboxFolderVal != "" {
		return label.FromServer(m.InboxFolderVal)
	}
	if m.Cfg != nil && m.Cfg.ReceivedFolder != "" {
		return label.FromServer(m.Cfg.ReceivedFolder)
	}
	return label.FromServer("Inbox")
}

// BackendType returns BackendTypeVal, or "mock" when it is unset.
func (m *MockMailClient) BackendType() string {
	if m.BackendTypeVal != "" {
		return m.BackendTypeVal
	}
	return "mock"
}

// RawMessages lets a test supply message bodies by ID. The accessors below
// serve these first, then fall back to whatever is in the cache directory, so a
// test can work either by seeding this map or by seeding a real temp cache.
func (m *MockMailClient) rawMessage(msgID email.ID) ([]byte, error) {
	if raw, ok := m.RawMessages[msgID]; ok {
		return raw, nil
	}
	if m.Cfg != nil && !m.Cfg.AccountCache.IsZero() {
		if data, err := msg.Read(m.Cfg.AccountCache, msgID); err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("mock: no message %q", msgID)
}

// GetRawMessage returns the body from RawMessages, falling back to the cache,
// and errors when neither holds the message.
func (m *MockMailClient) GetRawMessage(msgID email.ID) ([]byte, error) {
	if err := m.failure("GetRawMessage"); err != nil {
		return nil, err
	}
	if err, rejected := m.FailOnMessage[msgID]; rejected {
		return nil, err
	}
	return m.rawMessage(msgID)
}

// GetRawMessagesBatch returns one entry per ID in order, nil where retrieval
// failed, and an error counting the failures.
func (m *MockMailClient) GetRawMessagesBatch(msgIDs []email.ID) ([][]byte, error) {
	if err := m.failure("GetRawMessagesBatch"); err != nil {
		return nil, err
	}
	out := make([][]byte, len(msgIDs))
	var failed int
	for i, id := range msgIDs {
		raw, err := m.GetRawMessage(id)
		if err != nil {
			failed++
			continue
		}
		out[i] = raw
	}
	if failed > 0 {
		return out, fmt.Errorf("mock: %d of %d message(s) could not be retrieved", failed, len(msgIDs))
	}
	return out, nil
}

// GetMessageSummary parses the message body and returns its header metadata.
func (m *MockMailClient) GetMessageSummary(msgID email.ID) (*email.Email, error) {
	raw, err := m.GetRawMessage(msgID)
	if err != nil {
		return nil, err
	}
	parsed := email.ParseReader(bytes.NewReader(raw), msgID, "")
	if parsed == nil {
		return nil, fmt.Errorf("mock: message %q does not parse", msgID)
	}
	return parsed, nil
}

// GetMessageSummariesBatch returns one summary per ID in order, nil where the
// message could not be retrieved, and an error counting the failures.
func (m *MockMailClient) GetMessageSummariesBatch(msgIDs []email.ID) ([]*email.Email, error) {
	if err := m.failure("GetMessageSummariesBatch"); err != nil {
		return nil, err
	}
	out := make([]*email.Email, len(msgIDs))
	var failed int
	for i, id := range msgIDs {
		summary, err := m.GetMessageSummary(id)
		if err != nil {
			failed++
			continue
		}
		out[i] = summary
	}
	if failed > 0 {
		return out, fmt.Errorf("mock: %d of %d summary(ies) could not be retrieved", failed, len(msgIDs))
	}
	return out, nil
}

// SearchEmails records the query and limit, then returns SearchResults for that
// query, falling back to the empty-string catch-all for tests that do not care
// about the query; a positive limit truncates the result.
func (m *MockMailClient) SearchEmails(query string, limit int) ([]email.ID, error) {
	m.LastSearchQuery = query
	m.LastSearchLimit = limit
	if err := m.failure("SearchEmails"); err != nil {
		return nil, err
	}
	ids, ok := m.SearchResults[query]
	if !ok {
		ids = m.SearchResults[""]
	}
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

// PrefetchMessages fails only for IDs listed in FailOnMessage; nothing is
// downloaded because the bodies already live in RawMessages.
func (m *MockMailClient) PrefetchMessages(msgIDs []email.ID) error {
	if err := m.failure("PrefetchMessages"); err != nil {
		return err
	}
	_, err := m.partition(msgIDs)
	return err
}

// GetRawMessageContext returns ctx's error if it is already done, then delegates
// to GetRawMessage.
func (m *MockMailClient) GetRawMessageContext(ctx context.Context, msgID email.ID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.GetRawMessage(msgID)
}

// GetRawMessagesBatchContext returns ctx's error if it is already done, then
// delegates to GetRawMessagesBatch.
func (m *MockMailClient) GetRawMessagesBatchContext(ctx context.Context, msgIDs []email.ID) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.GetRawMessagesBatch(msgIDs)
}

// GetMessageSummaryContext returns ctx's error if it is already done, then
// delegates to GetMessageSummary.
func (m *MockMailClient) GetMessageSummaryContext(ctx context.Context, msgID email.ID) (*email.Email, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.GetMessageSummary(msgID)
}

// GetMessageSummariesBatchContext returns ctx's error if it is already done,
// then delegates to GetMessageSummariesBatch.
func (m *MockMailClient) GetMessageSummariesBatchContext(ctx context.Context, msgIDs []email.ID) ([]*email.Email, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.GetMessageSummariesBatch(msgIDs)
}

// PrefetchMessagesContext returns ctx's error if it is already done, then
// delegates to PrefetchMessages.
func (m *MockMailClient) PrefetchMessagesContext(ctx context.Context, msgIDs []email.ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.PrefetchMessages(msgIDs)
}
