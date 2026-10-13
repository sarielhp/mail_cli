package runner

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/cli"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

type exportItem struct {
	ID      email.ID
	Subject string
	From    string
	Date    time.Time
	Raw     []byte
}

// RunExport exports an email or conversation thread into mbox or eml format.
func RunExport(out io.Writer, config *cfg_g.Config, opts cli.ExportOptions, rep report.Reporter) error {
	client, acc, targetID, rawBytes, err := findMessageAcrossAccounts(config, opts.Account, opts.MessageID, rep)
	if err != nil {
		return err
	}

	var items []exportItem
	if opts.Single {
		em := email.ParseReader(bytes.NewReader(rawBytes), targetID, "")
		subj, from, dt := "", "", time.Time{}
		if em != nil {
			subj, from, dt = em.Subject, em.FromRaw, em.EmailDate
		}
		items = []exportItem{{ID: targetID, Subject: subj, From: from, Date: dt, Raw: rawBytes}}
	} else {
		items, err = collectThreadItems(config, acc, client, targetID, rawBytes)
		if err != nil {
			return err
		}
	}

	if opts.DryRun {
		return previewExportDryRun(out, items, opts.Format)
	}

	if opts.Format == "eml" {
		return writeEML(out, items, opts.Output, rep)
	}
	return writeMBox(out, items, opts.Output, rep)
}

func previewExportDryRun(out io.Writer, items []exportItem, format string) error {
	fmt.Fprintf(out, "[DRY-RUN] Would export %d message(s) in %s format:\n", len(items), format)
	for i, item := range items {
		dateStr := item.Date.Format(time.RFC3339)
		if item.Date.IsZero() {
			dateStr = "unknown-date"
		}
		fmt.Fprintf(out, "  [%d/%d] %s | From: %s | Subject: %s (%d bytes)\n",
			i+1, len(items), dateStr, item.From, item.Subject, len(item.Raw))
	}
	return nil
}

func collectThreadItems(config *cfg_g.Config, acc cfg_acc.AccountConfig, client mailclient.MailClient, targetID email.ID, rawBytes []byte) ([]exportItem, error) {
	targetEmail := email.ParseReader(bytes.NewReader(rawBytes), targetID, "")
	if targetEmail == nil {
		return []exportItem{{ID: targetID, Raw: rawBytes}}, nil
	}

	dlDir := accountCacheDir(config, acc)
	if dlDir.IsZero() {
		return []exportItem{{ID: targetID, Subject: targetEmail.Subject, From: targetEmail.FromRaw, Date: targetEmail.EmailDate, Raw: rawBytes}}, nil
	}

	pool := loadAccountEmailPool(dlDir, targetEmail)
	roots := email.BuildThreadTree(pool)
	threadEmails := extractThreadEmails(roots, targetEmail)
	if len(threadEmails) == 0 {
		threadEmails = []*email.Email{targetEmail}
	}

	sort.Slice(threadEmails, func(i, j int) bool {
		return threadEmails[i].EmailDate.Before(threadEmails[j].EmailDate)
	})

	return resolveExportItems(dlDir, client, targetID, rawBytes, threadEmails), nil
}

func loadAccountEmailPool(dlDir cachedir.Account, targetEmail *email.Email) []email.Email {
	var pool []email.Email
	seenTarget := false
	msg.ForEachID(dlDir, func(id email.ID) bool {
		em, err := msg.CachedEmail(dlDir, id)
		if err == nil && em != nil {
			if em.ID == targetEmail.ID {
				seenTarget = true
			}
			pool = append(pool, *em)
		}
		return false
	})
	if !seenTarget {
		pool = append(pool, *targetEmail)
	}
	return pool
}

func extractThreadEmails(roots []*email.ThreadNode, target *email.Email) []*email.Email {
	for _, root := range roots {
		if nodeMatchesTarget(root, target) {
			return collectSubtreeEmails(root)
		}
	}
	return nil
}

func nodeMatchesTarget(node *email.ThreadNode, target *email.Email) bool {
	if node.Email.ID == target.ID || (target.MessageID != "" && node.Email.MessageID == target.MessageID) {
		return true
	}
	for _, child := range node.Children {
		if nodeMatchesTarget(child, target) {
			return true
		}
	}
	return false
}

