package gmail

import (
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"

	gmailapi "google.golang.org/api/gmail/v1"
)

func markAsReadGmailREST(rep report.Reporter, config *Config, messageIDs []email.ID) error {
	if len(messageIDs) == 0 {
		return nil
	}
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}
	return srv.Users.Messages.BatchModify("me", &gmailapi.BatchModifyMessagesRequest{
		Ids:            email.IDStrings(messageIDs),
		RemoveLabelIds: []string{"UNREAD"},
	}).Do()
}

// DeleteMessages permanently deletes messages; they do not pass through Trash.
func (c *client) DeleteMessages(messageIDs []email.ID, _ label.Folder) error {
	if len(messageIDs) == 0 {
		return nil
	}
	srv, err := GetGmailService(c.rep, c.config)
	if err != nil {
		return err
	}
	return srv.Users.Messages.BatchDelete("me", &gmailapi.BatchDeleteMessagesRequest{
		Ids: email.IDStrings(messageIDs),
	}).Do()
}
