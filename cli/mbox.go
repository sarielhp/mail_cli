package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"

	"github.com/sarielhp/clihelp"
)

var (
	flagDownloadRecursive bool
	flagDownloadDryRun    bool
	flagDownloadOverwrite bool
)

// DownloadCmd returns the clihelp.Command for the download command.
func downloadCmd(session *Session, name, path string) clihelp.Command {
	return clihelp.Command{
		Name:            name,
		Group:           "Folders",
		Description:     "Download messages to local mbox file(s).",
		LongDescription: "Downloads a folder into an mbox file. Safe to repeat: if the file already exists, only messages it does not yet contain are appended, and a run with nothing new leaves it untouched. Messages are matched by Message-ID (Subject and Date when there is none). The folder is also filled into the local cache. Use -d to see what would be added, or -w to replace the file instead.",
		UsageLine:       "mail_cli " + path + " <label_or_prefix> <destination> [flags]",
		Parameters: []clihelp.Param{
			{Name: "<label_or_prefix>", Description: "Label, or hierarchy prefix, to download.", Complete: CompleteLabelRefs},
			{Name: "<destination>", Description: "The local destination file (or directory if using -r)."},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagDownloadRecursive, "-r, --recursive", false, "Export sublabels into a mirrored tree"),
			clihelp.Bool(&flagDownloadDryRun, "-d", false, "Preview; create or change no files"),
			clihelp.Bool(&flagDownloadOverwrite, "-w, --overwrite", false, "Overwrite files instead of appending"),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + " receipts ~/archive/receipts", Description: "Create the mbox, or append what is missing"},
			{Line: "mail_cli " + path + " -d receipts ~/archive/receipts"},
			{Line: "mail_cli " + path + " -r research ~/archive/research/"},
			{Line: "mail_cli " + path + " -r -d FastMail:research ~/archive/research/"},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			isDryRun := app.FlagReadOnly || flagDownloadDryRun
			return runDownload(session, ctx.Args[0], ctx.Args[1], flagDownloadRecursive, isDryRun, flagDownloadOverwrite)
		},
	}
}

func runDownload(session *Session, labelArg, destPath string, recursive, dryRun, overwrite bool) error {
	client, labelName, err := session.ResolveClientAndLabel(labelArg)
	if err != nil {
		return err
	}
	if validateErr := client.Validate(); validateErr != nil {
		return validateErr
	}

	if recursive {
		return downloadRecursive(client, labelName, destPath, dryRun, overwrite, session)
	}

	uniqueLabel, err := resolveUniqueLabel(client, labelName)
	if err != nil {
		display := FormatAccountFolder(session, client, labelName)
		return fmt.Errorf("failed to resolve source label %s: %w", display, err)
	}

	mboxFile := destPath
	if fi, statErr := os.Stat(destPath); (statErr == nil && fi.IsDir()) || strings.HasSuffix(destPath, "/") {
		mboxFile = filepath.Join(destPath, cfg_g.SanitizeLabelForCache(uniqueLabel.String()))
	}

	_, _, err = downloadSingleLabel(client, uniqueLabel, mboxFile, dryRun, overwrite, session)
	return err
}

func downloadSingleLabel(client mailclient.Reader, folder label.Folder, mboxPath string, dryRun, overwrite bool, session *Session) (int, int, error) {
	labelName := folder.String()
	cacheDirName := cfg_g.SanitizeLabelForCache(labelName)
	if cacheDirName == "" {
		cacheDirName = labelName
	}

	display := FormatAccountFolder(session, client, labelName)

	existingIdx, existingCount, err := getMboxExistingMessages(mboxPath)
	if err != nil && !overwrite {
		return 0, 0, fmt.Errorf("error reading existing file %q: %w", mboxPath, err)
	}

	messageIDs, err := client.FetchAndDownloadEmails(folder, cacheDirName)
	if err != nil {
		return 0, 0, err
	}

	if len(messageIDs) == 0 {
		fmt.Fprintf(session.errOut(), "%s %s (0 messages on server)\n", app.PrefixWarn, display)
		return 0, 0, nil
	}

	downloadDir := client.Config().AccountCache
	var newEmails [][]byte

	for _, msgID := range messageIDs {
		rawBytes, readErr := msg.Read(downloadDir, msgID)
		if readErr != nil {
			continue
		}
		if !overwrite && existingCount > 0 {
			// The same authoritative-Message-ID-else-Subject+Date hierarchy
			// isEmailMatch/destFolderIndex use for splice: a header-less
			// message has no Message-ID to be found under, so it was
			// re-appended on every export until this fell back to matching
			// it by Subject and a non-zero Date too (M3).
			parsed := email.ParseReader(bytes.NewReader(rawBytes), msgID, "")
			if existingIdx.contains("", parsed) {
				continue
			}
		}
		newEmails = append(newEmails, rawBytes)
	}

	newCount := len(newEmails)
	if dryRun {
		printDryRunStatus(session.out(), display, mboxPath, newCount, existingCount, overwrite)
		return newCount, existingCount, nil
	}

	if newCount == 0 {
		fmt.Fprintf(session.errOut(), "%s %s -> %s: up to date (%d message(s) already archived)\n", app.PrefixInfo, display, mboxPath, existingCount)
		return 0, existingCount, nil
	}

	appended, err := writeMessagesToMbox(mboxPath, newEmails, overwrite)
	if err != nil {
		return 0, 0, err
	}

	printDownloadStatus(session.out(), display, mboxPath, appended, existingCount, overwrite)
	return appended, existingCount, nil
}

