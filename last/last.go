package last

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"mime"
	"sort"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cache/virtual"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/ui"
)

// Options is what a last command was asked to do.
type Options struct {
	JSON bool
}

// EmailWithFolder pairs an email with the folder it belongs to.
type EmailWithFolder struct {
	Email  email.Email
	Folder string
}

// Perform fetches emails across the account, collects the last N received,
// stores them into a virtual mailbox named "last", and prints them showing each email's folder.
func Perform(opts Options, out io.Writer, rep report.Reporter, client mailclient.Reader, config *cfg_g.Config, n int) error {
	if n <= 0 {
		return fmt.Errorf("number of emails must be greater than 0")
	}

	rep.Infof("Fetching last %d emails from server...\n", n)

	// 1. Try server-side account-wide fetch first
	var refs []cfg_acc.MessageFolderRef
	var err error
	if client != nil {
		refs, err = client.FetchLatestAccountEmails(n)
	}

	// 2. Fall back to folder-by-folder scan if account-wide query is not supported or returned nothing
	if err != nil || len(refs) == 0 {
		return performFolderByFolderFallback(opts, out, rep, client, config, n)
	}

	// 3. Build email list from message refs
	var allEmails []EmailWithFolder
	dec := new(mime.WordDecoder)
	for _, ref := range refs {
		eml := parseMessageData(config.AccountCache, ref.MessageID, dec)
		allEmails = append(allEmails, EmailWithFolder{
			Folder: ref.Folder,
			Email:  eml,
		})
	}

	return finishAndDisplay(opts, out, rep, allEmails, config, n)
}

func parseMessageData(downloadDir cachedir.Account, id email.ID, _ *mime.WordDecoder) email.Email {
	em, err := msg.CachedEmail(downloadDir, id)
	if err != nil {
		return ui.PlaceholderEmail(id)
	}
	em.Subject = email.ReverseHebrewRuns(em.Subject)
	return *em
}

func performFolderByFolderFallback(opts Options, out io.Writer, rep report.Reporter, client mailclient.Reader, config *cfg_g.Config, n int) error {
	folders, err := client.GetLabelItems()
	if err != nil {
		return fmt.Errorf("failed to get folders: %w", err)
	}

	if len(folders) == 0 {
		if opts.JSON {
			fmt.Fprintln(out, "[]")
			return nil
		}
		rep.Infof("No folders found in account.\n")
		return nil
	}

	var allEmails []EmailWithFolder
	seenMsgIDs := make(map[email.ID]bool)
	dec := new(mime.WordDecoder)

	for _, f := range folders {
		folderName := f.FullName
		if folderName == "" {
			folderName = f.Name
		}
		if folderName == "" {
			continue
		}

		cacheDirName := cfg_g.SanitizeLabelForCache(folderName)
		downloadedIDs, fetchErr := client.FetchAndDownloadEmails(label.FromServer(folderName), cacheDirName)
		if fetchErr != nil {
			if config.Verbose {
				rep.Warnf("Warning: failed to fetch emails from folder %q: %v\n", folderName, fetchErr)
			}
			continue
		}

		for _, id := range downloadedIDs {
			if seenMsgIDs[id] {
				continue
			}
			seenMsgIDs[id] = true

			eml := parseMessageData(config.AccountCache, id, dec)
			allEmails = append(allEmails, EmailWithFolder{
				Folder: folderName,
				Email:  eml,
			})
		}
	}

	return finishAndDisplay(opts, out, rep, allEmails, config, n)
}

func finishAndDisplay(opts Options, out io.Writer, rep report.Reporter, allEmails []EmailWithFolder, config *cfg_g.Config, n int) error {
	if len(allEmails) == 0 {
		if opts.JSON {
			fmt.Fprintln(out, "[]")
			return nil
		}
		rep.Infof("No emails found.\n")
		return nil
	}

	// 1. Sort all emails by date descending (most recently received first)
	sort.Slice(allEmails, func(i, j int) bool {
		return allEmails[i].Email.EmailDate.After(allEmails[j].Email.EmailDate)
	})

	// 2. Keep only the top N
	if len(allEmails) > n {
		allEmails = allEmails[:n]
	}

	// 3. Reverse to chronological order (oldest to newest among the last N)
	sort.Slice(allEmails, func(i, j int) bool {
		return allEmails[i].Email.EmailDate.Before(allEmails[j].Email.EmailDate)
	})

	// 4. Save to Mailbox "last"
	vm := &virtual.Mailbox{
		Name:        "last",
		Description: fmt.Sprintf("Last %d emails received across all folders", len(allEmails)),
		MessageIDs:  email.IDs(make([]string, len(allEmails))),
		FolderMap:   make(map[email.ID]string),
	}
	for i, ef := range allEmails {
		vm.MessageIDs[i] = ef.Email.ID
		vm.FolderMap[ef.Email.ID] = ef.Folder
	}
	_ = virtual.Save(config.AccountCache, vm)

	// 5. Print results with folder names
	var emailsOnly []email.Email
	folderMap := make(map[email.ID]string)
	for _, ef := range allEmails {
		emailsOnly = append(emailsOnly, ef.Email)
		folderMap[ef.Email.ID] = ef.Folder
	}

	if opts.JSON {
		summaries := email.ToSummariesJSON(emailsOnly, folderMap)
		data, err := json.MarshalIndent(summaries, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding last emails as JSON: %w", err)
		}
		fmt.Fprintf(out, "%s\n", data)
		return nil
	}

	ui.FormatEmailsWithFolders(out,
		fmt.Sprintf("LAST %d EMAILS (ACROSS ALL FOLDERS)", len(allEmails)),
		emailsOnly,
		folderMap,
		config,
	)

	return nil
}
