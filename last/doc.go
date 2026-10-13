// Package last implements the listing of the most recent messages across every
// folder of an account.
//
// The result is also saved as a named virtual mailbox, so the IDs it printed
// can be referred to afterwards. The listing is ordered oldest first, which
// puts the newest message at the bottom -- nearest the prompt, where the eye
// already is.
package last
