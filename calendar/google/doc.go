// Package google integrates with the Google Calendar API: adding events parsed
// from .ics mail attachments, checking whether they are already present, and
// printing a week's schedule.
//
// It lived in backend/gmail, which made every consumer of a mail backend depend
// on a calendar client, and made show, scan and cli import Gmail for reasons
// that had nothing to do with mail. None of this is mail handling, so none of it
// belongs in a mail backend.
//
// The one real tie to Gmail is OAuth: both products authorize against the same
// token and the same scope list. Rather than duplicate that plumbing or import
// backend/gmail here, Service takes the client provider as a field, and the
// composition root in mailops injects gmail.NewOAuthHTTPClient.
package google