func printDryRunStatus(w io.Writer, display, mboxPath string, newCount, existingCount int, overwrite bool) {
	switch {
	case overwrite || existingCount == 0:
		fmt.Fprintf(w, "[DRY-RUN] %s -> %s: would download %d message(s)\n", display, mboxPath, newCount)
	case newCount == 0:
		fmt.Fprintf(w, "[DRY-RUN] %s -> %s: up to date (%d message(s) already archived)\n", display, mboxPath, existingCount)
	default:
		fmt.Fprintf(w, "[DRY-RUN] %s -> %s: would append %d new message(s) (%d already archived)\n", display, mboxPath, newCount, existingCount)
	}
}

func printDownloadStatus(w io.Writer, display, mboxPath string, appended, existingCount int, overwrite bool) {
	if overwrite || existingCount == 0 {
		fmt.Fprintf(w, "%s %s -> %s: downloaded %d message(s)\n", app.PrefixSuccess, display, mboxPath, appended)
	} else {
		fmt.Fprintf(w, "%s %s -> %s: added %d new message(s) (%d already archived)\n", app.PrefixSuccess, display, mboxPath, appended, existingCount)
	}
}

func downloadRecursive(client mailclient.Reader, prefix, destDir string, dryRun, overwrite bool, session *Session) error {
	// A listing that fails or comes back empty falls back to the prefix
	// itself, as the one folder to try.
	allLabels, err := client.GetMatchingLabels(prefix)
	if err != nil || len(allLabels) == 0 {
		allLabels = []string{prefix}
	}

	display := FormatAccountFolder(session, client, prefix)
	modeStr := ""
	if dryRun {
		modeStr = " [DRY RUN]"
	}
	fmt.Fprintf(session.errOut(), "%s Target: %s (recursive, %d folder(s)) -> %s%s\n", app.PrefixInfo, display, len(allLabels), destDir, modeStr)
	fmt.Fprintln(session.errOut(), "======================================================================")

	totalNew := 0
	totalFolders := 0
	prefixLower := strings.ToLower(prefix)

	for _, lbl := range allLabels {
		var relPath string
		lblLower := strings.ToLower(lbl)
		switch {
		case strings.EqualFold(lbl, prefix):
			relPath = filepath.Base(prefix)
		case strings.HasPrefix(lblLower, prefixLower+"/"):
			relPath = lbl[len(prefix)+1:]
		default:
			relPath = lbl
		}

		targetMbox, err := resolveSafeMboxPath(destDir, relPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s Skipping label %q: %v\n", app.PrefixError, lbl, err)
			continue
		}
		newCount, _, err := downloadSingleLabel(client, label.FromServer(lbl), targetMbox, dryRun, overwrite, session)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s Error downloading %s: %v\n", app.PrefixError, lbl, err)
			continue
		}
		if newCount > 0 {
			totalNew += newCount
			totalFolders++
		}
	}

	fmt.Fprintln(session.out(), "======================================================================")
	if dryRun {
		fmt.Fprintf(session.out(), "Finished dry run: %d new message(s) across %d folder(s) would be downloaded.\n", totalNew, totalFolders)
	} else {
		fmt.Fprintf(session.errOut(), "%s Finished: downloaded %d new message(s) across %d folder(s).\n", app.PrefixSuccess, totalNew, totalFolders)
	}
	return nil
}

func resolveSafeMboxPath(destDir, relPath string) (string, error) {
	cleanDest := filepath.Clean(destDir)
	cleanRel := filepath.Clean(filepath.ToSlash(relPath))
	if strings.HasPrefix(cleanRel, "../") || cleanRel == ".." || filepath.IsAbs(relPath) {
		return "", fmt.Errorf("path traversal attempt detected: %q", relPath)
	}
	targetMbox := filepath.Join(cleanDest, filepath.FromSlash(cleanRel))
	rel, err := filepath.Rel(cleanDest, targetMbox)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return "", fmt.Errorf("target path escapes destination directory: %q", relPath)
	}
	return targetMbox, nil
}

