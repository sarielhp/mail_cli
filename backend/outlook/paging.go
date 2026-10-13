package outlook

import (
	"github.com/sarielhp/mail_cli/backend/paging"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
)

// eachSpamBatch hands the spam folder to fn a batch at a time, until empty.
//
// The bulk operations used to call FetchAndDownloadEmails once and act on what
// came back. That request carries $top=config.Limit -- 100 by default -- and
// never follows @odata.nextLink, so a Junk folder holding more than the limit
// was partly processed and then reported as "Successfully purged N". The
// residue fed back into every later run.
//
// The loop that walks it to the end is paging.Converge, which owns the exit
// conditions: it re-fetches rather than advancing a cursor, because fn empties
// the folder underneath it and a cursor into a shrinking collection skips
// messages at every page boundary. Go's type inference binds Converge's ID
// parameter to email.ID, so the loop needs no wrapper here.
//
// Fetching is kept as the existing download so that the bodies land in the
// cache: spam training reads them after the move, and an ID-only listing would
// have quietly stopped feeding it.
func (c *client) eachSpamBatch(spamFolder string, fn func(ids []email.ID) (int, error)) (int, error) {
	next := func() ([]email.ID, error) {
		return c.FetchAndDownloadEmails(label.FromServer(spamFolder), "spam")
	}
	return paging.Converge(next, fn)
}
