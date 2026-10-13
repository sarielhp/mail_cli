package show

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/browser"
	"github.com/sarielhp/mail_cli/cache/msgstore"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/pgp"
	"github.com/sarielhp/mail_cli/ui"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
)

// Client is what showing a message needs: labels to resolve a prefix, and
// message access to retrieve one by ID whether or not it is cached.
type Client interface {
	mailclient.LabelReader
	mailclient.EmailFetcher
	mailclient.MessageAccessor
	mailclient.ConfigProvider
}

// FolderListing is one matched folder in `show --json` output.
type FolderListing struct {
	Account    string   `json:"account,omitempty"`
	Folder     string   `json:"folder"`
	MessageIDs []string `json:"message_ids"`
	Total      int      `json:"total"`
}

// MarshalListings writes every folder listing as one JSON array. show used to
// print one object per matched folder, which concatenated into invalid JSON
// whenever a prefix matched more than one folder or -A named more than one
// account; the array is the one document, as scan --json does it.
func MarshalListings(w io.Writer, listings []FolderListing) error {
	if listings == nil {
		listings = []FolderListing{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(listings)
}

// Perform lists the messages of every label matching labelPrefix, or renders the
// single message named by targetMsgID; it errors when no label matches, and,
// when a target was asked for, when no matching label holds it.
//
// Under JSON a listing is not printed but returned, so the caller can write one
// document across every account; a target message is rendered as one object,
// and no folder listing precedes it.
func Perform(opts Options, w io.Writer, client Client, config *cfg_g.Config, labelPrefix string, targetMsgID string) ([]FolderListing, error) {
	matchedLabels, err := client.GetMatchingLabels(labelPrefix)
	if err != nil {
		return nil, err
	}
	if len(matchedLabels) == 0 {
		return nil, fmt.Errorf("no labels found matching prefix %q", labelPrefix)
	}

	var listings []FolderListing
	for _, matchedLabel := range matchedLabels {
		cacheDirName := cfg_g.SanitizeLabelForCache(matchedLabel)

		downloadedIDs, err := client.FetchAndDownloadEmails(label.FromServer(matchedLabel), cacheDirName)
		if err != nil {
			return nil, fmt.Errorf("error downloading emails for label %s: %w", matchedLabel, err)
		}

		if targetMsgID != "" {
			foundID := findTargetEmailID(downloadedIDs, targetMsgID)
			if foundID == "" {
				continue
			}
			return nil, ByID(opts, w, client, config, foundID, matchedLabel)
		}

		if opts.JSON {
			listings = append(listings, FolderListing{
				Account:    accountName(config),
				Folder:     matchedLabel,
				MessageIDs: append([]string{}, email.IDStrings(downloadedIDs)...),
				Total:      len(downloadedIDs),
			})
			continue
		}

		if len(downloadedIDs) == 0 {
			fmt.Fprintf(w, "%s No emails found in label %s.\n", app.PrefixInfo, matchedLabel)
			continue
		}
		ui.FormatFolderSummary(w, strings.ToUpper(matchedLabel)+" EMAILS", "", downloadedIDs, nil, nil, config, cacheDirName, 0, 0)
	}
	if targetMsgID != "" {
		return nil, fmt.Errorf("no email found with ID %q in any label matching %q", targetMsgID, labelPrefix)
	}
	return listings, nil
}

func accountName(config *cfg_g.Config) string {
	if config == nil {
		return ""
	}
	return string(config.SelectedAccount)
}

// findTargetEmailID resolves ref, as the user typed it, against a folder listing.
func findTargetEmailID(downloadedIDs []email.ID, ref string) email.ID {
	for _, id := range downloadedIDs {
		if email.IDMatches(id, ref) {
			return id
		}
	}
	return ""
}

// ByID shows a single message, identified by ID alone.
//
// It goes through MessageAccessor rather than reading the cache directly, so a
// message that has never been downloaded is fetched from the server instead of
// reported missing. That is what lets `show <id>` work on a result the user has
// only just seen listed.
func ByID(opts Options, w io.Writer, client interface {
	mailclient.MessageAccessor
	mailclient.ConfigProvider
}, config *cfg_g.Config, msgID email.ID, folderName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), msgstore.DefaultFetchTimeout)
	defer cancel()
	return ByIDContext(ctx, opts, w, client, config, msgID, folderName)
}

