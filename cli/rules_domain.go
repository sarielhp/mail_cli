package cli

import (
	"bytes"
	"fmt"
	"net/mail"
	"strings"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"

	"github.com/sarielhp/clihelp"
)

func ruleAddDomainCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "add-domain",
		Title:           "rule add-domain <message_id> [lbl]",
		Description:     "Add rule for a sender's domain.",
		LongDescription: "Add an auto-labeling rule for a sender's domain from a cached message ID.",
		UsageLine:       "mail_cli rule add-domain <message_id> [lbl]",
		Parameters: []clihelp.Param{
			{Name: "<message_id>", Description: "The message ID or short ID of the email."},
			{Name: "[lbl]", Description: "Target label (default: message's folder).", Complete: CompleteLabelRefs},
		},
		Examples: []clihelp.Example{
			{Line: `mail_cli rule add-domain 12345 %Sort/Newsletters`},
			{Line: "mail_cli rule add-domain 12345"},
		},
		Args: clihelp.RangeArgs(1, 2),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			messageID := args[0]
			label, err := ruleLabelArg(session, args, 1)
			if err != nil {
				return err
			}
			return addDomainRuleToConfig(session, messageID, label)
		},
	}
}

func addDomainRuleToConfig(session *Session, messageID, label string) error {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}

	foundID, folderName, downloadDir, err := resolveCachedEmail(session, resolved.Account, messageID)
	if err != nil {
		return fmt.Errorf("failed to find message %s: %w", messageID, err)
	}

	rawMsg, err := msg.Read(downloadDir, foundID)
	if err != nil {
		return fmt.Errorf("failed to read cached message %s: %w", messageID, err)
	}

	parsedMsg, err := mail.ReadMessage(bytes.NewReader(rawMsg))
	if err != nil {
		return fmt.Errorf("failed to parse cached message %s: %w", messageID, err)
	}

	fromRaw := strings.TrimSpace(parsedMsg.Header.Get("From"))
	fromEmail := email.ParseEmailAddress(fromRaw)
	if fromEmail == "" {
		return fmt.Errorf("could not extract sender email address from message %s", messageID)
	}

	parts := strings.Split(fromEmail, "@")
	if len(parts) < 2 || parts[len(parts)-1] == "" {
		return fmt.Errorf("invalid sender email address %q in message %s", fromEmail, messageID)
	}

	domain := strings.ToLower(parts[len(parts)-1])
	domainPattern := "@" + domain

	if label == "" {
		switch {
		case folderName != "" && !strings.EqualFold(folderName, "inbox"):
			label = folderName
		case resolved.Account.SpamLearn != "":
			label = resolved.Account.SpamLearn
		default:
			return fmt.Errorf("no label specified and message is in inbox; please provide a target label")
		}
	}

	return addRuleToConfig(session, domainPattern, label, "")
}

func addRuleFromMessageID(session *Session, messageID, label string) error {
	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		return err
	}

	foundID, folderName, downloadDir, err := resolveCachedEmail(session, resolved.Account, messageID)
	if err != nil {
		if !strings.Contains(messageID, "@") {
			return fmt.Errorf("message %s not found locally and %q is not a valid email address", messageID, messageID)
		}
		return fmt.Errorf("failed to find message %s: %w", messageID, err)
	}

	rawMsg, err := msg.Read(downloadDir, foundID)
	if err != nil {
		return fmt.Errorf("failed to read cached message %s: %w", messageID, err)
	}

	parsedMsg, err := mail.ReadMessage(bytes.NewReader(rawMsg))
	if err != nil {
		return fmt.Errorf("failed to parse cached message %s: %w", messageID, err)
	}

	fromRaw := strings.TrimSpace(parsedMsg.Header.Get("From"))
	fromEmail := email.ParseEmailAddress(fromRaw)
	if fromEmail == "" || !email.IsValidEmail(fromEmail) {
		return fmt.Errorf("could not extract valid sender email address from message %s (found %q)", messageID, fromRaw)
	}

	if label == "" {
		switch {
		case folderName != "" && !strings.EqualFold(folderName, "inbox"):
			label = folderName
		case resolved.Account.SpamLearn != "":
			label = resolved.Account.SpamLearn
		default:
			return fmt.Errorf("no label specified and message is in inbox; please provide a target label")
		}
	}

	return addRuleToConfig(session, fromEmail, label, "")
}

// resolveCachedEmail finds a cached message in an account's cache: the
// selected client's first, then the target account's, then every other
// configured account's. It used to also search the cache root and every
// directory under it -- drafts and mbox views included -- because the root
// and an account's directory were one field and it could not tell which it
// had.
func resolveCachedEmail(session *Session, targetAcc *cfg_acc.AccountConfig, messageID string) (foundID email.ID, folderName string, dir cachedir.Account, err error) {
	var candidates []cachedir.Account
	if client, cErr := session.clients().ForSelected(session.Config); cErr == nil && client != nil {
		if cfg := client.Config(); cfg != nil {
			candidates = append(candidates, cfg.AccountCache)
		}
	}
	if targetAcc != nil {
		candidates = append(candidates, session.Config.AccountCacheFor(*targetAcc))
	}
	for _, acc := range session.Config.Accounts {
		candidates = append(candidates, session.Config.AccountCacheFor(acc))
	}

	seen := make(map[cachedir.Account]bool)
	for _, c := range candidates {
		if c.IsZero() || seen[c] {
			continue
		}
		seen[c] = true
		if fid, fname, fErr := cache.FindCachedEmailByID(c, messageID); fErr == nil {
			return fid, fname, c, nil
		}
	}
	return "", "", cachedir.Account{}, fmt.Errorf("no cached email found with ID %q", messageID)
}
