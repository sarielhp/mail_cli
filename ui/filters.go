package ui

import (
	"fmt"
	"io"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/mailclient"
)

// FormatFilterListing writes a listing of mail rules.
//
// This was three printers, one per backend, because ListFilters was an
// interface method that printed rather than returning anything. They had
// drifted into three layouts for the same information. The backends now report
// what their rules are and this decides how it looks.
func FormatFilterListing(w io.Writer, listing mailclient.FilterListing) {
	const rule = "======================================================================"

	app.ColorBoldCyan.Fprintln(w, "\n"+rule)
	app.ColorBoldCyan.Fprintf(w, "%s\n", centreTitle(listing.Title))
	app.ColorBoldCyan.Fprintln(w, rule)

	if listing.Note != "" {
		fmt.Fprintf(w, "  Note: %s\n\n", listing.Note)
	}

	if len(listing.Filters) == 0 {
		fmt.Fprintln(w, "  No filters found.")
		app.ColorBoldCyan.Fprintln(w, rule)
		return
	}

	for i, f := range listing.Filters {
		header := f.ID
		if f.Status != "" {
			header = fmt.Sprintf("%s (%s)", f.ID, f.Status)
		}
		fmt.Fprintf(w, "  [%d] %s\n", i+1, header)
		if f.Match != "" {
			fmt.Fprintf(w, "      Matches: %s\n", f.Match)
		}
		if f.Action != "" {
			fmt.Fprintf(w, "      Action:  %s\n", f.Action)
		}
	}
	app.ColorBoldCyan.Fprintln(w, rule)
}
