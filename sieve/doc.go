// Package sieve exports an account's routing rules as a Sieve script, for a
// server that can filter mail without this program running.
//
// Rule values reach the script from message senders and subjects, so they are
// not trusted: control characters are dropped rather than escaped, since a
// Sieve quoted string cannot contain a newline at all and a comment ends at
// one. Without that, a rule could break out of its literal and inject script
// into a file the user is told to upload to their mail server.
package sieve
