package mailclient

import (
	"errors"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"log/slog"
)

// ErrReadOnlyOperationBlocked is the error a mutating method returns when it is
// refused outright under read-only mode rather than simulated. Compare with
// errors.Is; the read-only decorator returns this exact value.
var ErrReadOnlyOperationBlocked = errors.New("operation blocked: account is in read-only mode")

// ReadOnlyMailClient decorates a MailClient to prevent any remote mutations.
//
// The embedded Reader carries every method that cannot change server state, so
// a read added to the interface passes through without an edit here. Every
// Mutator method is written out below, which means a mutating method added to
// the interface will not compile until it has been given a decision: block it,
// or simulate it. That asymmetry is deliberate -- a read this decorator forgets
// to forward is an inconvenience, a write it forgets to block is a wrong answer
// to --read-only.
type ReadOnlyMailClient struct {
	Reader

	Delegate    MailClient
	AccountName string
}

// NewReadOnlyMailClient wraps delegate so that reads reach it unchanged while
// every server mutation is either refused with ErrReadOnlyOperationBlocked or
// simulated, with accountName used only in the log lines it emits.
func NewReadOnlyMailClient(delegate MailClient, accountName string) *ReadOnlyMailClient {
	return &ReadOnlyMailClient{
		Reader:      delegate,
		Delegate:    delegate,
		AccountName: accountName,
	}
}

// simulatedTransfer logs a simulated move or copy of ids from source to dest.
func (r *ReadOnlyMailClient) simulatedTransfer(op string, ids []email.ID, source, dest label.Folder) error {
	slog.Info("[READ-ONLY] Simulated "+op, slog.String("account", r.AccountName), slog.Int("count", len(ids)), slog.String("from", source.String()), slog.String("to", dest.String()))
	return nil
}

// simulatedMoveFrom logs a simulated move of ids out of source, an operation
// with no destination to report.
func (r *ReadOnlyMailClient) simulatedMoveFrom(op string, ids []email.ID, source label.Folder) error {
	slog.Info("[READ-ONLY] Simulated "+op, slog.String("account", r.AccountName), slog.Int("count", len(ids)), slog.String("from", source.String()))
	return nil
}

// RenameLabel returns nil without contacting the server; the label keeps its
// name on the server and in any local index.
func (r *ReadOnlyMailClient) RenameLabel(oldName, newName label.Folder) error {
	slog.Info("[READ-ONLY] Simulated RenameLabel", slog.String("account", r.AccountName), slog.String("old", oldName.String()), slog.String("new", newName.String()))
	return nil
}

// FixLabels returns nil without repairing anything on the server.
func (r *ReadOnlyMailClient) FixLabels() error {
	slog.Info("[READ-ONLY] Simulated FixLabels", slog.String("account", r.AccountName))
	return nil
}

// DeleteLabel is blocked rather than simulated: it returns
// ErrReadOnlyOperationBlocked and leaves the label in place.
func (r *ReadOnlyMailClient) DeleteLabel(name label.Folder) error {
	slog.Warn("[READ-ONLY] DeleteLabel blocked", slog.String("account", r.AccountName), slog.String("label", name.String()))
	return ErrReadOnlyOperationBlocked
}

// EnsureLabelExists returns nil without creating the folder on the server, so a
// later upload into it fails at the server rather than here.
func (r *ReadOnlyMailClient) EnsureLabelExists(name label.Folder) error {
	slog.Info("[READ-ONLY] Simulated EnsureLabelExists", slog.String("account", r.AccountName), slog.String("label", name.String()))
	return nil
}

// MoveEmail returns nil without moving anything; the messages stay in source on
// the server.
func (r *ReadOnlyMailClient) MoveEmail(messageIDs []email.ID, source, dest label.Folder) error {
	return r.simulatedTransfer("MoveEmail", messageIDs, source, dest)
}

// CopyEmail returns nil without copying anything to dest on the server.
func (r *ReadOnlyMailClient) CopyEmail(messageIDs []email.ID, source, dest label.Folder) error {
	return r.simulatedTransfer("CopyEmail", messageIDs, source, dest)
}

// MarkAsRead returns nil without changing any message's read state on the server.
func (r *ReadOnlyMailClient) MarkAsRead(messageIDs []email.ID) error {
	slog.Debug("[READ-ONLY] Simulated MarkAsRead", slog.String("account", r.AccountName), slog.Int("count", len(messageIDs)))
	return nil
}