// getMboxExistingMessages indexes an mbox file's existing messages for
// dedup, using the same authoritative-Message-ID-else-Subject+Date matching
// as splice's destFolderIndex, rather than Message-ID alone: a message
// without one (M3) would otherwise never be recorded as "already archived"
// and would be re-appended on every export.
func getMboxExistingMessages(mboxPath string) (*destFolderIndex, int, error) {
	f, err := os.Open(mboxPath)
	if err != nil {
		if os.IsNotExist(err) {
			return newDestFolderIndex(0), 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	msgs, err := readFromMbox(f)
	if err != nil {
		return nil, 0, err
	}

	idx := newDestFolderIndex(len(msgs))
	for _, m := range msgs {
		parsed := email.ParseReader(bytes.NewReader(m.Body), "", "")
		if parsed != nil {
			idx.add("", "", parsed)
		}
	}
	return idx, len(msgs), nil
}

func writeMessagesToMbox(mboxPath string, newRawEmails [][]byte, overwrite bool) (int, error) {
	if len(newRawEmails) == 0 {
		return 0, nil
	}

	dir := filepath.Dir(mboxPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	tmpPath := mboxPath + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, fmt.Errorf("failed to create temp mbox: %w", err)
	}

	if !overwrite {
		srcFile, err := os.Open(mboxPath)
		if err != nil && !os.IsNotExist(err) {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return 0, fmt.Errorf("failed to open existing mbox file: %w", err)
		}
		if err == nil {
			_, copyErr := io.Copy(tmpFile, srcFile)
			_ = srcFile.Close()
			if copyErr != nil {
				_ = tmpFile.Close()
				_ = os.Remove(tmpPath)
				return 0, fmt.Errorf("failed to copy existing mbox data: %w", copyErr)
			}
		}
	}

	bw := bufio.NewWriterSize(tmpFile, 64*1024)
	appended := 0
	for _, raw := range newRawEmails {
		fromAddr, dateVal := extractEnvelopeFromDate(raw)
		if err := writeToMbox(bw, raw, fromAddr, dateVal); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return 0, err
		}
		appended++
	}

	if err := bw.Flush(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return 0, err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return 0, err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, err
	}

	if err := os.Rename(tmpPath, mboxPath); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("failed to atomically update %q: %w", mboxPath, err)
	}
	return appended, nil
}

func extractEnvelopeFromDate(rawBytes []byte) (string, time.Time) {
	m, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	cleanFrom := "placeholder@example.com"
	dateVal := time.Now()
	if err == nil {
		fromAddr := strings.TrimSpace(m.Header.Get("From"))
		if addr, pErr := mail.ParseAddress(fromAddr); pErr == nil && addr != nil {
			cleanFrom = addr.Address
		}
		if dateStr := m.Header.Get("Date"); dateStr != "" {
			if parsed, pErr := mail.ParseDate(dateStr); pErr == nil {
				dateVal = parsed
			}
		}
	}
	return cleanFrom, dateVal
}

// UploadCmd returns the clihelp.Command for the upload command.
func uploadCmd(session *Session, name, path string) clihelp.Command {
	return clihelp.Command{
		Name:        name,
		Group:       "Folders",
		Description: "Upload messages from mbox file to folder.",
		UsageLine:   "mail_cli " + path + " <label> <file_name>",
		Parameters: []clihelp.Param{
			{Name: "<label>", Description: "The target label/folder to upload messages into.", Complete: CompleteLabelRefs},
			{Name: "<file_name>", Description: "The local mbox file path to read messages from."},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli " + path + " %receipts ~/archive/receipts.mbox"},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			args := ctx.Args
			mboxPath := args[1]

			// A label reference, parsed like every other -- it used to be taken
			// raw, so the sigil the completion offers reached the folder
			// matcher literally and matched nothing.
			client, labelName, err := session.ResolveClientAndLabel(args[0])
			if err != nil {
				return err
			}

			uniqueLabel, err := resolveUniqueLabel(client, labelName)
			if err != nil {
				return fmt.Errorf("failed to resolve target label: %w", err)
			}

			inFile, err := os.Open(mboxPath)
			if err != nil {
				return fmt.Errorf("failed to open mbox file for reading: %w", err)
			}
			defer inFile.Close()

			messages, err := readFromMbox(inFile)
			if err != nil {
				return fmt.Errorf("failed to parse mbox file: %w", err)
			}
			if len(messages) == 0 {
				fmt.Fprintf(session.errOut(), "%s No messages found in mbox file %q.\n", app.PrefixInfo, mboxPath)
				return nil
			}

			fmt.Fprintf(session.errOut(), "%s Uploading %d messages to label %q...\n", app.PrefixInfo, len(messages), uniqueLabel)

			successCount := 0
			var lastErr error
			for idx, msg := range messages {
				if _, err := client.UploadRawEmail(msg.Body, uniqueLabel); err != nil {
					fmt.Fprintf(os.Stderr, "%s [%d] Failed to upload message, skipping: %v\n", app.PrefixError, idx+1, err)
					lastErr = err
					continue
				}
				successCount++
			}

			if successCount < len(messages) {
				return fmt.Errorf("uploaded %d/%d messages to %q; last error: %w", successCount, len(messages), uniqueLabel, lastErr)
			}

			fmt.Fprintf(session.errOut(), "%s Successfully uploaded %d/%d messages to %q.\n", app.PrefixSuccess, successCount, len(messages), uniqueLabel)
			return nil
		},
	}
}

