// Package spam is the manual side of spam handling: marking a message, taking
// the mark back, and training bogofilter from either.
//
// Marking trains before it moves, so the classifier learns from every message a
// user files by hand. That is the whole value of the operation, and it is why
// the TUI's spam key routes through here rather than moving the message itself.
package spam
