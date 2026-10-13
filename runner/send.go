package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cli"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/mailops"
	"github.com/sarielhp/mail_cli/outbox"
	"github.com/sarielhp/mail_cli/pgp"
	"github.com/sarielhp/mail_cli/report"
)

// SendService implements cli.SendOps to deliver outbound emails.
type SendService struct {
	Rep report.Reporter
	Out io.Writer
	TUI cli.TUILauncher
}

func (s SendService) rep() report.Reporter { return report.Or(s.Rep) }

func (s SendService) out() io.Writer {
	if s.Out != nil {
		return s.Out
	}
	return os.Stdout
}

func (s SendService) tui() cli.TUILauncher {
	if s.TUI != nil {
		return s.TUI
	}
	return TUIService{}
}

// Send resolves the sender account, builds RFC822 MIME message, and transmits it.
func (s SendService) Send(config *cfg_g.Config, opts cli.SendOptions) error {
	if opts.Raw {
		return s.sendRaw(config, opts)
	}

	client, acc, err := mailops.GetClientForSending(config, opts.Account, s.rep())
	if err != nil {
		return err
	}

	sendAddr := acc.GetSendAddress()
	from := sendAddr
	if name := acc.GetSenderName(); name != "" && sendAddr != "" {
		from = fmt.Sprintf("%s <%s>", name, sendAddr)
	}

	toRecipients := expandRecipients(config, opts.To)
	ccRecipients := expandRecipients(config, opts.Cc)
	bccRecipients := expandRecipients(config, opts.Bcc)

	attachments, err := loadAttachments(opts.Attachments)
	if err != nil {
		return err
	}

	comp := &email.ComposeMessage{
		From:        from,
		To:          strings.Join(toRecipients, ", "),
		Cc:          strings.Join(ccRecipients, ", "),
		Bcc:         strings.Join(bccRecipients, ", "),
		Subject:     opts.Subject,
		Body:        opts.Body,
		Attachments: attachments,
	}

	recipients := pgp.Recipients{
		Visible: append(append([]string{}, toRecipients...), ccRecipients...),
		Hidden:  bccRecipients,
	}
	rawBytes, err := s.buildOutbound(comp, opts, *acc, recipients)
	if err != nil {
		return err
	}

	if opts.DryRun {
		_, _ = s.out().Write(rawBytes)
		fmt.Fprintln(s.out())
		s.rep().Infof("[DRY-RUN] RFC822 message constructed (%d bytes) for delivery via %s (%s)", len(rawBytes), acc.Name, acc.Username)
		return nil
	}

	cacheRoot := ""
	if config != nil {
		cacheRoot = config.CacheRoot
	}

	if opts.Queue {
		entry, err := outbox.Enqueue(outbox.Dir(cacheRoot), acc.Name, toRecipients, opts.Subject, rawBytes)
		if err != nil {
			return fmt.Errorf("queueing email to outbox: %w", err)
		}
		s.rep().Successf("Email queued to outbox (ID: %s, Account: %s)", entry.ID, acc.Name)
		return nil
	}

	s.rep().Infof("Sending email via %s (%s)...", acc.Name, acc.Username)
	if err := client.SendEmail(rawBytes); err != nil {
		s.spoolOffline(outbox.Dir(cacheRoot), outbox.Entry{Account: acc.Name, To: toRecipients, Subject: opts.Subject}, rawBytes, err)
		return fmt.Errorf("failed to send email via %s: %w", acc.Name, err)
	}

	contacts.RecordSentRecipients(config, rawBytes)
	s.rep().Successf("Email sent to %s via %s (%s)", strings.Join(toRecipients, ", "), acc.Name, acc.Username)
	return nil
}

// buildOutbound builds the message and applies the PGP protection the caller
// asked for.
//
// Protection happens before the message is printed, queued or spooled, so what
// the user reviews, and what sits in the outbox, is what will be sent.
// Encrypting after spooling would leave a plaintext copy of the message in the
// cache.
func (s SendService) buildOutbound(comp *email.ComposeMessage, opts cli.SendOptions, acc cfg_acc.AccountConfig, recipients pgp.Recipients) ([]byte, error) {
	built, err := comp.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build RFC822 email: %w", err)
	}
	if !opts.Sign && !opts.Encrypt {
		return built.Bytes(), nil
	}

	keyID := opts.PgpKeyID
	if keyID == "" {
		keyID = acc.PgpKeyID
	}

	ctx := context.Background()
	targets := recipients.Visible
	if opts.Encrypt {
		targets = s.encryptTargets(ctx, recipients.Visible, acc.GetSendAddress())
	}

	rawBytes, err := pgp.Protect(ctx, built, pgp.OutboundOptions{
		Sign:             opts.Sign,
		Encrypt:          opts.Encrypt,
		KeyID:            keyID,
		Recipients:       targets,
		HiddenRecipients: recipients.Hidden,
	})
	if err != nil {
		return nil, err
	}
	return rawBytes, nil
}

