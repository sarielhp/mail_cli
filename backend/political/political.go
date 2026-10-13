// Package political classifies cached messages as political donation mail.
//
// Every backend's political-spam command walks its spam folder the same way:
// read the cached body, drop a whitelisted sender, and score what is left. The
// walk is the same on each backend, so it lives here.
package political

import (
	"bytes"
	"mime"
	"net/mail"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
)

// maxBodyBytes bounds how much of a body is scored, as the detector does not
// read beyond it.
const maxBodyBytes = 8192

// FilterIDs returns the subset of ids whose cached message reads as political
// donation mail. A message that cannot be read, or whose sender is whitelisted,
// is skipped.
func FilterIDs(accountCache cachedir.Account, ids []email.ID, whitelist []string) []email.ID {
	dec := new(mime.WordDecoder)
	var politicalIDs []email.ID

	for _, id := range ids {
		emailBytes, err := msg.Read(accountCache, id)
		if err != nil {
			continue
		}

		localEmail, errMail := mail.ReadMessage(bytes.NewReader(emailBytes))
		if errMail != nil {
			continue
		}

		fromHeader := email.DecodeHeader(dec, localEmail.Header.Get("From"))
		sender := email.ParseEmailAddress(fromHeader)
		if cfg_g.IsWhitelisted(sender, whitelist) {
			continue
		}

		subject := email.DecodeHeader(dec, localEmail.Header.Get("Subject"))
		bodyStr, _ := email.ExtractPlainBodyText(localEmail)
		bodyStr = email.StripHTML(bodyStr)
		if len(bodyStr) > maxBodyBytes {
			bodyStr = bodyStr[:maxBodyBytes]
		}

		isPolitical, _, _ := email.DetectPolitical(subject, bodyStr)
		if isPolitical {
			politicalIDs = append(politicalIDs, id)
		}
	}
	return politicalIDs
}
