package contacts

import (
	"time"
)

// Contact represents a single contact or distribution list from any source.
type Contact struct {
	ID        string   `json:"id"`
	Name      string   `json:"name,omitempty"`
	Emails    []string `json:"emails,omitempty"`
	Nicknames []string `json:"nicknames,omitempty"`
	Groups    []string `json:"groups,omitempty"`
	Source    string   `json:"source,omitempty"`
	Notes     string   `json:"notes,omitempty"`
}

// PrimaryEmail returns the first email address, or empty string.
func (c Contact) PrimaryEmail() string {
	if len(c.Emails) > 0 {
		return c.Emails[0]
	}
	return ""
}

// DisplayAlias returns the primary nickname or alias, or empty string.
func (c Contact) DisplayAlias() string {
	if len(c.Nicknames) > 0 {
		return c.Nicknames[0]
	}
	return ""
}

// Book represents the collection of contacts saved in local storage.
type Book struct {
	Contacts []Contact `json:"contacts"`
	Updated  time.Time `json:"updated"`
}

// NewBook returns an empty Book initialized with current timestamp.
func NewBook() *Book {
	return &Book{
		Contacts: []Contact{},
		Updated:  time.Now().UTC(),
	}
}