// ByIDContext shows a single message, identified by ID alone, respecting ctx cancellation.
func ByIDContext(ctx context.Context, opts Options, w io.Writer, client interface {
	mailclient.MessageAccessor
	mailclient.ConfigProvider
}, config *cfg_g.Config, msgID email.ID, folderName string) error {
	rawBytes, err := client.GetRawMessageContext(ctx, msgID)
	if err != nil {
		return fmt.Errorf("failed to retrieve message %s: %w", msgID, err)
	}
	return Render(opts, w, config, msgID, folderName, rawBytes)
}

// Render prints one message: its headers, then either the plain-text body or,
// under --web, the HTML body in a browser.
//
// This was two near-identical functions, one reached through a label prefix and
// one by ID, which had already drifted apart: only one of them printed the To
// header.
// Render writes the message to w. The rendered message is what the user asked
// for, not a progress note, so it goes to a writer rather than the Reporter --
// the two are different channels and only one of them may be suppressed.
//
// PGP content is handled first: a signed message is verified and an encrypted
// one is decrypted, and what is rendered is the result. A message with no PGP
// is passed through untouched.
func Render(opts Options, w io.Writer, config *cfg_g.Config, msgID email.ID, folderName string, rawBytes []byte) error {
	if opts.Raw {
		_, err := w.Write(rawBytes)
		return err
	}

	processed := pgp.Process(context.Background(), rawBytes, pgp.Options{Verify: true, Decrypt: true})
	if opts.JSON {
		return renderJSON(w, msgID, folderName, processed)
	}

	parsed, err := mail.ReadMessage(bytes.NewReader(processed.Raw))
	if err != nil {
		return fmt.Errorf("failed to parse email: %w", err)
	}

	printMessageHeaders(w, parsed, msgID, folderName)
	printSecurityLine(w, processed.Status)

	if opts.Web {
		if html := htmlBodyOf(processed.Raw); html != "" {
			return openHTMLInBrowser(w, html, config)
		}
		fmt.Fprintf(w, "%s No HTML body found in email, displaying plain text instead.\n", app.PrefixWarn)
	}

	fmt.Fprintf(w, "%v\n", bodyText(processed))
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint("======================================================================"))
	fmt.Fprintln(w)
	return nil
}

// bodyText is the text of a processed message. Body is set only when the
// plaintext is not itself a MIME message -- an inline encrypted body, or the
// cleartext of an inline signature -- and those have no headers to extract a
// body from.
func bodyText(processed pgp.Result) string {
	if processed.Body != "" {
		return email.EmbedOSC8Hyperlinks(email.StripEscapeSequences(processed.Body))
	}
	return plainBodyOf(processed.Raw)
}

// printSecurityLine reports the PGP verdict, if there is one.
//
// The colour is the point: a reader skimming output has to be able to tell a
// verified signature from a forged one without reading the sentence.
func printSecurityLine(w io.Writer, status *email.CryptoStatus) {
	if status == nil || status.Tag() == "" {
		return
	}
	fmt.Fprint(w, app.ColorBoldYellow.Sprintf("Security:   "))
	fmt.Fprintf(w, "[%s] %s\n", securityTagColor(status.Tag()), app.ColorWhite.Sprint(status.Description()))
}

func securityTagColor(tag string) string {
	// Green is earned: only VERIFIED and an encryption-only state have it, so
	// a tag added later defaults to the warning colour, never to "trusted".
	switch tag {
	case "INVALID", "REVOKED KEY", "SENDER MISMATCH":
		return app.ColorBoldRed.Sprint(tag)
	case "VERIFIED", "ENCRYPTED":
		return app.ColorBoldGreen.Sprint(tag)
	case "DECRYPTED":
		return app.ColorBoldCyan.Sprint(tag)
	default:
		return app.ColorBoldYellow.Sprint(tag)
	}
}

