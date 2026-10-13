package cli

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/email"

	"github.com/sarielhp/clihelp"
)

// MboxCmd returns the clihelp.Command for the mbox command tree.
func MboxCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "mbox",
		Group:       "Folders",
		Description: "Inspect, validate and merge local mbox files.",
		UsageLine:   "mail_cli mbox <subcommand> [args...]",
		Subcommands: []clihelp.Command{
			mboxCheckCmd(),
			mboxMergeCmd(),
			mboxInfoCmd(),
			mboxViewCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli mbox check archive.mbox", Description: "Validate a single mbox file"},
			{Line: "mail_cli mbox check ~/archive/", Description: "Validate all mbox files in a directory"},
			{Line: "mail_cli mbox merge new.mbox archive.mbox", Description: "Merge new.mbox into archive.mbox deduplicating messages"},
			{Line: "mail_cli mbox info archive.mbox", Description: "Show message count, file size, and attachment statistics"},
			{Line: "mail_cli mbox view archive.mbox", Description: "Open and browse an mbox file in the TUI (read-only)"},
		},
	}
}

func mboxCheckCmd() clihelp.Command {
	return clihelp.Command{
		Name:            "check",
		Description:     "Validate mbox file or directory.",
		LongDescription: "Check if a file (or all files in a directory) is a valid mbox file.",
		UsageLine:       "mail_cli mbox check <file>",
		Parameters: []clihelp.Param{
			{Name: "<file>", Description: "Mbox file or directory to validate"},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli mbox check inbox.mbox"},
			{Line: "mail_cli mbox check ~/archive/"},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runMboxCheck(ctx.Args[0])
		},
	}
}

func mboxMergeCmd() clihelp.Command {
	return clihelp.Command{
		Name:            "merge",
		Description:     "Merge mbox file x into y.",
		LongDescription: "Merge mbox file x into y (copies messages in x but not in y, appending to y).",
		UsageLine:       "mail_cli mbox merge <x> <y>",
		Parameters: []clihelp.Param{
			{Name: "<x>", Description: "Source mbox file to merge from"},
			{Name: "<y>", Description: "Destination mbox file to merge into"},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli mbox merge incoming.mbox archive.mbox"},
		},
		Args: clihelp.ExactArgs(2),
		Run: func(ctx *clihelp.Context) error {
			return runMboxMerge(ctx.Args[0], ctx.Args[1])
		},
	}
}

func mboxInfoCmd() clihelp.Command {
	return clihelp.Command{
		Name:            "info",
		Description:     "Display message count and file size.",
		LongDescription: "Display message count, total file size, and attachment sizes for an mbox file.",
		UsageLine:       "mail_cli mbox info <file>",
		Parameters: []clihelp.Param{
			{Name: "<file>", Description: "Mbox file to inspect"},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli mbox info archive.mbox"},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runMboxInfo(ctx.Args[0])
		},
	}
}

func runMboxCheck(path string) error {
	return runMboxCheckTo(os.Stdout, path)
}

func runMboxCheckTo(w io.Writer, path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access %q: %w", path, err)
	}
	if fi.IsDir() {
		return checkDirectoryMboxes(w, path)
	}
	return checkSingleMboxFile(w, path)
}

func checkSingleMboxFile(w io.Writer, path string) error {
	count, err := ValidateMboxFile(path)
	if err != nil {
		fmt.Fprintf(w, "%s %s — %v\n", app.ColorBoldRed.Sprint("[INVALID]"), path, err)
		return fmt.Errorf("validation failed for %s: %w", path, err)
	}
	fmt.Fprintf(w, "%s   %s (%d message(s))\n", app.ColorBoldGreen.Sprint("[VALID]"), path, count)
	return nil
}

