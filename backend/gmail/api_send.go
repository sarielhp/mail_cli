package gmail

import (
	"encoding/base64"
	"fmt"
	"github.com/sarielhp/mail_cli/report"

	gmailapi "google.golang.org/api/gmail/v1"
)

func (c *client) SendEmail(rawBytes []byte) error {
	return sendGmailEmailREST(c.rep, c.config, rawBytes)
}

func sendGmailEmailREST(rep report.Reporter, config *Config, rawBytes []byte) error {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	rawStr := base64.URLEncoding.EncodeToString(rawBytes)
	gMsg := &gmailapi.Message{
		Raw: rawStr,
	}

	_, err = srv.Users.Messages.Send("me", gMsg).Do()
	if err != nil {
		return fmt.Errorf("failed to send raw email via Gmail REST API: %w", err)
	}
	return nil
}