// encryptTargets is everyone the message has to be readable by: its recipients,
// plus the sender when the keyring holds a key for them, so the copy in Sent
// can be read back.
//
// A sender with no key is dropped rather than fatal -- they may be sending from
// a machine that has never held their own key -- but it is said out loud,
// because the alternative is a sent folder the sender cannot read and no
// explanation of why.
func (s SendService) encryptTargets(ctx context.Context, recipients []string, sender string) []string {
	targets := append([]string{}, recipients...)
	if sender == "" {
		return targets
	}
	if pgp.HasPublicKey(ctx, sender) {
		return append(targets, sender)
	}
	s.rep().Warnf("No public key for %s; the copy saved to Sent will not be readable here", sender)
	return targets
}

func loadAttachments(paths []string) ([]email.Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	attachments := make([]email.Attachment, 0, len(paths))
	for _, p := range paths {
		cleaned := strings.TrimSpace(p)
		if cleaned == "" {
			continue
		}
		data, err := os.ReadFile(cleaned)
		if err != nil {
			return nil, fmt.Errorf("reading attachment %q: %w", cleaned, err)
		}
		cType := mime.TypeByExtension(filepath.Ext(cleaned))
		if cType == "" {
			cType = "application/octet-stream"
		}
		attachments = append(attachments, email.Attachment{
			Filename:    filepath.Base(cleaned),
			ContentType: cType,
			Data:        data,
		})
	}
	return attachments, nil
}

func expandRecipients(config *cfg_g.Config, raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	bookPath := contacts.DefaultPath(config)
	book, _ := contacts.LoadBook(bookPath)

	var expanded []string
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			expanded = append(expanded, resolveSingleRecipient(book, trimmed))
		}
	}
	return expanded
}

func resolveSingleRecipient(book *contacts.Book, item string) string {
	if strings.Contains(item, "@") || book == nil {
		return item
	}
	for _, c := range book.Contacts {
		if matched, ok := matchContactRecipient(c, item); ok {
			return matched
		}
	}
	return item
}

func matchContactRecipient(c contacts.Contact, item string) (string, bool) {
	primary := c.PrimaryEmail()
	if primary == "" {
		return "", false
	}
	if strings.EqualFold(c.Name, item) {
		return formatRecipient(c.Name, primary), true
	}
	for _, nick := range c.Nicknames {
		if strings.EqualFold(nick, item) {
			return formatRecipient(c.Name, primary), true
		}
	}
	return "", false
}

func formatRecipient(name, email string) string {
	if name != "" && email != "" {
		return fmt.Sprintf("%s <%s>", name, email)
	}
	return email
}

func (s SendService) sendRaw(config *cfg_g.Config, opts cli.SendOptions) error {
	msg, err := mail.ReadMessage(bytes.NewReader(opts.RawBytes))
	if err != nil {
		return fmt.Errorf("parsing raw RFC822 message: %w", err)
	}

	targetAccount := opts.Account
	if targetAccount == "" {
		targetAccount = inferAccountFromFromHeader(config, msg.Header.Get("From"))
	}

	client, acc, err := mailops.GetClientForSending(config, targetAccount, s.rep())
	if err != nil {
		return err
	}

	if opts.DryRun {
		_, _ = s.out().Write(opts.RawBytes)
		fmt.Fprintln(s.out())
		s.rep().Infof("[DRY-RUN] Raw RFC822 message (%d bytes) for delivery via %s (%s)", len(opts.RawBytes), acc.Name, acc.Username)
		return nil
	}

	cacheRoot := ""
	if config != nil {
		cacheRoot = config.CacheRoot
	}
	to := msg.Header.Get("To")
	if to == "" {
		to = strings.Join(opts.To, ", ")
	}
	subj := msg.Header.Get("Subject")

	if opts.Queue {
		entry, err := outbox.Enqueue(outbox.Dir(cacheRoot), acc.Name, []string{to}, subj, opts.RawBytes)
		if err != nil {
			return fmt.Errorf("queueing raw email to outbox: %w", err)
		}
		s.rep().Successf("Raw email queued to outbox (ID: %s, Account: %s)", entry.ID, acc.Name)
		return nil
	}

	s.rep().Infof("Sending raw email via %s (%s)...", acc.Name, acc.Username)
	if err := client.SendEmail(opts.RawBytes); err != nil {
		s.spoolOffline(outbox.Dir(cacheRoot), outbox.Entry{Account: acc.Name, To: []string{to}, Subject: subj}, opts.RawBytes, err)
		return fmt.Errorf("failed to send raw email via %s: %w", acc.Name, err)
	}

	contacts.RecordSentRecipients(config, opts.RawBytes)
	s.rep().Successf("Raw email sent to %s via %s (%s)", to, acc.Name, acc.Username)
	return nil
}