func checkDirectoryMboxes(w io.Writer, dir string) error {
	validCount := 0
	invalidCount := 0

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			invalidCount++
			fmt.Fprintf(w, "%s %s — %v\n", app.ColorBoldRed.Sprint("[INVALID]"), path, err)
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		count, valErr := ValidateMboxFile(path)
		if valErr != nil {
			invalidCount++
			fmt.Fprintf(w, "%s %s — %v\n", app.ColorBoldRed.Sprint("[INVALID]"), path, valErr)
		} else {
			validCount++
			fmt.Fprintf(w, "%s   %s (%d message(s))\n", app.ColorBoldGreen.Sprint("[VALID]"), path, count)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if invalidCount > 0 {
		return fmt.Errorf("directory check completed with %d invalid file(s) (%d valid)", invalidCount, validCount)
	}
	return nil
}

// ValidateMboxFile verifies that a file is a valid RFC 4155 mbox with clean RFC 5322 messages.
func ValidateMboxFile(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if fi.IsDir() {
		return 0, fmt.Errorf("%q is a directory", path)
	}
	if fi.Size() == 0 {
		return 0, fmt.Errorf("file is empty, missing RFC 4155 'From ' delimiter")
	}

	reader := bufio.NewReader(f)
	return validateMboxStream(reader)
}

func validateMboxStream(r *bufio.Reader) (int, error) {
	firstLine, err := r.ReadString('\n')
	if err != nil && len(firstLine) == 0 {
		return 0, fmt.Errorf("failed to read first line: %w", err)
	}
	if !strings.HasPrefix(firstLine, "From ") {
		return 0, fmt.Errorf("does not start with RFC 4155 'From ' delimiter")
	}

	currentEnvelope := firstLine
	var currentBody bytes.Buffer
	msgCount := 0

	for {
		line, readErr := r.ReadString('\n')
		if len(line) > 0 {
			if strings.HasPrefix(line, "From ") {
				msgCount++
				if err := validateRFC5322Message(currentEnvelope, currentBody.Bytes(), msgCount); err != nil {
					return 0, err
				}
				currentEnvelope = line
				currentBody.Reset()
			} else {
				currentBody.WriteString(line)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, fmt.Errorf("read error: %w", readErr)
		}
	}

	msgCount++
	if err := validateRFC5322Message(currentEnvelope, currentBody.Bytes(), msgCount); err != nil {
		return 0, err
	}
	return msgCount, nil
}

func validateRFC5322Message(envelope string, rawMsg []byte, idx int) error {
	parts := strings.SplitN(strings.TrimSpace(envelope), " ", 3)
	if len(parts) < 2 {
		return fmt.Errorf("message %d: malformed RFC 4155 From delimiter: %q", idx, strings.TrimSpace(envelope))
	}

	if len(bytes.TrimSpace(rawMsg)) == 0 {
		return fmt.Errorf("message %d: missing message headers and body", idx)
	}

	m, err := mail.ReadMessage(bytes.NewReader(rawMsg))
	if err != nil {
		return fmt.Errorf("message %d: failed to parse RFC 5322 message: %w", idx, err)
	}
	if len(m.Header) == 0 {
		return fmt.Errorf("message %d: missing RFC 5322 headers", idx)
	}

	if from := m.Header.Get("From"); from != "" {
		if _, err := mail.ParseAddressList(from); err != nil {
			return fmt.Errorf("message %d: invalid RFC 5322 From header: %w", idx, err)
		}
	}
	if date := m.Header.Get("Date"); date != "" {
		if _, err := mail.ParseDate(date); err != nil {
			return fmt.Errorf("message %d: invalid RFC 5322 Date header: %w", idx, err)
		}
	}
	return nil
}

type mboxEntry struct {
	envelope string
	rawMsg   []byte
	email    *email.Email
}

func runMboxMerge(srcPath, destPath string) error {
	return runMboxMergeTo(os.Stdout, srcPath, destPath)
}

func runMboxMergeTo(w io.Writer, srcPath, destPath string) error {
	if filepath.Clean(srcPath) == filepath.Clean(destPath) {
		return fmt.Errorf("cannot merge file into itself")
	}

	if _, err := ValidateMboxFile(srcPath); err != nil {
		return fmt.Errorf("pre-validation failed for source mbox %q: %w", srcPath, err)
	}
	if _, err := ValidateMboxFile(destPath); err != nil {
		return fmt.Errorf("pre-validation failed for destination mbox %q: %w", destPath, err)
	}

	srcMsgs, err := readMboxEntries(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source mbox: %w", err)
	}
	destMsgs, err := readMboxEntries(destPath)
	if err != nil {
		return fmt.Errorf("failed to read destination mbox: %w", err)
	}

	newMsgs := filterNewMessages(srcMsgs, destMsgs)
	if len(newMsgs) == 0 {
		fmt.Fprintf(w, "%s No new messages to merge (%s already contains all messages from %s)\n", app.PrefixInfo, destPath, srcPath)
		return nil
	}

	return executeMerge(w, destPath, destMsgs, newMsgs)
}

func readMboxEntries(path string) ([]mboxEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	return parseMboxStream(reader)
}

func parseMboxStream(r *bufio.Reader) ([]mboxEntry, error) {
	firstLine, err := r.ReadString('\n')
	if err != nil && len(firstLine) == 0 {
		return nil, fmt.Errorf("failed to read mbox: %w", err)
	}
	if !strings.HasPrefix(firstLine, "From ") {
		return nil, fmt.Errorf("not an mbox file")
	}

	var entries []mboxEntry
	currentEnv := firstLine
	var currentBody bytes.Buffer

	for {
		line, readErr := r.ReadString('\n')
		if len(line) > 0 {
			if strings.HasPrefix(line, "From ") {
				entries = append(entries, makeMboxEntry(currentEnv, bytes.Clone(currentBody.Bytes())))
				currentEnv = line
				currentBody.Reset()
			} else {
				currentBody.WriteString(line)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}

	entries = append(entries, makeMboxEntry(currentEnv, bytes.Clone(currentBody.Bytes())))
	return entries, nil
}

func makeMboxEntry(env string, raw []byte) mboxEntry {
	parsed := email.ParseReader(bytes.NewReader(raw), "", "")
	return mboxEntry{
		envelope: env,
		rawMsg:   raw,
		email:    parsed,
	}
}

// isMboxEmailMatch is the mbox spelling of the shared email-identity check: it
// matches by Message-ID, or by Subject and Date when neither side carries one.
func isMboxEmailMatch(src, dest *email.Email) bool {
	return isEmailMatch(src, dest)
}

func filterNewMessages(srcMsgs, destMsgs []mboxEntry) []mboxEntry {
	idx := newMessageIdentityIndex(len(destMsgs) + len(srcMsgs))
	for _, dm := range destMsgs {
		idx.add(dm.email)
	}

	var newMsgs []mboxEntry
	for _, sm := range srcMsgs {
		if sm.email == nil {
			continue
		}
		if !idx.contains(sm.email) {
			newMsgs = append(newMsgs, sm)
			idx.add(sm.email)
		}
	}
	return newMsgs
}

func executeMerge(w io.Writer, destPath string, destMsgs, newMsgs []mboxEntry) error {
	tmpPath := destPath + ".tmp"
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := writeMergedTempFile(tmpPath, destPath, newMsgs); err != nil {
		return fmt.Errorf("failed to write merged file: %w", err)
	}

	if err := postValidateMerge(tmpPath, destPath, len(destMsgs), newMsgs); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to atomically replace destination file: %w", err)
	}
	success = true

	total := len(destMsgs) + len(newMsgs)
	fmt.Fprintf(w, "%s Successfully merged %d message(s) into %s (total: %d messages)\n", app.PrefixSuccess, len(newMsgs), destPath, total)
	return nil
}

func writeMergedTempFile(tmpPath, destPath string, newMsgs []mboxEntry) error {
	destBytes, err := os.ReadFile(destPath)
	if err != nil {
		return err
	}

	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(destBytes); err != nil {
		return err
	}
	if len(destBytes) > 0 && destBytes[len(destBytes)-1] != '\n' {
		if _, err := tmpFile.Write([]byte("\n")); err != nil {
			return err
		}
	}

	for _, msg := range newMsgs {
		if _, err := tmpFile.WriteString(msg.envelope); err != nil {
			return err
		}
		if _, err := tmpFile.Write(msg.rawMsg); err != nil {
			return err
		}
		if len(msg.rawMsg) > 0 && msg.rawMsg[len(msg.rawMsg)-1] != '\n' {
			if _, err := tmpFile.Write([]byte("\n")); err != nil {
				return err
			}
		}
	}

	return tmpFile.Sync()
}

func postValidateMerge(tmpPath, destPath string, origDestCount int, newMsgs []mboxEntry) error {
	count, err := ValidateMboxFile(tmpPath)
	if err != nil {
		return fmt.Errorf("post-validation failed: merged file is not valid mbox: %w", err)
	}

	expectedCount := origDestCount + len(newMsgs)
	if count != expectedCount {
		return fmt.Errorf("post-validation failed: expected %d messages, got %d", expectedCount, count)
	}

	destFi, err := os.Stat(destPath)
	if err != nil {
		return err
	}
	tmpFi, err := os.Stat(tmpPath)
	if err != nil {
		return err
	}

	var addedBytes int64
	for _, m := range newMsgs {
		addedBytes += int64(len(m.envelope)) + int64(len(m.rawMsg))
	}
	expectedSize := destFi.Size() + addedBytes

	if tmpFi.Size() < destFi.Size() {
		return fmt.Errorf("post-validation failed: merged file size (%d) smaller than original (%d)", tmpFi.Size(), destFi.Size())
	}
	if tmpFi.Size() < expectedSize-100 || tmpFi.Size() > expectedSize+1024 {
		return fmt.Errorf("post-validation failed: file size %d deviates from expected ~%d", tmpFi.Size(), expectedSize)
	}
	return nil
}

func runMboxInfo(path string) error {
	return runMboxInfoTo(os.Stdout, path)
}

func runMboxInfoTo(w io.Writer, path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access %q: %w", path, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("%q is a directory, expected an mbox file", path)
	}

	if _, validateErr := ValidateMboxFile(path); validateErr != nil {
		return fmt.Errorf("%q is not a valid mbox file: %w", path, validateErr)
	}

	msgs, err := readMboxEntries(path)
	if err != nil {
		return fmt.Errorf("failed to read mbox: %w", err)
	}

	totalAttParts := 0
	var totalAttBytes int64
	for _, m := range msgs {
		pCount, pBytes := inspectMessageAttachments(m.rawMsg)
		totalAttParts += pCount
		totalAttBytes += pBytes
	}

	fileSizeStr := fmt.Sprintf("%s bytes (%s)", FormatWithCommas(fi.Size()), FormatHumanSize(fi.Size()))
	attSizeStr := fmt.Sprintf("%s bytes (%s)", FormatWithCommas(totalAttBytes), FormatHumanSize(totalAttBytes))

	fmt.Fprintf(w, "File:             %s\n", path)
	fmt.Fprintf(w, "Messages:         %s\n", FormatWithCommas(int64(len(msgs))))
	fmt.Fprintf(w, "Total File Size:  %s\n", fileSizeStr)
	fmt.Fprintf(w, "Attachment Count: %s\n", FormatWithCommas(int64(totalAttParts)))
	fmt.Fprintf(w, "Attachment Size:  %s\n", attSizeStr)
	return nil
}

func inspectMessageAttachments(raw []byte) (int, int64) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return 0, 0
	}
	return countAttachmentsInMIME(msg.Header, msg.Body)
}

