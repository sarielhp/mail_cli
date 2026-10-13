// Package jmap implements the mail client interfaces against a JMAP server.
//
// JMAP answers per message: an Email/set for forty messages can accept
// thirty-eight and refuse two. That is reported upward as a PartialMoveError
// carrying the refusals, so the caller can reconcile what did move instead of
// treating the batch as having done nothing. Bulk operations page by
// re-querying rather than advancing a cursor, because they empty the mailbox
// underneath themselves.
package jmap