// MoveToInbox returns nil without moving the messages out of source on the server.
func (r *ReadOnlyMailClient) MoveToInbox(messageIDs []email.ID, source label.Folder) error {
	return r.simulatedMoveFrom("MoveToInbox", messageIDs, source)
}

// UploadRawEmail is blocked rather than simulated: it returns an empty ID and
// ErrReadOnlyOperationBlocked, and no message is stored.
func (r *ReadOnlyMailClient) UploadRawEmail(_ []byte, _ label.Folder) (email.ID, error) {
	slog.Warn("[READ-ONLY] UploadRawEmail blocked", slog.String("account", r.AccountName))
	return "", ErrReadOnlyOperationBlocked
}

// DeleteMessages is blocked rather than simulated: it returns
// ErrReadOnlyOperationBlocked and deletes nothing.
func (r *ReadOnlyMailClient) DeleteMessages(messageIDs []email.ID, _ label.Folder) error {
	slog.Warn("[READ-ONLY] DeleteMessages blocked", slog.String("account", r.AccountName), slog.Int("count", len(messageIDs)))
	return ErrReadOnlyOperationBlocked
}

// ReportSpam returns nil without moving the messages to the spam folder on the
// server.
func (r *ReadOnlyMailClient) ReportSpam(messageIDs []email.ID, source label.Folder) error {
	return r.simulatedMoveFrom("ReportSpam", messageIDs, source)
}

// MoveAllSpam returns nil without moving any message out of the spam folder on
// the server.
func (r *ReadOnlyMailClient) MoveAllSpam(dest label.Folder) error {
	slog.Info("[READ-ONLY] Simulated MoveAllSpam", slog.String("account", r.AccountName), slog.String("dest", dest.String()))
	return nil
}

// DeleteAllSpam is blocked rather than simulated: it returns
// ErrReadOnlyOperationBlocked and deletes nothing, permanently or otherwise.
func (r *ReadOnlyMailClient) DeleteAllSpam() error {
	slog.Warn("[READ-ONLY] DeleteAllSpam blocked", slog.String("account", r.AccountName))
	return ErrReadOnlyOperationBlocked
}

// ShowPoliticalSpam delegates so the report is still produced, with acting
// suppressed. The backends additionally gate their unsubscribe and delete paths
// on Config.ReadOnly, so this is a second lock on the same door rather than the
// only one.
func (r *ReadOnlyMailClient) ShowPoliticalSpam(_ bool) error {
	return r.Delegate.ShowPoliticalSpam(false)
}

// LearnSpam returns nil without training the spam filter, server-side or local.
func (r *ReadOnlyMailClient) LearnSpam() error {
	slog.Info("[READ-ONLY] Simulated LearnSpam", slog.String("account", r.AccountName))
	return nil
}

// CheckAndApplyRules returns messageIDs unchanged and nil, reporting every
// message as accepted without applying a rule to any of them.
func (r *ReadOnlyMailClient) CheckAndApplyRules(messageIDs []email.ID, _ label.Folder, _ string) ([]email.ID, error) {
	slog.Info("[READ-ONLY] Simulated CheckAndApplyRules", slog.String("account", r.AccountName), slog.Int("count", len(messageIDs)))
	return messageIDs, nil
}

// ExportRules is a server mutation, not a read: on Gmail it creates and deletes
// real server-side filters. It must never reach the delegate in read-only mode.
func (r *ReadOnlyMailClient) ExportRules() error {
	slog.Warn("[READ-ONLY] ExportRules blocked", slog.String("account", r.AccountName))
	return ErrReadOnlyOperationBlocked
}

// DeleteFilter is blocked rather than simulated: it returns
// ErrReadOnlyOperationBlocked and leaves the server-side rule in place.
func (r *ReadOnlyMailClient) DeleteFilter(id string) error {
	slog.Warn("[READ-ONLY] DeleteFilter blocked", slog.String("account", r.AccountName), slog.String("id", id))
	return ErrReadOnlyOperationBlocked
}

// SendEmail is blocked rather than simulated: it returns
// ErrReadOnlyOperationBlocked and transmits nothing.
func (r *ReadOnlyMailClient) SendEmail(_ []byte) error {
	slog.Warn("[READ-ONLY] SendEmail blocked", slog.String("account", r.AccountName))
	return ErrReadOnlyOperationBlocked
}

// Retrieving a message is a read, so the accessors pass straight through. They
// write to the local cache, which --read-only permits: a downloaded body is a
// fact the server reported, not a change to it.