// eachLine calls fn for every line in r, without its trailing newline.
//
// bufio.Scanner was used for this on both sides of the mbox format, capped at
// 10 MB per line. Exceeding the cap is not a truncation but an abort: Scan()
// stops, Err() reports bufio.ErrTooLong, and readFromMbox returned (nil, err) --
// so one overlong line in a mailbox discarded every message parsed before it.
// A bufio.Reader grows to the line instead. The file is one the user named, not
// input arriving from the network.
func eachLine(r io.Reader, fn func(line string) error) error {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" || err == nil {
			if cbErr := fn(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")); cbErr != nil {
				return cbErr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func writeToMbox(w io.Writer, rawEmail []byte, fromAddr string, date time.Time) error {
	if fromAddr == "" {
		fromAddr = "unknown@example.com"
	}

	if _, err := mail.ParseAddress(fromAddr); err != nil {
		fromAddr = "invalid@example.com"
	}

	dateStr := date.Format("Mon Jan _2 15:04:05 2006")
	_, err := fmt.Fprintf(w, "From %s %s\n", fromAddr, dateStr)
	if err != nil {
		return err
	}

	// Any depth of ">"-quoting is escalated by one, which is what makes the
	// write/read pair idempotent: the reader strips exactly one.
	err = eachLine(bytes.NewReader(rawEmail), func(line string) error {
		if strings.HasPrefix(strings.TrimLeft(line, ">"), "From ") {
			_, wErr := fmt.Fprintf(w, ">%s\n", line)
			return wErr
		}
		_, wErr := fmt.Fprintf(w, "%s\n", line)
		return wErr
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w)
	return err
}

type mboxMessage struct {
	From string
	Date string
	Body []byte
}

func readFromMbox(r io.Reader) ([]mboxMessage, error) {
	var msgs []mboxMessage
	var currentMsg *strings.Builder
	var currentFrom string
	var currentDate string

	readErr := eachLine(r, func(line string) error {
		if strings.HasPrefix(line, "From ") {
			if currentMsg != nil {
				if currentFrom == "" {
					currentFrom = "unknown@example.com"
				}
				msgs = append(msgs, mboxMessage{
					From: currentFrom,
					Date: currentDate,
					Body: []byte(currentMsg.String()),
				})
			}
			currentMsg = &strings.Builder{}
			parts := strings.SplitN(line, " ", 3)
			if len(parts) >= 2 {
				currentFrom = parts[1]
			}
			if len(parts) >= 3 {
				currentDate = parts[2]
			}
		} else if currentMsg != nil {
			if strings.HasPrefix(line, ">") && strings.HasPrefix(strings.TrimLeft(line, ">"), "From ") {
				line = line[1:]
			}
			currentMsg.WriteString(line + "\n")
		}
		return nil
	})
	if currentMsg != nil {
		if currentFrom == "" {
			currentFrom = "unknown@example.com"
		}
		msgs = append(msgs, mboxMessage{
			From: currentFrom,
			Date: currentDate,
			Body: []byte(currentMsg.String()),
		})
	}

	// Messages parsed before the failure are returned with it. Discarding them
	// meant one bad line lost an entire mailbox, and the caller could not even
	// say how far it had got.
	if readErr != nil {
		return msgs, fmt.Errorf("mbox read failed after %d message(s): %w", len(msgs), readErr)
	}
	return msgs, nil
}

// LabelDownloadCmd writes a folder to a local mbox file.
func LabelDownloadCmd(session *Session) clihelp.Command {
	return downloadCmd(session, "download", "label download")
}

// LabelUploadCmd reads an mbox file into a folder.
func LabelUploadCmd(session *Session) clihelp.Command {
	return uploadCmd(session, "upload", "label upload")
}