func countAttachmentsInMIME(header mail.Header, body io.Reader) (int, int64) {
	ct := header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		mediaType = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
		params = make(map[string]string)
	}

	cd := header.Get("Content-Disposition")
	dispType, dispParams, _ := mime.ParseMediaType(cd)

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return 0, 0
		}
		mr := multipart.NewReader(body, boundary)
		count := 0
		var totalBytes int64
		for {
			part, err := mr.NextPart()
			if err == io.EOF || err != nil {
				break
			}
			pCount, pBytes := countAttachmentsInMIME(mail.Header(part.Header), part)
			count += pCount
			totalBytes += pBytes
		}
		return count, totalBytes
	}

	if isAttachmentMIMEPart(mediaType, params, dispType, dispParams) {
		data, err := decodeMIMEPartData(header, body)
		if err == nil {
			return 1, int64(len(data))
		}
	}
	return 0, 0
}

func isAttachmentMIMEPart(mediaType string, params map[string]string, dispType string, dispParams map[string]string) bool {
	if strings.HasPrefix(mediaType, "multipart/") {
		return false
	}
	if dispType == "attachment" {
		return true
	}
	if dispParams["filename"] != "" || params["name"] != "" {
		return true
	}
	if mediaType == "text/calendar" || mediaType == "application/ics" || mediaType == "text/x-vcalendar" {
		return true
	}
	if mediaType != "" && !strings.HasPrefix(mediaType, "text/") && mediaType != "message/rfc822" && dispType != "inline" {
		return true
	}
	return false
}