// htmlBodyOf and plainBodyOf each parse the message afresh.
//
// A mail.Message's Body is an io.Reader and can only be consumed once. Under
// --web the HTML extraction used to run against the same parsed message that
// the plain-text fallback then read, so a message with no HTML part printed the
// "displaying plain text instead" notice followed by nothing at all.
func htmlBodyOf(rawBytes []byte) string {
	parsed, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	if err != nil {
		return ""
	}
	html, err := extractHTMLBodyText(parsed)
	if err != nil {
		return ""
	}
	return html
}

func plainBodyOf(rawBytes []byte) string {
	parsed, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	if err != nil {
		return ""
	}
	body, _ := email.ExtractPlainBodyText(parsed)
	return email.EmbedOSC8Hyperlinks(body)
}

func printMessageHeaders(w io.Writer, parsed *mail.Message, msgID email.ID, folderName string) {
	dec := new(mime.WordDecoder)
	subject := email.DecodeHeader(dec, parsed.Header.Get("Subject"))
	from := email.DecodeHeader(dec, parsed.Header.Get("From"))
	date := email.DecodeHeader(dec, parsed.Header.Get("Date"))
	to := email.DecodeHeader(dec, parsed.Header.Get("To"))

	fmt.Fprintln(w)
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint("======================================================================"))
	fmt.Fprint(w, app.ColorBoldGreen.Sprintf("Message ID: "))
	fmt.Fprintln(w, app.ColorWhite.Sprint(msgID))
	if folderName != "" {
		fmt.Fprint(w, app.ColorBoldGreen.Sprintf("Folder:     "))
		fmt.Fprintln(w, app.ColorWhite.Sprint(folderName))
	}
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint("======================================================================"))
	fmt.Fprint(w, app.ColorBoldYellow.Sprintf("From:       "))
	fmt.Fprintln(w, app.ColorWhite.Sprint(from))
	fmt.Fprint(w, app.ColorBoldYellow.Sprintf("To:         "))
	fmt.Fprintln(w, app.ColorWhite.Sprint(to))
	fmt.Fprint(w, app.ColorBoldYellow.Sprintf("Subject:    "))
	fmt.Fprintln(w, app.ColorBold.Sprint(subject))
	fmt.Fprint(w, app.ColorBoldYellow.Sprintf("Date:       "))
	fmt.Fprintln(w, app.ColorWhite.Sprint(date))
	fmt.Fprintln(w, app.ColorBoldCyan.Sprint("----------------------------------------------------------------------"))
}

func extractHTMLBodyText(msg *mail.Message) (string, error) {
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		bodyBytes, _ := io.ReadAll(msg.Body)
		return string(bodyBytes), nil
	}

	encoding := msg.Header.Get("Content-Transfer-Encoding")

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(msg.Body, params["boundary"])
		var htmlParts []string

		for {
			part, partErr := mr.NextPart()
			if partErr == io.EOF {
				break
			}
			if partErr != nil {
				return "", partErr
			}

			partMediaType, _, mediaErr := mime.ParseMediaType(part.Header.Get("Content-Type"))
			if mediaErr != nil {
				continue
			}

			partEncoding := part.Header.Get("Content-Transfer-Encoding")
			partBytes, decodeErr := email.DecodePartBody(part, partEncoding)
			if decodeErr != nil {
				continue
			}

			if partMediaType == "text/html" {
				htmlParts = append(htmlParts, string(partBytes))
			} else if strings.HasPrefix(partMediaType, "multipart/") {
				_, nestedParams, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
				nestedMR := multipart.NewReader(part, nestedParams["boundary"])
				_, nestedHTML, nestedErr := email.ExtractNestedMultipartParts(nestedMR)
				if nestedErr == nil {
					htmlParts = append(htmlParts, nestedHTML...)
				}
			}
		}

		if len(htmlParts) > 0 {
			return strings.Join(htmlParts, "\n"), nil
		}
		return "", nil
	}

	bodyBytes, err := email.DecodePartBody(msg.Body, encoding)
	if err != nil {
		return "", err
	}

	if mediaType == "text/html" {
		return string(bodyBytes), nil
	}
	return fmt.Sprintf("<html><body><pre>%s</pre></body></html>", string(bodyBytes)), nil
}