func collectSubtreeEmails(node *email.ThreadNode) []*email.Email {
	emails := []*email.Email{node.Email}
	for _, child := range node.Children {
		emails = append(emails, collectSubtreeEmails(child)...)
	}
	return emails
}

func resolveExportItems(dlDir cachedir.Account, client mailclient.MailClient, targetID email.ID, targetRaw []byte, threadEmails []*email.Email) []exportItem {
	var items []exportItem
	for _, em := range threadEmails {
		raw := targetRaw
		if em.ID != targetID {
			data, err := msg.Read(dlDir, em.ID)
			if err == nil && len(data) > 0 {
				raw = data
			} else if client != nil {
				remoteData, rErr := client.GetRawMessage(em.ID)
				if rErr == nil && len(remoteData) > 0 {
					raw = remoteData
				}
			}
		}
		if len(raw) > 0 {
			items = append(items, exportItem{
				ID:      em.ID,
				Subject: em.Subject,
				From:    em.FromRaw,
				Date:    em.EmailDate,
				Raw:     raw,
			})
		}
	}
	return items
}

func writeMBox(out io.Writer, items []exportItem, outputPath string, rep report.Reporter) error {
	dest := out
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("creating mbox file %q: %w", outputPath, err)
		}
		defer f.Close()
		dest = f
	}

	for _, item := range items {
		sender := extractSenderEmail(item.From)
		dateStr := item.Date.Format(time.ANSIC)
		if item.Date.IsZero() {
			dateStr = time.Now().UTC().Format(time.ANSIC)
		}
		if _, err := fmt.Fprintf(dest, "From %s %s\n", sender, dateStr); err != nil {
			return err
		}
		if err := writeMboxRDEscaped(dest, item.Raw); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(dest); err != nil {
			return err
		}
	}

	if outputPath != "" {
		rep.Successf("Exported %d message(s) to %s", len(items), outputPath)
	}
	return nil
}

func writeMboxRDEscaped(w io.Writer, raw []byte) error {
	lines := bytes.Split(raw, []byte("\n"))
	for i, line := range lines {
		trimmed := bytes.TrimRight(line, "\r")
		if isFromLine(trimmed) {
			if _, err := w.Write([]byte(">")); err != nil {
				return err
			}
		}
		if _, err := w.Write(trimmed); err != nil {
			return err
		}
		if i < len(lines)-1 || len(line) > 0 {
			if _, err := w.Write([]byte("\n")); err != nil {
				return err
			}
		}
	}
	return nil
}

func isFromLine(line []byte) bool {
	l := line
	for len(l) > 0 && l[0] == '>' {
		l = l[1:]
	}
	return bytes.HasPrefix(l, []byte("From "))
}

func extractSenderEmail(fromRaw string) string {
	if addr, err := mail.ParseAddress(fromRaw); err == nil && addr != nil && addr.Address != "" {
		return addr.Address
	}
	trimmed := strings.TrimSpace(fromRaw)
	if trimmed != "" {
		return trimmed
	}
	return "MAILER-DAEMON"
}

func writeEML(out io.Writer, items []exportItem, outputDir string, rep report.Reporter) error {
	if outputDir == "" {
		if len(items) == 1 {
			_, err := out.Write(items[0].Raw)
			return err
		}
		return fmt.Errorf("exporting multiple messages in eml format requires -o/--output <directory>")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory %q: %w", outputDir, err)
	}

	for i, item := range items {
		// The running number already keeps names apart, so the subject is
		// simply cut to length.
		safeSubj := email.ShortStem(email.SubjectFileStem(item.Subject), email.FileStemLimit, nil)
		filename := fmt.Sprintf("%03d_%s.eml", i+1, safeSubj)
		filePath := filepath.Join(outputDir, filename)
		if err := os.WriteFile(filePath, item.Raw, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", filePath, err)
		}
	}

	rep.Successf("Exported %d message(s) to %s/", len(items), outputDir)
	return nil
}
