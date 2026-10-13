// Package show renders one message for reading: headers, the decoded body,
// attachments, and any calendar invitation it carries.
//
// It writes to an io.Writer the caller supplies, so the same rendering serves
// the terminal, a pager and a test.
package show