func openHTMLInBrowser(w io.Writer, html string, config *cfg_g.Config) error {
	tempDir, err := app.GetTempDir()
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	tempPath := filepath.Join(tempDir, "show.html")
	if err := os.WriteFile(tempPath, []byte(html), 0600); err != nil {
		return fmt.Errorf("failed to write html to temp file: %w", err)
	}

	fmt.Fprintf(w, "%s Opening email in browser...\n", app.PrefixInfo)

	if config.Browser != "" {
		errPreferred := browser.OpenWith(config.Browser, tempPath)
		if errPreferred == nil {
			return nil
		}
		fmt.Fprintf(w, "%s Failed to launch your configured browser %q: %v. Falling back to system defaults...\n",
			app.PrefixWarn, config.Browser, errPreferred)
	}

	if err := browser.Open(tempPath); err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	return nil
}

// MessageDetailJSON represents detailed email metadata for JSON output.
type MessageDetailJSON struct {
	ID          string           `json:"id"`
	MessageID   string           `json:"message_id,omitempty"`
	Folder      string           `json:"folder,omitempty"`
	Subject     string           `json:"subject"`
	From        string           `json:"from"`
	To          []string         `json:"to,omitempty"`
	Cc          []string         `json:"cc,omitempty"`
	Bcc         []string         `json:"bcc,omitempty"`
	Date        string           `json:"date"`
	BodyPlain   string           `json:"body_plain"`
	BodyHTML    string           `json:"body_html,omitempty"`
	Attachments []AttachmentJSON `json:"attachments,omitempty"`
	// Crypto is what the PGP layer reported, and is absent for ordinary mail.
	Crypto *email.CryptoStatus `json:"crypto,omitempty"`
}

// AttachmentJSON represents an email attachment in JSON output.
type AttachmentJSON struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

func renderJSON(w io.Writer, msgID email.ID, folderName string, processed pgp.Result) error {
	rawBytes := processed.Raw
	parsed, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	if err != nil {
		return fmt.Errorf("failed to parse email: %w", err)
	}

	dec := new(mime.WordDecoder)
	subject := email.DecodeHeader(dec, parsed.Header.Get("Subject"))
	from := email.DecodeHeader(dec, parsed.Header.Get("From"))
	date := email.DecodeHeader(dec, parsed.Header.Get("Date"))
	msgHeaderID := parsed.Header.Get("Message-ID")

	to := parseAddressList(dec, parsed.Header.Get("To"))
	cc := parseAddressList(dec, parsed.Header.Get("Cc"))
	bcc := parseAddressList(dec, parsed.Header.Get("Bcc"))

	plainBody := bodyText(processed)
	htmlBody := htmlBodyOf(rawBytes)

	var attJSONs []AttachmentJSON
	if atts, extractErr := email.ExtractAttachments(rawBytes); extractErr == nil {
		for _, a := range atts {
			attJSONs = append(attJSONs, AttachmentJSON{
				Filename:    a.Filename,
				ContentType: a.ContentType,
				Size:        len(a.Data),
			})
		}
	}

	detail := MessageDetailJSON{
		ID:          string(msgID),
		MessageID:   msgHeaderID,
		Folder:      folderName,
		Subject:     subject,
		From:        from,
		To:          to,
		Cc:          cc,
		Bcc:         bcc,
		Date:        date,
		BodyPlain:   plainBody,
		BodyHTML:    htmlBody,
		Attachments: attJSONs,
		Crypto:      processed.Status,
	}

	data, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding email detail as JSON: %w", err)
	}
	fmt.Fprintf(w, "%s\n", data)
	return nil
}

func parseAddressList(dec *mime.WordDecoder, hdr string) []string {
	if strings.TrimSpace(hdr) == "" {
		return nil
	}
	addrs, err := mail.ParseAddressList(hdr)
	if err != nil {
		parts := strings.Split(hdr, ",")
		var out []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				out = append(out, email.DecodeHeader(dec, trimmed))
			}
		}
		return out
	}
	out := make([]string, len(addrs))
	for i, a := range addrs {
		name := email.DecodeHeader(dec, a.Name)
		if name != "" {
			out[i] = fmt.Sprintf("%s <%s>", name, a.Address)
		} else {
			out[i] = a.Address
		}
	}
	return out
}
