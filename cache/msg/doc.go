// Package msg stores raw message bodies on disk and the classification derived
// from them.
//
// Messages are filed under messages/YYYY/MM/DD taken from the message's own
// Date header rather than the time it was downloaded, so a mailbox groups by
// when its mail was sent. A message with an unparseable date still gets a
// stable home.
package msg
