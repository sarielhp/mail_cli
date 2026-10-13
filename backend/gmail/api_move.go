package gmail

import (
	"encoding/base64"
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

func ensureFolderGmail(rep report.Reporter, config *Config, folderName string) error {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	parts := strings.Split(folderName, "/")
	for _, part := range parts {
		_ = part
		labelsRes, err := srv.Users.Labels.List("me").Do()
		if err != nil {
			return fmt.Errorf("failed to list Gmail labels: %w", err)
		}
		found := false
		for _, l := range labelsRes.Labels {
			if l.Name == folderName {
				found = true
				break
			}
		}
		if !found {
			if err := label.ValidateNewName(folderName); err != nil {
				return err
			}
			_, err := srv.Users.Labels.Create("me", &gmailapi.Label{
				Name:                  folderName,
				LabelListVisibility:   "labelShow",
				MessageListVisibility: "show",
			}).Do()
			if err != nil {
				return fmt.Errorf("failed to create label %q: %w", folderName, err)
			}
		}
	}
	return nil
}

func (c *client) EnsureLabelExists(folder label.Folder) error {
	name := folder.String()
	return ensureFolderGmail(c.rep, c.config, name)
}

func (c *client) UploadRawEmail(rawBytes []byte, target label.Folder) (email.ID, error) {
	targetLabel := target.String()
	srv, err := GetGmailService(c.rep, c.config)
	if err != nil {
		return "", err
	}

	// Ensure the label exists
	if ensureErr := c.EnsureLabelExists(target); ensureErr != nil {
		return "", ensureErr
	}

	// Resolve target label ID
	destLabelID := ""
	labelsRes, err := srv.Users.Labels.List("me").Do()
	if err == nil {
		for _, l := range labelsRes.Labels {
			if strings.EqualFold(l.Name, targetLabel) {
				destLabelID = l.Id
				break
			}
		}
	}

	if destLabelID == "" {
		return "", fmt.Errorf("target label %q not found on Gmail server", targetLabel)
	}

	msg := &gmailapi.Message{
		Raw:      base64.URLEncoding.EncodeToString(rawBytes),
		LabelIds: []string{destLabelID},
	}

	inserted, err := srv.Users.Messages.Insert("me", msg).Do()
	if err != nil {
		return "", err
	}
	return email.ID(inserted.Id), nil
}