func inferAccountFromFromHeader(config *cfg_g.Config, fromHeader string) string {
	if config == nil || fromHeader == "" {
		return ""
	}
	addr, err := mail.ParseAddress(fromHeader)
	if err != nil {
		return ""
	}
	for _, acc := range config.Accounts {
		if strings.EqualFold(acc.Username, addr.Address) ||
			(acc.UserEmail != "" && strings.EqualFold(acc.UserEmail, addr.Address)) {
			return acc.Name
		}
	}
	return ""
}

type replyPlan struct {
	Account       cfg_acc.AccountConfig
	Client        mailclient.MailClient
	ResolvedMsgID email.ID
	Details       email.ReplyDetails
	Subject       string
	Cc            []string
	QuotedBody    string
	Opts          cli.ReplyOptions
}

// Reply resolves the target message, formats reply headers and quote, and delivers or launches compose.
func (s SendService) Reply(config *cfg_g.Config, opts cli.ReplyOptions) error {
	client, acc, resolvedMsgID, rawBytes, err := findMessageAcrossAccounts(config, opts.Account, opts.MessageID, s.rep())
	if err != nil {
		return err
	}

	origMsg, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	if err != nil {
		return fmt.Errorf("parsing original message: %w", err)
	}

	details := email.BuildReplyDetails(origMsg, acc.GetSendAddress(), opts.Group)
	subject := details.Subject
	if opts.Subject != "" {
		subject = opts.Subject
	}

	plan := replyPlan{
		Account:       acc,
		Client:        client,
		ResolvedMsgID: resolvedMsgID,
		Details:       details,
		Subject:       subject,
		Cc:            append(details.Cc, opts.Cc...),
		QuotedBody:    details.QuotedBody,
		Opts:          opts,
	}
	if opts.NoQuote {
		plan.QuotedBody = ""
	}

	if !opts.Headless {
		return s.replyInteractive(config, plan)
	}
	return s.replyHeadless(config, plan)
}

func (s SendService) replyInteractive(config *cfg_g.Config, plan replyPlan) error {
	composeBody := plan.Opts.Body
	if composeBody != "" && plan.QuotedBody != "" {
		composeBody = composeBody + "\n\n" + plan.QuotedBody
	} else if plan.QuotedBody != "" {
		composeBody = plan.QuotedBody
	}

	composeOpts := cli.ComposeOptions{
		To:          []string{plan.Details.To},
		Cc:          plan.Cc,
		Bcc:         plan.Opts.Bcc,
		Subject:     plan.Subject,
		Body:        composeBody,
		Attachments: plan.Opts.Attachments,
		Account:     plan.Account.Name,
		InReplyTo:   plan.Details.InReplyTo,
		References:  plan.Details.References,
		OriginalID:  plan.ResolvedMsgID,
	}

	return s.tui().Compose(config, composeOpts)
}

