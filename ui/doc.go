// Package ui formats the listings the command line prints: folder trees, label
// listings, message summaries, filter listings.
//
// Every function writes to an io.Writer the caller supplies rather than to
// stdout, because the thing a command was asked for is its product and must
// stay separate from the progress reported alongside it. The two used to be the
// same channel, which is why suppressing one suppressed the other.
package ui
