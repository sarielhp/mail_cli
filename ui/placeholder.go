package ui

import (
	"fmt"
	"time"

	"github.com/sarielhp/mail_cli/email"
)

// PlaceholderEmail is the row a listing shows in place of a message it could
// not read, so a broken cache entry does not silently shorten the list.
func PlaceholderEmail(id email.ID) email.Email {
	return email.Email{
		ID:            id,
		Subject:       fmt.Sprintf("[Error reading cached message ID: %s]", id),
		FromEmail:     "[Error]",
		FromRaw:       "[Error]",
		EmailDate:     time.Now(),
		FormattedDate: email.FormatEmailDate(time.Now()),
	}
}
