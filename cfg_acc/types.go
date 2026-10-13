package cfg_acc

import (
	"strings"

	"github.com/sarielhp/mail_cli/label"
)

// AccountConfig stores configuration for an individual mail account.
type AccountConfig struct {
	Name            string   `json:"name"`
	DisplayName     string   `json:"display_name,omitempty"`
	UserName        string   `json:"user_name,omitempty"`
	UserEmail       string   `json:"user_email,omitempty"`
	Type            string   `json:"type"`
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	IMAPHost        string   `json:"imap_host,omitempty"`
	SessionURL      string   `json:"jmap_session_url,omitempty"`
	SpamFolder      string   `json:"spam_folder,omitempty"`
	ReceivedFolder  string   `json:"received_folder,omitempty"`
	SpamLearn       string   `json:"spam_learn,omitempty"`
	UnspamLearn     string   `json:"unspam_learn,omitempty"`
	Aliases         []string `json:"aliases,omitempty"`
	Whitelist       []string `json:"whitelist,omitempty"`
	Blacklist       []string `json:"blacklist,omitempty"`
	Rules           []Rule   `json:"rules,omitempty"`
	CalendarManager bool     `json:"calendar_manager,omitempty"`
	AccountType     string   `json:"account_type,omitempty"`
	ReadOnly        bool     `json:"read_only,omitempty"`
	ReadOnlyFolders []string `json:"read_only_folders,omitempty"`
	SaveSentCopy    *bool    `json:"save_sent_copy,omitempty"`
	// PgpKeyID names the key to sign with when the command line does not name
	// one. Empty means "whatever gpg considers the default key".
	PgpKeyID string `json:"pgp_key_id,omitempty"`
	// Badge is the short tag (an emoji or a letter or two) that marks this
	// account's rows where several accounts are listed together. Empty means
	// the first letter of Name.
	Badge string `json:"badge,omitempty"`
}

// FolderDefaults is where an account of type accType keeps spam and where
// `mail_cli` files what it has read, as `account new` sets them. ok is false
// for a type it does not know.
func FolderDefaults(accType string) (spam, received string, ok bool) {
	switch strings.ToLower(accType) {
	case "gmail":
		return "[Gmail]/Spam", "received", true
	case "jmap":
		return "Spam", "Inbox", true
	case "outlook":
		return "Junk Email", "Archive", true
	}
	return "", "", false
}

// IsFolderReadOnly returns whether folder is marked read-only for this account.
func (acc AccountConfig) IsFolderReadOnly(folder string) bool {
	if acc.ReadOnly {
		return true
	}
	for _, f := range acc.ReadOnlyFolders {
		if strings.EqualFold(f, folder) {
			return true
		}
	}
	return false
}

// IsSaveSentCopyEnabled returns whether saving a sent copy is enabled for this account, falling back to global default.
func (acc AccountConfig) IsSaveSentCopyEnabled(globalDefault bool) bool {
	if acc.SaveSentCopy != nil {
		return *acc.SaveSentCopy
	}
	return globalDefault
}

// GetDisplayName returns the display name if configured, otherwise falls back to the internal name.
func (acc AccountConfig) GetDisplayName() string {
	if acc.DisplayName != "" {
		return acc.DisplayName
	}
	return acc.Name
}

// GetSenderName returns the name to put in the From header of outgoing mail.
//
// It is the person's name (UserName), not the account's label. DisplayName
// names the account in listings and the TUI switcher; reading it as the sender
// is how a message went out as "FastMail" instead of its author. An empty
// result means no name is known, and the caller decides the fallback.
func (acc AccountConfig) GetSenderName() string {
	return acc.UserName
}

// GetSendAddress returns the address outgoing mail is sent from: the configured
// user email if set, otherwise the account's login username.
//
// UserEmail exists for the case the login is not the address to write from --
// Fastmail, for instance, authenticates as sarielhp@sarielhp.org while the user
// sends as an alias such as sariel@sarielhp.org. Username stays the credential;
// only the From header and the submission envelope use this.
func (acc AccountConfig) GetSendAddress() string {
	if acc.UserEmail != "" {
		return acc.UserEmail
	}
	return acc.Username
}

// MatchesName returns true if name matches the account's Name, DisplayName, Type, or any of its Aliases (case-insensitive).
func (acc AccountConfig) MatchesName(name string) bool {
	if strings.EqualFold(acc.Name, name) {
		return true
	}
	if acc.DisplayName != "" && strings.EqualFold(acc.DisplayName, name) {
		return true
	}
	if acc.Type != "" && strings.EqualFold(acc.Type, name) {
		return true
	}
	for _, alias := range acc.Aliases {
		if strings.EqualFold(alias, name) {
			return true
		}
	}
	return false
}

// Item is an alias for label.Item.
type Item = label.Item

// MessageFolderRef is an alias for label.MessageFolderRef.
type MessageFolderRef = label.MessageFolderRef

// Rule defines a custom labeling and routing rule.
type Rule struct {
	Sender   string `json:"sender,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Label    string `json:"label"`
	Exported bool   `json:"exported"`
	Internal bool   `json:"internal,omitempty"`
}

// MatchRules searches a slice of rules for a match against the given sender and subject,
// returning a pointer to the matching rule or nil if none match.
func MatchRules(rules []Rule, sender, subject string) *Rule {
	cleanSender := strings.ToLower(strings.TrimSpace(sender))
	cleanSubject := strings.ToLower(strings.TrimSpace(subject))

	for i := range rules {
		r := &rules[i]
		if r.Subject != "" {
			if strings.HasPrefix(cleanSubject, strings.ToLower(strings.TrimSpace(r.Subject))) {
				return r
			}
		} else if r.Sender != "" {
			cleanRuleSender := strings.ToLower(strings.TrimSpace(r.Sender))
			if strings.EqualFold(cleanSender, cleanRuleSender) {
				return r
			}
			domain := cleanRuleSender
			domain = strings.TrimPrefix(domain, "*")
			if strings.HasPrefix(domain, "@") {
				if strings.HasSuffix(cleanSender, domain) {
					return r
				}
			} else if domain != "" && !strings.Contains(domain, "@") {
				if strings.HasSuffix(cleanSender, "@"+domain) || strings.HasSuffix(cleanSender, "."+domain) {
					return r
				}
			}
		}
	}
	return nil
}
