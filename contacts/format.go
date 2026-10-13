// Package contacts holds the local address book: the contacts file and its
// history, importers for mutt aliases and Google Contacts, recipient
// harvesting from mail, and the ranked suggestions offered while composing.
package contacts

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// FormatTable formats a slice of Contacts into a clean tabular listing.
func FormatTable(w io.Writer, contacts []Contact) error {
	if len(contacts) == 0 {
		fmt.Fprintln(w, "No contacts found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tPRIMARY EMAIL\tALIASES\tGROUPS\tSOURCE")

	for _, c := range contacts {
		name := c.Name
		if name == "" {
			name = "-"
		}
		email := c.PrimaryEmail()
		if email == "" {
			email = "-"
		}
		aliases := strings.Join(c.Nicknames, ", ")
		if aliases == "" {
			aliases = "-"
		}
		groups := strings.Join(c.Groups, ", ")
		if groups == "" {
			groups = "-"
		}
		source := c.Source
		if source == "" {
			source = "-"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", name, email, aliases, groups, source)
	}

	return tw.Flush()
}
