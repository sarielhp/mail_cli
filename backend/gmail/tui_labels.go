package gmail

import "github.com/sarielhp/mail_cli/report"

// gmailSystemLabels contains the Gmail system labels to include in the TUI folder tree.
var gmailSystemLabels = map[string]bool{
	"INBOX":   true,
	"SENT":    true,
	"TRASH":   true,
	"SPAM":    true,
	"DRAFT":   true,
	"STARRED": true,
}

// getLabelItemsGmailREST returns a []item for all Gmail labels suitable for the TUI.
func getLabelItemsGmailREST(rep report.Reporter, config *Config) ([]item, error) {
	systemLabels, userLabels, err := fetchGmailLabelsREST(rep, config)
	if err != nil {
		return nil, err
	}

	var items []item

	for _, l := range systemLabels {
		if !gmailSystemLabels[l.Name] {
			continue
		}
		items = append(items, item{
			Name:           l.Name,
			FullName:       l.Name,
			MessagesTotal:  l.MessagesTotal,
			MessagesUnread: l.MessagesUnread,
			IsLabel:        true,
			IsSystem:       true,
		})
	}

	for _, l := range userLabels {
		items = append(items, item{
			Name:           l.Name,
			FullName:       l.Name,
			MessagesTotal:  l.MessagesTotal,
			MessagesUnread: l.MessagesUnread,
			IsLabel:        true,
		})
	}

	return items, nil
}