func (s SendService) replyHeadless(config *cfg_g.Config, plan replyPlan) error {
	finalBody := plan.Opts.Body
	if !plan.Opts.NoQuote && plan.QuotedBody != "" {
		if finalBody != "" {
			finalBody = finalBody + "\n\n" + plan.QuotedBody
		} else {
			finalBody = plan.QuotedBody
		}
	}

	attachments, err := loadAttachments(plan.Opts.Attachments)
	if err != nil {
		return err
	}

	sendAddr := plan.Account.GetSendAddress()
	from := sendAddr
	if name := plan.Account.GetSenderName(); name != "" && sendAddr != "" {
		from = fmt.Sprintf("%s <%s>", name, sendAddr)
	}

	expandedTo := expandRecipients(config, []string{plan.Details.To})
	expandedCc := expandRecipients(config, plan.Cc)
	expandedBcc := expandRecipients(config, plan.Opts.Bcc)

	comp := &email.ComposeMessage{
		From:        from,
		To:          strings.Join(expandedTo, ", "),
		Cc:          strings.Join(expandedCc, ", "),
		Bcc:         strings.Join(expandedBcc, ", "),
		Subject:     plan.Subject,
		InReplyTo:   plan.Details.InReplyTo,
		References:  plan.Details.References,
		Body:        finalBody,
		Attachments: attachments,
	}

	rawRFC822, err := comp.BuildRFC822()
	if err != nil {
		return fmt.Errorf("failed to build RFC822 reply: %w", err)
	}

	if plan.Opts.DryRun {
		_, _ = s.out().Write(rawRFC822)
		fmt.Fprintln(s.out())
		s.rep().Infof("[DRY-RUN] RFC822 reply message constructed (%d bytes) for delivery via %s (%s)", len(rawRFC822), plan.Account.Name, plan.Account.Username)
		return nil
	}

	s.rep().Infof("Sending reply via %s (%s)...", plan.Account.Name, plan.Account.Username)
	if err := plan.Client.SendEmail(rawRFC822); err != nil {
		return fmt.Errorf("failed to send reply via %s: %w", plan.Account.Name, err)
	}

	contacts.RecordSentRecipients(config, rawRFC822)
	if plan.ResolvedMsgID != "" && plan.Client != nil && plan.Client.Config() != nil && !plan.Client.Config().AccountCache.IsZero() {
		_ = cache.MarkIDsReplied(plan.Client.Config().AccountCache, []email.ID{plan.ResolvedMsgID}, nil)
	}
	s.rep().Successf("Reply sent to %s via %s (%s)", strings.Join(expandedTo, ", "), plan.Account.Name, plan.Account.Username)
	return nil
}

func findCandidateAccounts(config *cfg_g.Config, overrideAccount string) ([]cfg_acc.AccountConfig, error) {
	if config == nil || len(config.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts configured")
	}
	if overrideAccount != "" {
		for _, acc := range config.Accounts {
			if strings.EqualFold(acc.Name, overrideAccount) {
				return []cfg_acc.AccountConfig{acc}, nil
			}
		}
		return nil, fmt.Errorf("account %q not found", overrideAccount)
	}

	primary := string(config.SelectedAccount)
	if primary == "" {
		primary = config.DefaultSendAccount
	}

	var ordered []cfg_acc.AccountConfig
	for _, acc := range config.Accounts {
		if primary != "" && strings.EqualFold(acc.Name, primary) {
			ordered = append([]cfg_acc.AccountConfig{acc}, ordered...)
		} else {
			ordered = append(ordered, acc)
		}
	}
	return ordered, nil
}

func accountCacheDir(config *cfg_g.Config, acc cfg_acc.AccountConfig) cachedir.Account {
	return config.AccountCacheFor(acc)
}

// findMessageAcrossAccounts resolves ref, a message ID as the user typed it,
// to the message and the account holding it.
func findMessageAcrossAccounts(config *cfg_g.Config, accountName, ref string, rep report.Reporter) (mailclient.MailClient, cfg_acc.AccountConfig, email.ID, []byte, error) {
	candidates, err := findCandidateAccounts(config, accountName)
	if err != nil {
		return nil, cfg_acc.AccountConfig{}, "", nil, err
	}

	for _, acc := range candidates {
		dlDir := accountCacheDir(config, acc)
		if dlDir.IsZero() {
			continue
		}
		foundID, _, errFind := cache.FindCachedEmailByID(dlDir, ref)
		if errFind != nil || foundID == "" {
			continue
		}
		raw, errRaw := msg.Read(dlDir, foundID)
		if errRaw != nil || len(raw) == 0 {
			continue
		}
		client, accCfg, errCl := mailops.GetClientForSending(config, acc.Name, rep)
		if errCl != nil {
			continue
		}
		return client, *accCfg, foundID, raw, nil
	}

	// Only a full ID can be asked of a server; a short one is a hash of it.
	if short, ok := email.ParseShortID(ref); ok {
		return nil, cfg_acc.AccountConfig{}, "", nil, errShortIDNotCached(short)
	}

	// Nothing local knows ref and it is not a short ID, so it can only be a
	// full server ID.
	id := email.ID(ref)
	for _, acc := range candidates {
		client, accCfg, errCl := mailops.GetClientForSending(config, acc.Name, rep)
		if errCl != nil {
			continue
		}
		raw, errRaw := client.GetRawMessage(id)
		if errRaw == nil && len(raw) > 0 {
			return client, *accCfg, id, raw, nil
		}
	}

	return nil, cfg_acc.AccountConfig{}, "", nil, fmt.Errorf("message %q not found in configured accounts", ref)
}
