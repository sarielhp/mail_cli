package email

import "time"

// SummaryJSON is the canonical JSON representation of an email for CLI outputs.
type SummaryJSON struct {
	ID            string    `json:"id"`
	ShortID       string    `json:"short_id,omitempty"`
	Account       string    `json:"account,omitempty"`
	Subject       string    `json:"subject"`
	From          string    `json:"from"`
	FromEmail     string    `json:"from_email,omitempty"`
	Date          time.Time `json:"date"`
	FormattedDate string    `json:"formatted_date,omitempty"`
	Folder        string    `json:"folder,omitempty"`
	IsRead        bool      `json:"is_read"`
	IsReplied     bool      `json:"is_replied"`
	HasAttachment bool      `json:"has_attachment"`
	HasICS        bool      `json:"has_ics"`
	IsSpam        bool      `json:"is_spam"`
	IsPolitical   bool      `json:"is_political"`
}

// ToSummaryJSON converts an Email into its JSON representation.
func ToSummaryJSON(e Email, folder string) SummaryJSON {
	return SummaryJSON{
		ID:            string(e.ID),
		ShortID:       string(ComputeShortID(e.ID)),
		Account:       e.Account,
		Subject:       e.Subject,
		From:          e.FromRaw,
		FromEmail:     e.FromEmail,
		Date:          e.EmailDate,
		FormattedDate: e.FormattedDate,
		Folder:        folder,
		IsRead:        e.IsRead,
		IsReplied:     e.IsReplied,
		HasAttachment: e.HasAttachment,
		HasICS:        e.HasICS,
		IsSpam:        e.IsSpam,
		IsPolitical:   e.IsPolitical,
	}
}

// ToSummariesJSON converts a slice of emails into their JSON representation.
func ToSummariesJSON(emails []Email, folderMap map[ID]string) []SummaryJSON {
	out := make([]SummaryJSON, len(emails))
	for i, e := range emails {
		f := ""
		if folderMap != nil {
			f = folderMap[e.ID]
		}
		out[i] = ToSummaryJSON(e, f)
	}
	return out
}