func decodeMIMEPartData(header mail.Header, body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	cte := strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding")))
	switch cte {
	case "base64":
		dec := base64.NewDecoder(base64.StdEncoding, bytes.NewReader(data))
		if decoded, err := io.ReadAll(dec); err == nil && len(decoded) > 0 {
			return decoded, nil
		}
		clean := bytes.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, data)
		dec = base64.NewDecoder(base64.RawStdEncoding, bytes.NewReader(clean))
		if decoded, err := io.ReadAll(dec); err == nil && len(decoded) > 0 {
			return decoded, nil
		}
	case "quoted-printable":
		dec := quotedprintable.NewReader(bytes.NewReader(data))
		if decoded, err := io.ReadAll(dec); err == nil && len(decoded) > 0 {
			return decoded, nil
		}
	}
	return data, nil
}

// FormatWithCommas formats an integer with thousands separator commas.
func FormatWithCommas(n int64) string {
	in := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(in, "-") {
		sign = "-"
		in = in[1:]
	}
	var b strings.Builder
	b.WriteString(sign)
	rem := len(in) % 3
	if rem > 0 {
		b.WriteString(in[:rem])
		if len(in) > rem {
			b.WriteString(",")
		}
	}
	for i := rem; i < len(in); i += 3 {
		b.WriteString(in[i : i+3])
		if i+3 < len(in) {
			b.WriteString(",")
		}
	}
	return b.String()
}

// FormatHumanSize formats bytes into human-readable units (B, KB, MB, GB, TB).
func FormatHumanSize(b int64) string {
	if b < 0 {
		return "-" + FormatHumanSize(-b)
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
		tb = 1024 * gb
	)
	switch {
	case b >= tb:
		return fmt.Sprintf("%.2f TB", float64(b)/float64(tb))
	case b >= gb:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
