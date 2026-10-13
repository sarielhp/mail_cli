// Package email parses messages: headers and their encoded words, MIME parts
// and attachments, HTML reduced to the words it displays, and the threading of
// a folder into conversations.
//
// Everything here runs on input the sender chose, so it is written to be
// survivable rather than strict: nesting is depth-capped, a truncated encoded
// word is marked rather than dropped silently, and an unclosed script element
// takes its contents with it instead of leaking them into the reading pane and
// the classifier.
package email
